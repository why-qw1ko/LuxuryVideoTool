package com.buleng.luxuryvideo.platform

import android.content.Context
import android.content.Intent
import android.content.pm.PackageInfo
import android.content.pm.PackageManager
import android.net.Uri
import android.os.SystemClock
import android.provider.Settings
import androidx.core.content.FileProvider
import com.buleng.luxuryvideo.data.AndroidRelease
import com.buleng.luxuryvideo.data.AppVersion
import com.buleng.luxuryvideo.data.appUpdateDownloadUri
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import org.json.JSONObject
import java.io.ByteArrayOutputStream
import java.io.File
import java.net.HttpURLConnection
import java.security.MessageDigest
import java.util.concurrent.atomic.AtomicReference

data class AppUpdateState(
    val currentVersion: AppVersion,
    val serverUrl: String = "",
    val release: AndroidRelease? = null,
    val visible: Boolean = false,
    val checking: Boolean = false,
    val downloading: Boolean = false,
    val downloadedBytes: Long = 0,
    val apkReady: Boolean = false,
    val installing: Boolean = false,
    val awaitingInstallPermission: Boolean = false,
    val message: String? = null,
    val error: String? = null,
) {
    val forced: Boolean get() = release?.forceUpdate == true && release.versionCode > currentVersion.code
}

class AppUpdateController(context: Context, private val scope: CoroutineScope, initialServer: String) {
    private val context = context.applicationContext
    private val preferences = this.context.getSharedPreferences("app_updates", Context.MODE_PRIVATE)
    private val updateDirectory = File(this.context.cacheDir, "app-updates")
    private val mutableState = MutableStateFlow(AppUpdateState(AppVersion.current(this.context)))
    val state = mutableState.asStateFlow()
    private var checkJob: Job? = null
    private var downloadJob: Job? = null
    private var installJob: Job? = null
    private var startupCheckPending = true
    private val checkingConnection = AtomicReference<HttpURLConnection?>()
    private val downloadingConnection = AtomicReference<HttpURLConnection?>()
    @Volatile private var generation = 0L
    @Volatile private var downloadRevision = 0L
    private var readyFile: File? = null
    private var dismissedRelease: String? = null

    init {
        configureServer(initialServer)
    }

    fun configureServer(server: String) {
        val normalized = server.trim().trimEnd('/')
        if (state.value.serverUrl == normalized) return
        generation++
        downloadRevision++
        checkJob?.cancel()
        downloadJob?.cancel()
        installJob?.cancel()
        checkingConnection.getAndSet(null)?.disconnect()
        downloadingConnection.getAndSet(null)?.disconnect()
        readyFile = null
        dismissedRelease = null
        val cached = runCatching {
            val json = preferences.getString(cacheKey(normalized), null) ?: return@runCatching null
            require(json.length <= 65_536)
            parseRelease(JSONObject(json)).takeIf { it.versionCode > state.value.currentVersion.code }
        }.getOrNull()
        mutableState.value = AppUpdateState(state.value.currentVersion, serverUrl = normalized,
            release = cached, visible = cached?.forceUpdate == true)
        if (startupCheckPending && normalized.isNotBlank()) {
            startupCheckPending = false
            check()
        }
    }

    fun check(manual: Boolean = false) {
        if (manual) mutableState.update { it.copy(visible = true, error = null, message = null) }
        val server = state.value.serverUrl
        if (server.isBlank()) {
            if (manual) mutableState.update { it.copy(message = "请先设置服务器地址，再检查更新。") }
            return
        }
        if (checkJob?.isActive == true) return
        val requestGeneration = generation
        mutableState.update { it.copy(checking = true, error = null) }
        checkJob = scope.launch {
            try {
                val release = withContext(Dispatchers.IO) { requestRelease(server) }
                if (requestGeneration != generation) return@launch
                val previous = state.value.release
                val newer = release?.takeIf { it.versionCode > state.value.currentVersion.code }
                val samePackage = previous?.id == newer?.id && previous?.sha256 == newer?.sha256 &&
                    previous?.versionCode == newer?.versionCode
                if (!samePackage) {
                    downloadRevision++
                    downloadJob?.cancel()
                    installJob?.cancel()
                    downloadingConnection.getAndSet(null)?.disconnect()
                    readyFile = null
                }
                val editor = preferences.edit()
                if (newer == null) editor.remove(cacheKey(server))
                else editor.putString(cacheKey(server), releaseJson(newer).toString())
                editor.apply()
                mutableState.update {
                    it.copy(
                        release = newer, checking = false,
                        visible = manual || it.visible || (newer != null &&
                            (newer.forceUpdate || newer.id != dismissedRelease)),
                        downloading = samePackage && it.downloading,
                        downloadedBytes = if (samePackage) it.downloadedBytes else 0,
                        apkReady = samePackage && it.apkReady,
                        installing = samePackage && it.installing,
                        awaitingInstallPermission = samePackage && it.awaitingInstallPermission,
                        message = if (newer == null) "当前已是最新版本。" else null,
                        error = null,
                    )
                }
            } catch (error: Exception) {
                if (error is CancellationException) throw error
                if (requestGeneration == generation) mutableState.update {
                    it.copy(checking = false, error = if (manual || it.visible || it.forced) readableError(error, "暂时无法检查更新，请检查网络后重试。") else null)
                }
            }
        }
    }

