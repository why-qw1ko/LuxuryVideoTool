package com.buleng.luxuryvideo.platform

import android.app.DownloadManager
import android.content.Context
import android.net.Uri
import android.os.Environment
import android.os.SystemClock
import java.io.File
import java.net.URI
import java.util.UUID
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import org.json.JSONArray
import org.json.JSONObject

data class DownloadSelection(
    val name: String,
    val mimeType: String,
    val sizeBytes: Long = 0,
    val fileId: String = "",
    val jobId: String = "",
    val admin: Boolean = false,
    val archive: Boolean = false,
    val retryId: Long? = null,
    val sourceKind: String = "",
)

data class DeviceDownload(
    val id: Long,
    val name: String,
    val mimeType: String,
    val totalBytes: Long,
    val downloadedBytes: Long,
    val status: Int,
    val reason: Int,
    val createdAt: Long,
    val source: DownloadSelection,
    val serverUrl: String,
    val ownerId: String,
    val localUri: String = "",
    val bytesPerSecond: Long = 0,
) {
    val active: Boolean get() = status == DownloadManager.STATUS_PENDING ||
        status == DownloadManager.STATUS_RUNNING || status == DownloadManager.STATUS_PAUSED
    val completed: Boolean get() = status == DownloadManager.STATUS_SUCCESSFUL
    val failed: Boolean get() = status == DownloadManager.STATUS_FAILED
    val cancelled: Boolean get() = status == STATUS_CANCELLED
    val progress: Float? get() = if (totalBytes > 0) {
        (downloadedBytes.toFloat() / totalBytes).coerceIn(0f, 1f)
    } else if (completed) 1f else null
    val statusLabel: String get() = when (status) {
        DownloadManager.STATUS_RUNNING -> "下载中"
        DownloadManager.STATUS_PENDING -> "等待下载"
        DownloadManager.STATUS_PAUSED -> when (reason) {
            DownloadManager.PAUSED_WAITING_FOR_NETWORK -> "等待网络"
            DownloadManager.PAUSED_QUEUED_FOR_WIFI -> "等待 Wi-Fi"
            DownloadManager.PAUSED_WAITING_TO_RETRY -> "等待重试"
            else -> "已暂停"
        }
        DownloadManager.STATUS_SUCCESSFUL -> "已完成"
        STATUS_CANCELLED -> "已取消"
        else -> when (reason) {
            401, 403 -> "下载授权已失效"
            404, 410 -> "文件不存在或已过期"
            DownloadManager.ERROR_INSUFFICIENT_SPACE -> "存储空间不足"
            DownloadManager.ERROR_DEVICE_NOT_FOUND -> "存储设备不可用"
            DownloadManager.ERROR_FILE_ALREADY_EXISTS -> "同名文件已存在"
            DownloadManager.ERROR_CANNOT_RESUME -> "无法继续下载"
            DownloadManager.ERROR_FILE_ERROR -> "文件不可用"
            else -> "下载失败"
        }
    }

    companion object {
        const val STATUS_CANCELLED = 0
    }
}

/** 系统负责后台传输；本机仅保存记录和重试来源，不保存令牌或签名地址。 */
class SystemDownloads(context: Context) {
    private val manager = context.applicationContext.getSystemService(DownloadManager::class.java)
    private val storage = context.applicationContext.getSharedPreferences("device_downloads_v1", Context.MODE_PRIVATE)
    private val mutex = Mutex()
    private val samples = mutableMapOf<Long, Pair<Long, Long>>()
    private var records: MutableList<Record>? = null

