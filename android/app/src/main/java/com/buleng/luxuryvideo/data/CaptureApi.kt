package com.buleng.luxuryvideo.data

import android.os.Build
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import org.json.JSONObject
import java.io.IOException
import java.io.OutputStream
import java.net.HttpURLConnection
import java.net.SocketTimeoutException
import java.net.URI
import java.time.Instant
import java.util.Locale
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException

class CaptureApi(val store: SessionStore) {
    private val refreshMutex = Mutex()
    val serverUrl: String get() = store.preferences.serverUrl

    suspend fun login(
        serverUrl: String,
        username: String,
        password: String,
        cdk: String = "",
    ): UserSession = withContext(Dispatchers.IO) {
        val base = normalizeServerUrl(serverUrl)
        refreshMutex.withLock {
            val revision = store.prepareLogin(base)
            val device = JSONObject()
                .put("id", store.deviceId)
                .put("name", "${Build.MANUFACTURER} ${Build.MODEL}".trim().take(128))
                .put("platform", "android")
                .put("appVersion", store.appVersion.name)
            val body = JSONObject().put("device", device)
            val path = if (cdk.isNotBlank()) {
                body.put("code", cdk.trim())
                "/api/v1/auth/cdk/login"
            } else {
                body.put("username", username.trim()).put("password", password)
                "/api/v1/auth/login"
            }
            val session = parseSession(sendJson(base, "POST", path, body))
            if (!store.saveSessionIfUnchanged(base, revision, session)) throw sessionChanged()
            session
        }
    }

    suspend fun restoreSession(): UserSession? = withContext(Dispatchers.IO) {
        val base = serverUrl
        val session = store.loadSessionFor(base) ?: return@withContext null
        if (expired(session.refreshTokenExpiresAt)) {
            clearMatchingSession(base, session)
            return@withContext null
        }
        if (!expired(session.accessTokenExpiresAt)) return@withContext session
        try {
            refresh(base, session)
        } catch (error: ApiException) {
            if (error.status == HttpURLConnection.HTTP_UNAUTHORIZED) null
            else throw error
        }
    }

    suspend fun logout(): Unit = withContext(Dispatchers.IO) {
        refreshMutex.withLock {
            val base = serverUrl
            val (revision, original) = store.sessionSnapshotFor(base) ?: return@withLock
            var current = original
            try {
                try {
                    sendJson(base, "POST", "/api/v1/auth/logout", token = current.accessToken)
                } catch (error: ApiException) {
                    if (!isSessionFailure(error) || error.code == "AUTH_SESSION_REVOKED"
                        || expired(current.refreshTokenExpiresAt)
                    ) throw error
                    current = parseSession(sendJson(
                        base, "POST", "/api/v1/auth/refresh",
                        JSONObject().put("refreshToken", current.refreshToken),
                    ))
                    // 切服时不保存旧站点的新令牌，仍撤销本次刚轮换出的远端会话。
                    store.saveSessionIfUnchanged(base, revision, current)
                    sendJson(base, "POST", "/api/v1/auth/logout", token = current.accessToken)
                }
            } catch (_: ApiException) {
                // 远端不可达或会话已失效时，仍在 finally 中完成本地退出。
            } finally {
                clearMatchingSession(base, current)
                if (current.refreshToken != original.refreshToken) clearMatchingSession(base, original)
            }
        }
    }

    suspend fun request(
        method: String,
        path: String,
        body: JSONObject? = null,
        authenticated: Boolean = true,
        idempotencyKey: String? = null,
    ): JSONObject = withContext(Dispatchers.IO) {
        val base = if (authenticated) configuredServer() else unauthenticatedServer(path)
        val session = if (authenticated) store.loadSessionFor(base) ?: throw loginRequired() else null
        try {
            verifyResponse(base, session, sendRequestJson(base, method, path, body, session?.accessToken, idempotencyKey))
        } catch (error: ApiException) {
            if (!isSessionFailure(error) || session == null) throw error
            if (error.code == "AUTH_SESSION_REVOKED") {
                clearMatchingSession(base, session)
                throw error
            }
            val replacement = refresh(base, session)
            try {
                verifyResponse(base, replacement, sendRequestJson(base, method, path, body, replacement.accessToken, idempotencyKey))
            } catch (retryError: ApiException) {
                if (isSessionFailure(retryError)) clearMatchingSession(base, replacement)
                throw retryError
            }
        }
    }