    fun dismiss() {
        if (state.value.forced) return
        downloadRevision++
        checkJob?.cancel()
        downloadJob?.cancel()
        installJob?.cancel()
        checkingConnection.getAndSet(null)?.disconnect()
        downloadingConnection.getAndSet(null)?.disconnect()
        dismissedRelease = state.value.release?.id
        mutableState.update { it.copy(visible = false, checking = false, downloading = false, installing = false,
            awaitingInstallPermission = false, error = null, message = null) }
    }

    fun download() {
        val release = state.value.release ?: return
        if (downloadJob?.isActive == true || state.value.installing) return
        val server = state.value.serverUrl
        val requestGeneration = generation
        val requestRevision = ++downloadRevision
        mutableState.update { it.copy(downloading = true, apkReady = false, downloadedBytes = 0, error = null, message = null) }
        downloadJob = scope.launch {
            try {
                val apk = withContext(Dispatchers.IO) {
                    check(updateDirectory.isDirectory || updateDirectory.mkdirs()) { "无法创建更新包缓存目录。" }
                    val destination = File(updateDirectory, release.sha256 + ".apk")
                    if (destination.isFile && runCatching { verifyApk(destination, release) }.isSuccess) destination
                    else {
                        currentCoroutineContext().ensureActive()
                        updateDirectory.listFiles()?.filter {
                            (it != destination && it.name.endsWith(".apk")) ||
                                (it.name.startsWith("update-") && it.name.endsWith(".part"))
                        }?.forEach { it.delete() }
                        downloadApk(server, release, destination, requestGeneration, requestRevision)
                    }
                }
                if (requestGeneration != generation || requestRevision != downloadRevision || state.value.release?.id != release.id) return@launch
                readyFile = apk
                mutableState.update {
                    it.copy(downloading = false, downloadedBytes = release.size, apkReady = true, message = "下载和校验完成，请安装新版。")
                }
            } catch (error: Exception) {
                if (error is CancellationException) throw error
                if (requestGeneration == generation && requestRevision == downloadRevision && state.value.release?.id == release.id) mutableState.update {
                    it.copy(downloading = false, apkReady = false, error = readableError(error, "更新包下载失败，请重试。"))
                }
            }
        }
    }