    suspend fun enqueue(
        url: String,
        headers: Map<String, String>,
        selection: DownloadSelection,
        ownerId: String,
        serverUrl: String,
        wifiOnly: Boolean = false,
    ): Long = withContext(Dispatchers.IO) {
        mutex.withLock {
            val origin = URI(serverUrl)
            val target = URI(url)
            require(target.scheme in setOf("https", "http") && target.userInfo == null && target.fragment == null) { "无效的下载地址" }
            fun port(uri: URI) = if (uri.port >= 0) uri.port else if (uri.scheme.equals("https", true)) 443 else 80
            val sameServer = origin.scheme.equals(target.scheme, true) && origin.host.equals(target.host, true) && port(origin) == port(target)
            val publicMusic = selection.sourceKind == "music" && selection.jobId.isNotBlank() && selection.fileId.isBlank() &&
                !target.host.isNullOrBlank() && headers.keys.all { it.equals("Referer", true) || it.equals("User-Agent", true) || it.equals("Accept", true) }
            require(sameServer || publicMusic) { "下载地址与当前服务器不一致" }
            require(!publicMusic || headers.keys.none { it.equals("Authorization", true) || it.equals("Cookie", true) }) { "原声下载不能携带登录凭证" }
            require(ownerId.isNotBlank()) { "请先登录" }
            val safeName = selection.name.replace(Regex("[\\\\/:*?\"<>|\\p{Cntrl}]"), "_").take(160).ifBlank { "download" }
            val dot = safeName.lastIndexOf('.').takeIf { it > 0 } ?: safeName.length
            val diskName = safeName.substring(0, dot) + "_" + UUID.randomUUID().toString().take(8) + safeName.substring(dot)
            val request = DownloadManager.Request(Uri.parse(url))
                .setTitle(safeName)
                .setDescription("Luxury Capture")
                .setMimeType(selection.mimeType.substringBefore(';').ifBlank { "application/octet-stream" })
                .setDestinationInExternalPublicDir(Environment.DIRECTORY_DOWNLOADS, "LuxuryCapture/$diskName")
                .setNotificationVisibility(DownloadManager.Request.VISIBILITY_VISIBLE_NOTIFY_COMPLETED)
                .setAllowedOverMetered(!wifiOnly)
                .setAllowedOverRoaming(false)
            if (wifiOnly) request.setAllowedNetworkTypes(DownloadManager.Request.NETWORK_WIFI)
            headers.forEach { (name, value) ->
                require(!name.contains('\r') && !name.contains('\n') && !value.contains('\r') && !value.contains('\n'))
                request.addRequestHeader(name, value)
            }
            val id = manager.enqueue(request)
            val items = readRecords().toMutableList()
            items.add(Record(id, selection.copy(name = safeName, retryId = null), ownerId, serverUrl.trimEnd('/'), System.currentTimeMillis(), diskName = diskName))
            try {
                saveRecords(items)
            } catch (error: Exception) {
                items.removeAll { it.id == id }
                manager.remove(id)
                throw error
            }
            id
        }
    }