    fun absoluteUrl(value: String): String {
        if (value.isBlank()) return ""
        val absolute = try {
            val uri = URI(value.trim())
            if (uri.isAbsolute) uri else URI(configuredServer() + "/").resolve(uri)
        } catch (_: Exception) {
            throw ApiException(0, "INVALID_URL", "服务器或媒体地址格式不正确")
        }
        validateHttpUrl(absolute)
        return absolute.toASCIIString()
    }

    suspend fun prepareSystemDownload(path: String, authenticated: Boolean = true): Pair<String, Map<String, String>> = withContext(Dispatchers.IO) {
        val base = configuredServer()
        val target = resolveOnServer(base, path).toASCIIString()
        var session = store.loadSessionFor(base) ?: throw loginRequired()
        if (authenticated && runCatching { !Instant.parse(session.accessTokenExpiresAt).isAfter(Instant.now().plusSeconds(60)) }.getOrDefault(true)) {
            session = refresh(base, session)
        }
        currentCoroutineContext().ensureActive()
        verifyResponse(base, session, JSONObject())
        target to if (authenticated) mapOf("Authorization" to "Bearer ${session.accessToken}") else emptyMap()
    }

    suspend fun downloadTo(path: String, output: OutputStream): Unit = withContext(Dispatchers.IO) {
        val base = configuredServer()
        val session = store.loadSessionFor(base) ?: throw loginRequired()
        try {
            streamDownload(base, path, session.accessToken, output)
        } catch (error: ApiException) {
            if (!isSessionFailure(error)) throw error
            if (error.code == "AUTH_SESSION_REVOKED") {
                clearMatchingSession(base, session)
                throw error
            }
            val replacement = refresh(base, session)
            try {
                streamDownload(base, path, replacement.accessToken, output)
            } catch (retryError: ApiException) {
                if (isSessionFailure(retryError)) clearMatchingSession(base, replacement)
                throw retryError
            }
        }
    }

    private suspend fun refresh(base: String, rejected: UserSession): UserSession = refreshMutex.withLock {
        val (revision, current) = store.sessionSnapshotFor(base) ?: throw loginRequired()
        if (current.user.id != rejected.user.id) throw sessionChanged()
        if (current.accessToken != rejected.accessToken && current.refreshToken != rejected.refreshToken) {
            return@withLock current
        }
        if (current.refreshToken.isBlank() || expired(current.refreshTokenExpiresAt)) {
            clearMatchingSession(base, current)
            throw loginRequired()
        }
        val replacement = try {
            parseSession(sendJson(base, "POST", "/api/v1/auth/refresh", JSONObject().put("refreshToken", current.refreshToken)))
        } catch (error: ApiException) {
            if (error.status == HttpURLConnection.HTTP_UNAUTHORIZED) clearMatchingSession(base, current)
            throw error
        }
        if (!store.saveSessionIfUnchanged(base, revision, replacement)) throw sessionChanged()
        replacement
    }

    private fun clearMatchingSession(base: String, expected: UserSession) {
        store.clearSessionIfMatches(base, expected.refreshToken)
    }

    private fun verifyResponse(base: String, session: UserSession?, response: JSONObject): JSONObject {
        if (session != null && store.loadSessionFor(base)?.user?.id != session.user.id) throw sessionChanged()
        return response
    }

    private suspend fun sendRequestJson(
        base: String,
        method: String,
        path: String,
        body: JSONObject?,
        token: String?,
        idempotencyKey: String?,
    ): JSONObject {
        currentCoroutineContext().ensureActive()
        if (method.uppercase(Locale.ROOT) !in setOf("GET", "HEAD")) return sendJson(base, method, path, body, token, idempotencyKey)
        return suspendCancellableCoroutine { continuation ->
            try {
                val response = sendJson(base, method, path, body, token, idempotencyKey) { connection ->
                    continuation.invokeOnCancellation { connection.disconnect() }
                    if (!continuation.isActive) throw kotlinx.coroutines.CancellationException("请求已取消")
                }
                continuation.resume(response)
            } catch (error: Exception) {
                continuation.resumeWithException(error)
            }
        }
    }