    fun install(activityContext: Context) {
        val release = state.value.release ?: return
        val apk = readyFile
        if (apk == null || !apk.isFile) {
            readyFile = null
            mutableState.update { it.copy(apkReady = false, error = "更新包缓存已清理，请重新下载。") }
            return
        }
        if (installJob?.isActive == true || state.value.downloading) return
        val requestGeneration = generation
        val requestRevision = downloadRevision
        mutableState.update { it.copy(installing = true, error = null, message = null) }
        installJob = scope.launch {
            try {
                withContext(Dispatchers.IO) { verifyApk(apk, release) }
                if (requestGeneration != generation || requestRevision != downloadRevision || state.value.release?.id != release.id) return@launch
                if (!context.packageManager.canRequestPackageInstalls()) {
                    mutableState.update { it.copy(installing = false, awaitingInstallPermission = true,
                        message = "请允许此应用安装更新，返回后将继续安装。") }
                    activityContext.startActivity(Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES, Uri.parse("package:" + context.packageName)))
                } else {
                    val uri = FileProvider.getUriForFile(context, context.packageName + ".updates", apk)
                    activityContext.startActivity(Intent(Intent.ACTION_VIEW)
                        .setDataAndType(uri, "application/vnd.android.package-archive")
                        .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION))
                    mutableState.update { it.copy(installing = false, awaitingInstallPermission = false,
                        message = "已打开系统安装界面。如取消安装，可再次点击安装新版。") }
                }
            } catch (error: Exception) {
                if (error is CancellationException) throw error
                if (requestGeneration == generation && requestRevision == downloadRevision && state.value.release?.id == release.id) mutableState.update {
                    it.copy(installing = false, awaitingInstallPermission = false,
                        error = readableError(error, "无法打开系统安装界面，请重试。"))
                }
            }
        }
    }

    fun onResume(activityContext: Context) {
        if (!state.value.awaitingInstallPermission) return
        mutableState.update { it.copy(awaitingInstallPermission = false) }
        if (context.packageManager.canRequestPackageInstalls()) install(activityContext)
        else mutableState.update { it.copy(error = "尚未允许安装更新，可点击安装新版重新授权。") }
    }

    private suspend fun requestRelease(server: String): AndroidRelease? {
        currentCoroutineContext().ensureActive()
        val connection = open(server, "/api/v1/app/android/version")
        checkingConnection.set(connection)
        try {
            val startedAt = SystemClock.elapsedRealtime()
            require(connection.responseCode == HttpURLConnection.HTTP_OK) {
                if (connection.responseCode == HttpURLConnection.HTTP_NOT_FOUND) "当前服务器尚未提供应用更新。" else "服务器暂时无法检查更新，请重试。"
            }
            val bytes = ByteArrayOutputStream()
            connection.inputStream.use { input ->
                val buffer = ByteArray(8192)
                while (true) {
                    currentCoroutineContext().ensureActive()
                    require(SystemClock.elapsedRealtime() - startedAt < 30_000) { "检查更新超时，请重试。" }
                    val count = input.read(buffer)
                    if (count == -1) break
                    require(bytes.size() + count <= 65_536) { "更新信息过大。" }
                    bytes.write(buffer, 0, count)
                }
            }
            currentCoroutineContext().ensureActive()
            val json = JSONObject(bytes.toString(Charsets.UTF_8.name()))
            require(json.has("release")) { "服务器更新信息不完整。" }
            return if (json.isNull("release")) null else parseRelease(json.getJSONObject("release"))
        } finally {
            checkingConnection.compareAndSet(connection, null)
            connection.disconnect()
        }
    }

    private suspend fun downloadApk(server: String, release: AndroidRelease, destination: File, requestGeneration: Long, requestRevision: Long): File {
        val partial = File.createTempFile("update-", ".part", updateDirectory)
        var connection: HttpURLConnection? = null
        try {
            val downloadConnection = open(server, release.downloadUrl)
            connection = downloadConnection
            downloadingConnection.set(downloadConnection)
            currentCoroutineContext().ensureActive()
            require(downloadConnection.responseCode == HttpURLConnection.HTTP_OK) { "更新包暂时无法下载，请重新检查更新。" }
            val contentLength = downloadConnection.contentLengthLong
            require(contentLength == -1L || contentLength == release.size) { "更新包大小与发布信息不符。" }
            val startedAt = SystemClock.elapsedRealtime()
            var received = 0L
            var lastProgressAt = 0L
            downloadConnection.inputStream.use { input ->
                partial.outputStream().use { output ->
                    val buffer = ByteArray(64 * 1024)
                    while (true) {
                        currentCoroutineContext().ensureActive()
                        require(SystemClock.elapsedRealtime() - startedAt < 15 * 60_000L) { "更新包下载超时，请重试。" }
                        val count = input.read(buffer)
                        if (count == -1) break
                        received += count
                        require(received <= release.size) { "更新包大小超过发布信息。" }
                        output.write(buffer, 0, count)
                        val now = SystemClock.elapsedRealtime()
                        if (now - lastProgressAt >= 150 && requestGeneration == generation && requestRevision == downloadRevision) {
                            mutableState.update { it.copy(downloadedBytes = received) }
                            lastProgressAt = now
                        }
                    }
                }
            }
            currentCoroutineContext().ensureActive()
            verifyApk(partial, release)
            currentCoroutineContext().ensureActive()
            check(requestGeneration == generation && requestRevision == downloadRevision) { "更新下载已取消。" }
            check((!destination.exists() || destination.delete()) && partial.renameTo(destination)) { "无法保存更新包，请重试。" }
            return destination
        } finally {
            downloadingConnection.compareAndSet(connection, null)
            connection?.disconnect()
            partial.delete()
        }
    }

    @Suppress("DEPRECATION")
    private fun verifyApk(file: File, release: AndroidRelease) {
        require(file.length() == release.size) { "更新包不完整，请重新下载。" }
        val digest = MessageDigest.getInstance("SHA-256")
        file.inputStream().use { input ->
            val buffer = ByteArray(64 * 1024)
            while (true) {
                val count = input.read(buffer)
                if (count == -1) break
                digest.update(buffer, 0, count)
            }
        }
        require(hex(digest.digest()) == release.sha256) { "更新包校验失败，请重新下载。" }
        val manager = context.packageManager
        val candidate = manager.getPackageArchiveInfo(file.absolutePath, PackageManager.GET_SIGNING_CERTIFICATES)
            ?: throw IllegalArgumentException("更新包不是有效的 Android 安装包。")
        val current = manager.getPackageInfo(context.packageName, PackageManager.GET_SIGNING_CERTIFICATES)
        require(candidate.packageName == context.packageName && candidate.packageName == release.packageName) { "更新包不属于当前应用。" }
        require(candidate.longVersionCode == release.versionCode && candidate.versionName.orEmpty() == release.versionName &&
            candidate.longVersionCode > current.longVersionCode) {
            "更新包版本与发布信息不符，请联系管理员。"
        }
        require(candidate.applicationInfo?.minSdkVersion?.let { it <= android.os.Build.VERSION.SDK_INT } == true) {
            "新版暂不支持当前 Android 版本。"
        }
        require(signaturesCompatible(current, candidate)) { "更新包签名与当前应用不兼容，请联系管理员。" }
    }

    private fun signaturesCompatible(current: PackageInfo, candidate: PackageInfo): Boolean {
        val installed = current.signingInfo ?: return false
        val update = candidate.signingInfo ?: return false
        val currentSigners = installed.apkContentsSigners?.map { hex(it.toByteArray()) }?.toSet().orEmpty()
        val nextSigners = update.apkContentsSigners?.map { hex(it.toByteArray()) }?.toSet().orEmpty()
        if (currentSigners.isEmpty() || nextSigners.isEmpty()) return false
        if (installed.hasMultipleSigners() || update.hasMultipleSigners()) return currentSigners == nextSigners
        val history = update.signingCertificateHistory?.map { hex(it.toByteArray()) }?.toSet().orEmpty()
        return history.containsAll(currentSigners)
    }

    private fun open(server: String, path: String): HttpURLConnection =
        (appUpdateDownloadUri(server, path).toURL().openConnection() as HttpURLConnection).apply {
            requestMethod = "GET"
            instanceFollowRedirects = false
            connectTimeout = 15_000
            readTimeout = 20_000
            useCaches = false
            setRequestProperty("Cache-Control", "no-cache")
            setRequestProperty("Accept-Encoding", "identity")
            setRequestProperty("X-Platform", "android")
            setRequestProperty("X-App-Version", state.value.currentVersion.name)
        }

    private fun parseRelease(json: JSONObject): AndroidRelease = AndroidRelease(
        id = json.getString("id"),
        versionCode = json.getLong("versionCode"),
        versionName = json.getString("versionName"),
        changelog = json.optString("changelog"),
        forceUpdate = json.getBoolean("forceUpdate"),
        packageName = json.getString("packageName"),
        size = json.getLong("size"),
        sha256 = json.getString("sha256"),
        downloadUrl = json.getString("downloadUrl"),
        publishedAt = json.optString("publishedAt"),
    ).also { it.validate(context.packageName) }

    private fun releaseJson(release: AndroidRelease): JSONObject = JSONObject()
        .put("id", release.id).put("versionCode", release.versionCode).put("versionName", release.versionName)
        .put("changelog", release.changelog).put("forceUpdate", release.forceUpdate)
        .put("packageName", release.packageName).put("size", release.size).put("sha256", release.sha256)
        .put("downloadUrl", release.downloadUrl).put("publishedAt", release.publishedAt)

    private fun cacheKey(server: String): String = hex(MessageDigest.getInstance("SHA-256").digest(server.toByteArray(Charsets.UTF_8)))

    private fun hex(value: ByteArray): String = value.joinToString("") { "%02x".format(it) }

    private fun readableError(error: Exception, fallback: String): String =
        if (error is IllegalArgumentException || error is IllegalStateException) error.message?.take(200) ?: fallback else fallback
}
