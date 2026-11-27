package com.buleng.luxuryvideo.data

import org.json.JSONArray
import org.json.JSONObject
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.Locale

data class AppPreferences(
    val serverUrl: String = "",
    val themeMode: String = "system",
    val motionEnabled: Boolean = true,
    val notificationsEnabled: Boolean = true,
)

data class UserProfile(
    val id: String,
    val displayName: String,
    val role: String,
    val isCdk: Boolean = false,
)

data class UserSession(
    val accessToken: String,
    val refreshToken: String,
    val accessTokenExpiresAt: String,
    val refreshTokenExpiresAt: String,
    val user: UserProfile,
)

data class JobFile(
    val id: String,
    val kind: String,
    val name: String,
    val mimeType: String,
    val sizeBytes: Long,
    val expiresAt: String = "",
    val previewUrl: String = "",
    val thumbnailUrl: String = "",
) {
    val isExpired: Boolean
        get() = expiresAt.isNotBlank() && runCatching {
            !Instant.parse(expiresAt).isAfter(Instant.now())
        }.getOrDefault(false)

    companion object {
        fun parse(json: JSONObject) = JobFile(
            id = json.text("id"),
            kind = json.text("kind", "file"),
            name = json.text("name", "下载文件"),
            mimeType = json.text("mimeType", "application/octet-stream"),
            sizeBytes = json.optLong("sizeBytes", 0L).coerceAtLeast(0L),
            expiresAt = json.text("expiresAt"),
            previewUrl = json.text("previewUrl"),
            thumbnailUrl = json.text("thumbnailUrl"),
        )
    }
}

data class JobMusic(
    val source: String = "unavailable",
    val file: JobFile? = null,
    val title: String = "",
    val artist: String = "",
) {
    companion object {
        fun parse(json: JSONObject?, files: List<JobFile>, work: JSONObject): JobMusic {
            val title = json?.text("title").orEmpty().ifBlank { work.text("musicTitle") }
            val artist = json?.text("artist").orEmpty().ifBlank { work.text("musicArtist") }
            val source = json?.text("source").orEmpty()
            val fallback = if (json == null) work.text("musicUrl") else json.text("fallbackUrl")
            if (json == null || source == "server") {
                val fileId = json?.text("fileId").orEmpty()
                val saved = files.firstOrNull { it.kind == "music" && !it.isExpired && (fileId.isBlank() || it.id == fileId) }
                val serverFile = saved?.let { file ->
                    file.copy(
                        previewUrl = json?.text("url").orEmpty().ifBlank { file.previewUrl },
                        expiresAt = json?.text("expiresAt").orEmpty().ifBlank { file.expiresAt },
                    )
                } ?: if (fileId.isNotBlank()) JobFile(
                    fileId, "music", title.ifBlank { "作品原声" } + ".mp3", "audio/mpeg", 0,
                    expiresAt = json?.text("expiresAt").orEmpty(), previewUrl = json?.text("url").orEmpty(),
                ) else null
                if (serverFile != null && !serverFile.isExpired) return JobMusic("server", serverFile, title, artist)
            }
            val original = if (source == "original") json?.text("url").orEmpty().ifBlank { fallback } else fallback
            if (original.isNotBlank()) return JobMusic(
                "original", JobFile("", "music", title.ifBlank { "作品原声" }, "audio/mpeg", 0, previewUrl = original), title, artist,
            )
            return JobMusic(title = title, artist = artist)
        }
    }
}