    private fun sendJson(
        base: String,
        method: String,
        path: String,
        body: JSONObject? = null,
        token: String? = null,
        idempotencyKey: String? = null,
        onConnection: ((HttpURLConnection) -> Unit)? = null,
    ): JSONObject {
        val verb = method.uppercase(Locale.ROOT)
        if (verb !in setOf("GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS")) {
            throw ApiException(0, "INVALID_REQUEST", "不支持的请求方法")
        }
        if (body != null && (verb == "GET" || verb == "HEAD")) {
            throw ApiException(0, "INVALID_REQUEST", "读取请求不能包含正文")
        }
        val connection = connection(base, path, verb, token)
        try {
            onConnection?.invoke(connection)
            if (!idempotencyKey.isNullOrBlank()) connection.setRequestProperty("Idempotency-Key", idempotencyKey)
            if (body != null) {
                val encoded = body.toString().toByteArray(Charsets.UTF_8)
                connection.doOutput = true
                connection.setRequestProperty("Content-Type", "application/json; charset=utf-8")
                connection.setFixedLengthStreamingMode(encoded.size)
                connection.outputStream.use { it.write(encoded) }
            }
            val status = connection.responseCode
            val stream = if (status in 200..299) connection.inputStream else connection.errorStream
            val text = stream?.bufferedReader(Charsets.UTF_8)?.use { it.readText() }.orEmpty()
            if (status !in 200..299) throw responseError(connection, status, text)
            if (text.isBlank()) return JSONObject()
            return try {
                JSONObject(text)
            } catch (_: Exception) {
                throw ApiException(status, "INVALID_RESPONSE", "服务器返回的数据格式不正确", requestId = connection.getHeaderField("X-Request-ID").orEmpty())
            }
        } catch (_: SocketTimeoutException) {
            throw ApiException(0, "NETWORK_TIMEOUT", "连接服务器超时，请稍后重试", true)
        } catch (_: IOException) {
            throw ApiException(0, "NETWORK_ERROR", "无法连接服务器，请检查网络和服务器地址", true)
        } finally {
            connection.disconnect()
        }
    }

    private suspend fun streamDownload(base: String, path: String, token: String, output: OutputStream) {
        var target = resolveOnServer(base, path)
        repeat(6) { redirects ->
            currentCoroutineContext().ensureActive()
            val connection = connection(base, target.toASCIIString(), "GET", token)
            try {
                val status = connection.responseCode
                if (status in setOf(301, 302, 303, 307, 308)) {
                    val location = connection.getHeaderField("Location")
                        ?: throw ApiException(status, "INVALID_REDIRECT", "下载地址缺少跳转目标")
                    if (redirects == 5) throw ApiException(status, "TOO_MANY_REDIRECTS", "下载地址跳转次数过多")
                    val redirected = try { target.resolve(location).toASCIIString() } catch (_: Exception) {
                        throw ApiException(status, "INVALID_REDIRECT", "下载跳转地址格式不正确")
                    }
                    target = resolveOnServer(base, redirected)
                    return@repeat
                }
                if (status !in 200..299) {
                    val text = connection.errorStream?.bufferedReader(Charsets.UTF_8)?.use { it.readText() }.orEmpty()
                    throw responseError(connection, status, text)
                }
                connection.inputStream.use { input ->
                    val buffer = ByteArray(DEFAULT_BUFFER_SIZE)
                    while (true) {
                        currentCoroutineContext().ensureActive()
                        val count = input.read(buffer)
                        if (count == -1) break
                        output.write(buffer, 0, count)
                    }
                }
                output.flush()
                return
            } catch (_: SocketTimeoutException) {
                throw ApiException(0, "NETWORK_TIMEOUT", "下载超时，请稍后重试", true)
            } catch (_: IOException) {
                throw ApiException(0, "DOWNLOAD_FAILED", "下载未能完成，请检查网络与存储空间", true)
            } finally {
                connection.disconnect()
            }
        }
    }

    private fun connection(base: String, path: String, method: String, token: String?): HttpURLConnection {
        val target = resolveOnServer(base, path)
        return (target.toURL().openConnection() as HttpURLConnection).apply {
            instanceFollowRedirects = false
            connectTimeout = 45_000
            readTimeout = 60_000
            requestMethod = method
            useCaches = false
            setRequestProperty("Accept", "application/json")
            setRequestProperty("X-App-Version", store.appVersion.name)
            setRequestProperty("X-Platform", "android")
            setRequestProperty("User-Agent", "LuxuryCapture/" + store.appVersion.name + " (Android)")
            if (!token.isNullOrBlank()) setRequestProperty("Authorization", "Bearer $token")
        }
    }