    suspend fun list(ownerId: String, serverUrl: String): List<DeviceDownload> = withContext(Dispatchers.IO) {
        mutex.withLock {
            val selected = readRecords().filter { it.ownerId == ownerId && it.serverUrl == serverUrl.trimEnd('/') }
            if (selected.isEmpty()) return@withLock emptyList()
            val results = mutableMapOf<Long, SystemRow>()
            val ids = selected.filterNot { it.cancelled }.map { it.id }.toLongArray()
            if (ids.isNotEmpty()) {
                manager.query(DownloadManager.Query().setFilterById(*ids))?.use { cursor ->
                    val idColumn = cursor.getColumnIndexOrThrow(DownloadManager.COLUMN_ID)
                    val statusColumn = cursor.getColumnIndexOrThrow(DownloadManager.COLUMN_STATUS)
                    val reasonColumn = cursor.getColumnIndexOrThrow(DownloadManager.COLUMN_REASON)
                    val bytesColumn = cursor.getColumnIndexOrThrow(DownloadManager.COLUMN_BYTES_DOWNLOADED_SO_FAR)
                    val totalColumn = cursor.getColumnIndexOrThrow(DownloadManager.COLUMN_TOTAL_SIZE_BYTES)
                    while (cursor.moveToNext()) {
                        val id = cursor.getLong(idColumn)
                        val status = cursor.getInt(statusColumn)
                        results[id] = SystemRow(status, cursor.getInt(reasonColumn), cursor.getLong(bytesColumn).coerceAtLeast(0), cursor.getLong(totalColumn),
                            if (status == DownloadManager.STATUS_SUCCESSFUL) manager.getUriForDownloadedFile(id)?.toString().orEmpty() else "")
                    }
                }
            }
            val now = SystemClock.elapsedRealtime()
            selected.sortedByDescending { it.createdAt }.map { item ->
                val row = results[item.id]
                val bytes = row?.bytes ?: 0L
                val previous = samples.put(item.id, bytes to now)
                val speed = if (row?.status == DownloadManager.STATUS_RUNNING && previous != null && now > previous.second) {
                    ((bytes - previous.first).coerceAtLeast(0) * 1000 / (now - previous.second))
                } else 0L
                DeviceDownload(
                    id = item.id, name = item.source.name, mimeType = item.source.mimeType,
                    totalBytes = row?.total?.takeIf { it > 0 } ?: item.source.sizeBytes,
                    downloadedBytes = bytes,
                    status = if (item.cancelled) DeviceDownload.STATUS_CANCELLED else row?.status ?: DownloadManager.STATUS_FAILED,
                    reason = row?.reason ?: DownloadManager.ERROR_FILE_ERROR, createdAt = item.createdAt,
                    source = item.source, serverUrl = item.serverUrl, ownerId = item.ownerId,
                    localUri = row?.uri.orEmpty(), bytesPerSecond = speed,
                )
            }
        }
    }

    suspend fun cancel(id: Long): Unit = withContext(Dispatchers.IO) {
        mutex.withLock {
            val items = readRecords().toMutableList()
            val item = items.firstOrNull { it.id == id } ?: return@withLock
            // 取消前保留已完成项；系统取消接口会同时移除下载文件，确认框会告知。
            manager.query(DownloadManager.Query().setFilterById(id))?.use { cursor ->
                if (cursor.moveToFirst() && cursor.getInt(cursor.getColumnIndexOrThrow(DownloadManager.COLUMN_STATUS)) == DownloadManager.STATUS_SUCCESSFUL) {
                    return@withLock
                }
            }
            manager.remove(id)
            items[items.indexOf(item)] = item.copy(cancelled = true)
            samples.remove(id)
            saveRecords(items)
        }
    }

    suspend fun removeRecord(id: Long, deleteLocalFile: Boolean = false): Unit = withContext(Dispatchers.IO) {
        mutex.withLock {
            val items = readRecords()
            val item = items.firstOrNull { it.id == id } ?: return@withLock
            val cursor = manager.query(DownloadManager.Query().setFilterById(id))
                ?: error("无法读取系统下载记录，请稍后重试。")
            val row = cursor.use {
                if (!cursor.moveToFirst()) null else {
                    val status = cursor.getInt(cursor.getColumnIndexOrThrow(DownloadManager.COLUMN_STATUS))
                    check(status == DownloadManager.STATUS_FAILED || status == DownloadManager.STATUS_SUCCESSFUL) { "请先取消下载" }
                    RemovalRow(status, cursor.getString(cursor.getColumnIndexOrThrow(DownloadManager.COLUMN_LOCAL_URI)).orEmpty())
                }
            }
            if (deleteLocalFile) deleteLocalDownload(item, row)
            // 删除失败或记录持久化失败时保留本机记录，允许用户重试或只移除记录。
            saveRecords(items.filterNot { it.id == id })
            samples.remove(id)
        }
    }

