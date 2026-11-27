package com.buleng.luxuryvideo.data

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import org.json.JSONArray
import org.json.JSONObject
import java.security.KeyStore
import java.util.UUID
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

class SessionStore(context: Context) {
    private val applicationContext = context.applicationContext
    val appVersion: AppVersion by lazy { AppVersion.current(applicationContext) }
    private val storage = applicationContext.getSharedPreferences("luxury_client_v1", Context.MODE_PRIVATE)
    private val lock = Any()
    private val associatedData = "${applicationContext.packageName}|session-v1".toByteArray(Charsets.UTF_8)
    private var sessionRevision = 0L

    val preferences: AppPreferences
        get() = synchronized(lock) {
            val value = try {
                JSONObject(storage.getString(PREFERENCES_KEY, "{}") ?: "{}")
            } catch (_: Exception) {
                JSONObject()
            }
            AppPreferences(
                serverUrl = value.text("serverUrl"),
                themeMode = value.text("themeMode", "system").takeIf {
                    it in setOf("system", "light", "dark")
                } ?: "system",
                motionEnabled = value.optBoolean("motionEnabled", true),
                notificationsEnabled = value.optBoolean("notificationsEnabled", true),
            )
        }

    fun savePreferences(value: AppPreferences): Unit = synchronized(lock) {
        val serverChanged = preferences.serverUrl != value.serverUrl
        val json = JSONObject()
            .put("serverUrl", value.serverUrl)
            .put("themeMode", value.themeMode)
            .put("motionEnabled", value.motionEnabled)
            .put("notificationsEnabled", value.notificationsEnabled)
        val editor = storage.edit().putString(PREFERENCES_KEY, json.toString())
        if (serverChanged) editor.remove(SESSION_KEY).remove(CACHE_KEY)
        check(editor.commit()) { "无法保存客户端设置" }
        if (serverChanged) sessionRevision++
    }

    fun loadSession(): UserSession? = synchronized(lock) {
        val encoded = storage.getString(SESSION_KEY, null) ?: return@synchronized null
        try {
            val encrypted = JSONObject(encoded)
            val cipher = Cipher.getInstance("AES/GCM/NoPadding")
            cipher.init(
                Cipher.DECRYPT_MODE,
                encryptionKey(),
                GCMParameterSpec(128, Base64.decode(encrypted.text("iv"), Base64.NO_WRAP)),
            )
            cipher.updateAAD(associatedData)
            val plaintext = cipher.doFinal(Base64.decode(encrypted.text("ciphertext"), Base64.NO_WRAP))
            val payload = JSONObject(String(plaintext, Charsets.UTF_8))
            if (payload.text("serverUrl") != preferences.serverUrl) {
                clearSession()
                return@synchronized null
            }
            val session = sessionFromJson(payload.getJSONObject("session"))
            if (session.accessToken.isBlank() || session.refreshToken.isBlank() || session.user.id.isBlank()) {
                clearSession()
                null
            } else session
        } catch (_: Exception) {
            clearSession()
            null
        }
    }

    internal fun loadSessionFor(serverUrl: String): UserSession? = synchronized(lock) {
        if (preferences.serverUrl == serverUrl) loadSession() else null
    }

    internal fun sessionSnapshotFor(serverUrl: String): Pair<Long, UserSession>? = synchronized(lock) {
        loadSessionFor(serverUrl)?.let { sessionRevision to it }
    }

    internal fun prepareLogin(serverUrl: String): Long = synchronized(lock) {
        clearSession()
        savePreferences(preferences.copy(serverUrl = serverUrl))
        sessionRevision
    }

    internal fun saveSessionIfUnchanged(serverUrl: String, revision: Long, value: UserSession): Boolean = synchronized(lock) {
        if (preferences.serverUrl != serverUrl || sessionRevision != revision) return@synchronized false
        saveSession(value)
        true
    }

    internal fun clearSessionIfMatches(serverUrl: String, refreshToken: String): Unit = synchronized(lock) {
        if (loadSessionFor(serverUrl)?.refreshToken == refreshToken) clearSession()
    }