data class CaptureJob(
    val id: String,
    val action: String,
    val status: String,
    val progress: Int,
    val statusMessage: String,
    val createdAt: String,
    val updatedAt: String,
    val title: String,
    val authorName: String,
    val description: String,
    val transcript: String,
    val rawText: String,
    val canonicalUrl: String,
    val coverUrl: String,
    val workType: String,
    val musicTitle: String,
    val musicArtist: String,
    val files: List<JobFile>,
    val errorCode: String = "",
    val errorMessage: String = "",
    val mediaExpiresAt: String = "",
    val asrProvider: String = "",
    val raw: JSONObject = JSONObject(),
) {
    val terminal: Boolean get() = status in setOf("completed", "failed", "cancelled")
    val canCancel: Boolean get() = !terminal
    val canRetry: Boolean get() = status == "failed" || status == "cancelled"
    val music: JobMusic get() = JobMusic.parse(raw.optJSONObject("music"), files, raw.optJSONObject("work") ?: JSONObject())
    val mediaRetentionExpiresAt: String
        get() = (sequenceOf(mediaExpiresAt) + files.asSequence()
            .filter { it.kind in setOf("video", "image", "animated", "music") }
            .map { it.expiresAt })
            .mapNotNull { runCatching { Instant.parse(it) }.getOrNull() }
            .minOrNull()?.toString().orEmpty()
    val mediaRetentionExpired: Boolean
        get() = runCatching { !Instant.parse(mediaRetentionExpiresAt).isAfter(Instant.now()) }.getOrDefault(false)

    companion object {
        fun parse(json: JSONObject): CaptureJob {
            val work = json.optJSONObject("work") ?: JSONObject()
            val result = json.optJSONObject("result") ?: JSONObject()
            val error = json.optJSONObject("error") ?: JSONObject()
            return CaptureJob(
                id = json.text("id"),
                action = json.text("action", "info"),
                status = json.text("status", "queued"),
                progress = json.optInt("progress", 0).coerceIn(0, 100),
                statusMessage = json.text("statusMessage"),
                createdAt = json.text("createdAt"),
                updatedAt = json.text("updatedAt"),
                title = work.text("title").ifBlank { "等待解析的作品" },
                authorName = work.text("authorName"),
                description = work.text("description"),
                transcript = result.text("normalizedText").ifBlank {
                    result.text("transcript").ifBlank { result.text("text") }
                },
                rawText = result.text("rawText"),
                canonicalUrl = work.text("canonicalUrl"),
                coverUrl = work.text("coverPreviewUrl").ifBlank { work.text("coverUrl") },
                workType = work.text("type"),
                musicTitle = json.optJSONObject("music")?.text("title").orEmpty().ifBlank { work.text("musicTitle") },
                musicArtist = json.optJSONObject("music")?.text("artist").orEmpty().ifBlank { work.text("musicArtist") },
                files = result.objects("files").map(JobFile::parse),
                errorCode = error.text("code"),
                errorMessage = error.text("message"),
                mediaExpiresAt = json.text("mediaExpiresAt"),
                asrProvider = result.text("asrProvider").ifBlank { json.text("asrProvider") },
                raw = json,
            )
        }
    }
}

fun JSONObject.text(key: String, default: String = ""): String {
    val value = opt(key)
    return if (value == null || value == JSONObject.NULL) default else value.toString()
}

fun JSONObject.objects(key: String): List<JSONObject> = optJSONArray(key)?.objects().orEmpty()

fun JSONArray.objects(): List<JSONObject> = (0 until length()).mapNotNull(::optJSONObject)

fun formatBytes(bytes: Long): String {
    val safeBytes = bytes.coerceAtLeast(0L)
    if (safeBytes < 1024) return "$safeBytes B"
    val units = arrayOf("KB", "MB", "GB", "TB", "PB", "EB")
    var value = safeBytes.toDouble() / 1024.0
    var unit = 0
    while (value >= 1024.0 && unit < units.lastIndex) {
        value /= 1024.0
        unit++
    }
    return String.format(Locale.CHINA, "%.1f %s", value, units[unit])
}

fun formatTime(value: String): String = runCatching {
    DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm", Locale.CHINA)
        .withZone(ZoneId.systemDefault())
        .format(Instant.parse(value))
}.getOrElse { value.ifBlank { "暂无时间" } }

class ApiException(
    val status: Int,
    val code: String,
    override val message: String,
    val retryable: Boolean = false,
    val requestId: String = "",
) : Exception(message)