    private fun deleteLocalDownload(item: Record, row: RemovalRow?) {
        val file = runCatching { ownedDownloadFile(item, row?.localUri.orEmpty()) }.getOrElse {
            throw IllegalStateException("无法确认本地文件位置，记录已保留。请在文件管理器删除文件，或取消勾选后仅移除记录。", it)
        }
        check(file != null || item.cancelled || row?.status == DownloadManager.STATUS_FAILED) {
            "无法确认本地文件位置，记录已保留。请在文件管理器删除文件，或取消勾选后仅移除记录。"
        }
        if (file != null) {
            check(Environment.getExternalStorageState(file) == Environment.MEDIA_MOUNTED) {
                "存储设备暂时不可写，记录已保留。请稍后重试，或取消勾选后仅移除记录。"
            }
            check(!file.isDirectory) { "本地路径不是下载文件，记录已保留。" }
        }
        // 优先使用系统下载接口；只对本条记录的确定路径执行文件删除兜底。
        val removed = if (row != null) runCatching { manager.remove(item.id) } else Result.success(0)
        if (file != null) {
            val deleted = runCatching { !file.exists() || file.delete() || !file.exists() }.getOrDefault(false)
            check(deleted) {
                "本地文件无法删除，记录已保留。请在文件管理器删除文件，或取消勾选后仅移除记录。"
            }
        } else {
            check(item.cancelled || removed.getOrDefault(0) > 0) {
                "系统未能删除下载文件，记录已保留。请稍后重试，或取消勾选后仅移除记录。"
            }
        }
    }

    @Suppress("DEPRECATION")
    private fun ownedDownloadFile(item: Record, localUri: String): File? {
        val folder = File(Environment.getExternalStoragePublicDirectory(Environment.DIRECTORY_DOWNLOADS), "LuxuryCapture").canonicalFile
        val saved = item.diskName.takeIf { it.isNotBlank() }?.let { File(folder, it).canonicalFile }
        val reported = localUri.takeIf { it.isNotBlank() }?.let { value ->
            val uri = Uri.parse(value)
            if (uri.scheme == "file" && uri.authority.isNullOrBlank()) uri.path?.let { File(it).canonicalFile } else null
        }
        val file = saved ?: reported ?: return null
        check(file.parentFile == folder && (saved == null || reported == null || saved == reported)) {
            "无法确认本地文件位置，记录已保留。请在文件管理器处理该文件。"
        }
        return file
    }

    private fun readRecords(): MutableList<Record> {
        records?.let { return it }
        val saved = runCatching { JSONArray(storage.getString("records", "[]")) }.getOrElse { JSONArray() }
        return (0 until saved.length()).mapNotNull { index ->
            runCatching {
                val item = saved.getJSONObject(index)
                Record(
                    id = item.getLong("id"),
                    source = DownloadSelection(item.getString("name"), item.optString("mimeType", "application/octet-stream"), item.optLong("size"),
                        item.optString("fileId"), item.optString("jobId"), item.optBoolean("admin"), item.optBoolean("archive"), sourceKind = item.optString("sourceKind")),
                    ownerId = item.getString("owner"), serverUrl = item.getString("server"),
                    createdAt = item.getLong("createdAt"), cancelled = item.optBoolean("cancelled"), diskName = item.optString("diskName"),
                )
            }.getOrNull()
        }.toMutableList().also { records = it }
    }

    private fun saveRecords(items: List<Record>) {
        val json = JSONArray()
        items.forEach { item ->
            json.put(JSONObject().put("id", item.id).put("name", item.source.name).put("mimeType", item.source.mimeType)
                .put("size", item.source.sizeBytes).put("fileId", item.source.fileId).put("jobId", item.source.jobId)
                .put("admin", item.source.admin).put("archive", item.source.archive).put("sourceKind", item.source.sourceKind).put("owner", item.ownerId)
                .put("server", item.serverUrl).put("createdAt", item.createdAt).put("cancelled", item.cancelled).put("diskName", item.diskName))
        }
        check(storage.edit().putString("records", json.toString()).commit()) { "无法保存下载记录" }
        records = items.toMutableList()
    }

    private data class Record(val id: Long, val source: DownloadSelection, val ownerId: String, val serverUrl: String, val createdAt: Long, val cancelled: Boolean = false, val diskName: String = "")
    private data class RemovalRow(val status: Int, val localUri: String)
    private data class SystemRow(val status: Int, val reason: Int, val bytes: Long, val total: Long, val uri: String)
}