    fun saveSession(value: UserSession): Unit = synchronized(lock) {
        val previous = loadSession()
        val payload = JSONObject()
            .put("serverUrl", preferences.serverUrl)
            .put("session", value.toJson())
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.ENCRYPT_MODE, encryptionKey())
        cipher.updateAAD(associatedData)
        val ciphertext = cipher.doFinal(payload.toString().toByteArray(Charsets.UTF_8))
        val encrypted = JSONObject()
            .put("iv", Base64.encodeToString(cipher.iv, Base64.NO_WRAP))
            .put("ciphertext", Base64.encodeToString(ciphertext, Base64.NO_WRAP))
        val editor = storage.edit().putString(SESSION_KEY, encrypted.toString())
        if (previous?.user?.id != value.user.id) editor.remove(CACHE_KEY)
        check(editor.commit()) { "无法保存登录状态" }
        sessionRevision++
    }

    fun clearSession(): Unit = synchronized(lock) {
        check(storage.edit().remove(SESSION_KEY).remove(CACHE_KEY).commit()) { "无法清除登录状态" }
        sessionRevision++
    }

    fun loadJobCache(): List<CaptureJob> = synchronized(lock) {
        val session = loadSession() ?: return@synchronized emptyList()
        try {
            val cache = JSONObject(storage.getString(CACHE_KEY, "{}") ?: "{}")
            if (cache.text("userId") != session.user.id || cache.text("serverUrl") != preferences.serverUrl) {
                return@synchronized emptyList()
            }
            cache.objects("jobs").map(CaptureJob::parse)
        } catch (_: Exception) {
            emptyList()
        }
    }

    fun saveJobCache(
        jobs: List<CaptureJob>,
        expectedUserId: String? = null,
        expectedServerUrl: String? = null,
    ) = synchronized(lock) {
        val session = loadSession() ?: return@synchronized
        if (expectedUserId != null && session.user.id != expectedUserId) return@synchronized
        if (expectedServerUrl != null && preferences.serverUrl != expectedServerUrl) return@synchronized
        val cache = JSONObject()
            .put("userId", session.user.id)
            .put("serverUrl", preferences.serverUrl)
            .put("jobs", JSONArray().apply { jobs.take(100).forEach { put(it.raw) } })
        storage.edit().putString(CACHE_KEY, cache.toString()).apply()
    }

    val deviceId: String
        get() = synchronized(lock) {
            storage.getString(DEVICE_KEY, null)?.takeIf(String::isNotBlank) ?: UUID.randomUUID().toString().also {
                check(storage.edit().putString(DEVICE_KEY, it).commit()) { "无法保存设备标识" }
            }
        }

    private fun encryptionKey(): SecretKey {
        val keyStore = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        (keyStore.getKey(KEY_ALIAS, null) as? SecretKey)?.let { return it }
        return KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore").run {
            init(
                KeyGenParameterSpec.Builder(KEY_ALIAS, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                    .setKeySize(256)
                    .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                    .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                    .setRandomizedEncryptionRequired(true)
                    .build(),
            )
            generateKey()
        }
    }

    private fun UserSession.toJson() = JSONObject()
        .put("accessToken", accessToken)
        .put("refreshToken", refreshToken)
        .put("accessTokenExpiresAt", accessTokenExpiresAt)
        .put("refreshTokenExpiresAt", refreshTokenExpiresAt)
        .put("user", JSONObject()
            .put("id", user.id)
            .put("displayName", user.displayName)
            .put("role", user.role)
            .put("isCdk", user.isCdk))

    private fun sessionFromJson(json: JSONObject): UserSession {
        val user = json.getJSONObject("user")
        return UserSession(
            accessToken = json.text("accessToken"),
            refreshToken = json.text("refreshToken"),
            accessTokenExpiresAt = json.text("accessTokenExpiresAt"),
            refreshTokenExpiresAt = json.text("refreshTokenExpiresAt"),
            user = UserProfile(user.text("id"), user.text("displayName"), user.text("role"), user.optBoolean("isCdk")),
        )
    }

    private companion object {
        const val PREFERENCES_KEY = "preferences"
        const val SESSION_KEY = "encrypted_session"
        const val CACHE_KEY = "job_cache"
        const val DEVICE_KEY = "device_id"
        const val KEY_ALIAS = "luxury_capture_session_v1"
    }
}