    private fun responseError(connection: HttpURLConnection, status: Int, text: String): ApiException {
        val json = try { JSONObject(text).optJSONObject("error") } catch (_: Exception) { null }
        return ApiException(
            status = status,
            code = json?.text("code")?.ifBlank { null } ?: if (status in 300..399) "HTTP_REDIRECT" else "HTTP_$status",
            message = json?.text("message")?.ifBlank { null }
                ?: if (status in 300..399) "服务器返回了重定向，请检查服务器地址" else "服务器请求失败（HTTP $status）",
            retryable = json?.optBoolean("retryable", false) ?: (status >= 500),
            requestId = json?.text("requestId")?.ifBlank { null } ?: connection.getHeaderField("X-Request-ID").orEmpty(),
        )
    }

    private fun parseSession(json: JSONObject): UserSession {
        val user = json.optJSONObject("user") ?: throw ApiException(0, "INVALID_RESPONSE", "服务器未返回账号信息")
        val session = UserSession(
            json.text("accessToken"), json.text("refreshToken"),
            json.text("accessTokenExpiresAt"), json.text("refreshTokenExpiresAt"),
            UserProfile(user.text("id"), user.text("displayName"), user.text("role"), user.optBoolean("isCdk", false)),
        )
        if (session.accessToken.isBlank() || session.refreshToken.isBlank() || session.user.id.isBlank()
            || runCatching { Instant.parse(session.accessTokenExpiresAt); Instant.parse(session.refreshTokenExpiresAt) }.isFailure
        ) throw ApiException(0, "INVALID_RESPONSE", "服务器返回的登录信息不完整")
        return session
    }

    private fun configuredServer(): String = serverUrl.takeIf(String::isNotBlank)
        ?: throw ApiException(0, "SERVER_REQUIRED", "请先设置服务器地址")

    private fun unauthenticatedServer(path: String): String {
        val target = try { URI(path) } catch (_: Exception) {
            throw ApiException(0, "INVALID_URL", "请求地址格式不正确")
        }
        if (!target.isAbsolute) return configuredServer()
        validateHttpUrl(target)
        return URI(target.scheme, null, target.host, target.port, null, null, null).toASCIIString()
    }

    private fun normalizeServerUrl(value: String): String {
        val uri = try { URI(value.trim()) } catch (_: Exception) {
            throw ApiException(0, "INVALID_URL", "请输入完整的 HTTP 或 HTTPS 服务器地址")
        }
        validateHttpUrl(uri)
        if (uri.rawQuery != null || uri.rawFragment != null) throw ApiException(0, "INVALID_URL", "服务器地址不能包含查询参数或片段")
        return uri.toASCIIString().trimEnd('/')
    }

    private fun resolveOnServer(base: String, path: String): URI {
        val origin = try { URI(base) } catch (_: Exception) { throw ApiException(0, "INVALID_URL", "服务器地址格式不正确") }
        val target = try { URI(base.trimEnd('/') + "/").resolve(path) } catch (_: Exception) {
            throw ApiException(0, "INVALID_URL", "请求地址格式不正确")
        }
        validateHttpUrl(target)
        if (!origin.scheme.equals(target.scheme, true) || !origin.host.equals(target.host, true) || port(origin) != port(target)) {
            throw ApiException(0, "UNSAFE_REDIRECT", "拒绝向其他服务器发送登录凭证")
        }
        return target
    }

    private fun validateHttpUrl(uri: URI) {
        if (uri.scheme?.lowercase(Locale.ROOT) !in setOf("http", "https") || uri.host.isNullOrBlank() || uri.rawUserInfo != null) {
            throw ApiException(0, "INVALID_URL", "请输入不含账号密码的完整 HTTP 或 HTTPS 地址")
        }
    }

    private fun port(uri: URI): Int = if (uri.port >= 0) uri.port else if (uri.scheme.equals("https", true)) 443 else 80
    private fun isSessionFailure(error: ApiException): Boolean = error.status == HttpURLConnection.HTTP_UNAUTHORIZED &&
        error.code in setOf("AUTH_TOKEN_EXPIRED", "AUTH_SESSION_REVOKED", "HTTP_401")
    private fun expired(value: String): Boolean = runCatching { !Instant.parse(value).isAfter(Instant.now()) }.getOrDefault(true)
    private fun loginRequired() = ApiException(401, "AUTH_TOKEN_EXPIRED", "登录状态已失效，请重新登录")
    private fun sessionChanged() = ApiException(401, "AUTH_SESSION_REVOKED", "登录账号或服务器已变化，请重试")
}
