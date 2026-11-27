package com.buleng.luxuryvideo.ui

import android.content.ClipData
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.widget.Toast
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Checkbox
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.buleng.luxuryvideo.CaptureViewModel
import com.buleng.luxuryvideo.data.formatBytes
import com.buleng.luxuryvideo.platform.DeviceDownload
import com.buleng.luxuryvideo.platform.DownloadSelection
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter

@Composable
fun DownloadConfirmation(selection: DownloadSelection, preparing: Boolean, onDismiss: () -> Unit, onConfirm: (Boolean) -> Unit) {
    var wifiOnly by remember(selection) { mutableStateOf(false) }
    AlertDialog(
        onDismissRequest = { if (!preparing) onDismiss() },
        title = { Text(if (selection.archive) "下载 ZIP 压缩包" else "下载文件") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(16.dp)) {
                Row(horizontalArrangement = Arrangement.spacedBy(12.dp), verticalAlignment = Alignment.CenterVertically) {
                    ArcoIcon(if (selection.archive) "archive" else "download", size = 28.dp, tint = MaterialTheme.colorScheme.primary)
                    Column(verticalArrangement = Arrangement.spacedBy(3.dp)) {
                        Text(selection.name, style = MaterialTheme.typography.bodyLarge, maxLines = 3, overflow = TextOverflow.Ellipsis)
                        if (selection.sizeBytes > 0) Text(formatBytes(selection.sizeBytes), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                }
                if (selection.archive) Text("ZIP 压缩包，包含任务中已保存的图片和动图。", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                Text("保存到 下载 / LuxuryCapture", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                Row(Modifier.fillMaxWidth().clip(RoundedCornerShape(10.dp)).clickable(enabled = !preparing) { wifiOnly = !wifiOnly }.padding(horizontal = 10.dp, vertical = 4.dp), verticalAlignment = Alignment.CenterVertically) {
                    Text("仅使用 Wi-Fi", Modifier.weight(1f), style = MaterialTheme.typography.bodyMedium)
                    Switch(checked = wifiOnly, onCheckedChange = { wifiOnly = it }, enabled = !preparing)
                }
            }
        },
        confirmButton = { TextButton(onClick = { onConfirm(wifiOnly) }, enabled = !preparing) { Text(if (preparing) "正在准备…" else "下载") } },
        dismissButton = { TextButton(onClick = onDismiss, enabled = !preparing) { Text("取消") } },
    )
}

@Composable
fun ActiveDownloadsBar(vm: CaptureViewModel, onOpen: () -> Unit) {
    val downloads by vm.downloads.collectAsState()
    val active = downloads.filter { it.active }
    if (active.isEmpty()) return
    val first = active.first()
    Surface(onClick = onOpen, color = MaterialTheme.colorScheme.surface) {
        Column(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 9.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                ArcoIcon("download", size = 18.dp, tint = MaterialTheme.colorScheme.primary)
                Text(if (active.size == 1) first.name else "${active.size} 项下载中", Modifier.weight(1f), maxLines = 1, overflow = TextOverflow.Ellipsis, style = MaterialTheme.typography.bodySmall)
                Text(first.progress?.let { "${(it * 100).toInt()}%" } ?: first.statusLabel, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.primary)
                ArcoIcon("chevron", size = 14.dp)
            }
            val progress = first.progress
            if (progress == null) LinearProgressIndicator(modifier = Modifier.fillMaxWidth().height(2.dp))
            else LinearProgressIndicator(progress = { progress }, modifier = Modifier.fillMaxWidth().height(2.dp))
        }
    }
}

@Composable
fun DownloadsScreen(vm: CaptureViewModel) {
    val downloads by vm.downloads.collectAsState()
    var filter by rememberSaveable { mutableIntStateOf(0) }
    var action by remember { mutableStateOf<Pair<DeviceDownload, Boolean>?>(null) }
    LaunchedEffect(vm) { vm.refreshDownloads() }
    val visible = remember(downloads, filter) { downloads.filter { when (filter) { 1 -> it.active; 2 -> it.completed; 3 -> it.failed || it.cancelled; else -> true } } }
    Column(Modifier.fillMaxSize()) {
        Row(Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()).padding(horizontal = 16.dp, vertical = 5.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            listOf("全部", "下载中", "已完成", "未完成").forEachIndexed { index, label ->
                ChoicePill(label, filter == index) { filter = index }
            }
        }
        if (visible.isEmpty()) {
            Box(Modifier.weight(1f).fillMaxWidth(), contentAlignment = Alignment.Center) {
                Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    ArcoIcon("download", size = 36.dp)
                    Text(if (filter == 0) "暂无下载" else "没有${listOf("", "正在下载", "已完成", "未完成")[filter]}的文件", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
        } else {
            LazyColumn(Modifier.weight(1f).fillMaxWidth(), contentPadding = PaddingValues(start = 16.dp, end = 16.dp, bottom = 24.dp)) {
                item(key = "count") {
                    Text("${visible.size} 个文件", Modifier.padding(start = 4.dp, top = 4.dp, bottom = 10.dp), style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                itemsIndexed(visible, key = { _, item -> item.id }, contentType = { _, _ -> "download" }) { index, item ->
                    val shape = RoundedCornerShape(topStart = if (index == 0) 14.dp else 0.dp, topEnd = if (index == 0) 14.dp else 0.dp,
                        bottomStart = if (index == visible.lastIndex) 14.dp else 0.dp, bottomEnd = if (index == visible.lastIndex) 14.dp else 0.dp)
                    Surface(shape = shape, color = MaterialTheme.colorScheme.surface) {
                        Column {
                            DownloadRow(item, onRetry = { vm.retryDownload(item.id) }, onCancel = { action = item to true }, onRemove = { action = item to false })
                            if (index != visible.lastIndex) HorizontalDivider(Modifier.padding(start = 64.dp), color = MaterialTheme.colorScheme.outlineVariant.copy(alpha = .65f), thickness = .5.dp)
                        }
                    }
                }
            }
        }
    }
    action?.let { (item, cancelling) ->
        var deleteLocalFile by remember(item.id, cancelling) { mutableStateOf(false) }
        AlertDialog(
            onDismissRequest = { action = null },
            title = { Text(if (cancelling) "取消下载？" else "移除记录？") },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    Text(item.name, style = MaterialTheme.typography.bodyLarge)
                    if (cancelling) Text("取消后会移除本次下载及文件。")
                    else {
                        Text(if (deleteLocalFile) "删除文件失败时会保留记录，便于重试。" else "已保存的文件会保留。")
                        Row(
                            Modifier.fillMaxWidth().clip(RoundedCornerShape(10.dp)).clickable { deleteLocalFile = !deleteLocalFile }.padding(horizontal = 6.dp, vertical = 4.dp),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            Checkbox(checked = deleteLocalFile, onCheckedChange = { deleteLocalFile = it })
                            Text("同时删除本地文件", style = MaterialTheme.typography.bodyMedium)
                        }
                    }
                }
            },
            confirmButton = { TextButton(onClick = { action = null; if (cancelling) vm.cancelDownload(item.id) else vm.removeDownload(item.id, deleteLocalFile) }) { Text(if (cancelling) "取消下载" else "移除", color = MaterialTheme.colorScheme.error) } },
            dismissButton = { TextButton(onClick = { action = null }) { Text("返回") } },
        )
    }
}

@Composable
private fun DownloadRow(item: DeviceDownload, onRetry: () -> Unit, onCancel: () -> Unit, onRemove: () -> Unit) {
    val context = LocalContext.current
    var menu by remember(item.id) { mutableStateOf(false) }
    val icon = when { item.source.archive || item.mimeType.substringBefore(';') == "application/zip" -> "archive"; item.mimeType.startsWith("video/") -> "play"; item.mimeType.startsWith("image/") -> "image"; item.mimeType.startsWith("audio/") -> "music"; else -> "document" }
    Row(Modifier.fillMaxWidth().padding(start = 12.dp, end = 4.dp, top = 10.dp, bottom = 10.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
        Box(Modifier.size(42.dp).background(MaterialTheme.colorScheme.surfaceContainerLow, RoundedCornerShape(10.dp)), contentAlignment = Alignment.Center) {
            ArcoIcon(icon, size = 22.dp, tint = MaterialTheme.colorScheme.primary)
        }
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(3.dp)) {
            Text(item.name, style = MaterialTheme.typography.bodyMedium, maxLines = 1, overflow = TextOverflow.Ellipsis)
            val details = if (item.active) buildString {
                append(formatBytes(item.downloadedBytes))
                if (item.totalBytes > 0) append(" / ${formatBytes(item.totalBytes)}")
                if (item.bytesPerSecond > 0) append(" · ${formatBytes(item.bytesPerSecond)}/s")
            } else buildString {
                if (item.totalBytes > 0) append("${formatBytes(item.totalBytes)} · ")
                append(Instant.ofEpochMilli(item.createdAt).atZone(ZoneId.systemDefault()).format(DateTimeFormatter.ofPattern("MM-dd HH:mm")))
            }
            Text(details, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                Text(item.statusLabel, style = MaterialTheme.typography.labelSmall, color = if (item.failed) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant)
                if (item.active && item.progress != null) Text("${(requireNotNull(item.progress) * 100).toInt()}%", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.primary)
            }
            if (item.active) {
                val progress = item.progress
                if (progress == null) LinearProgressIndicator(Modifier.fillMaxWidth().height(2.dp))
                else LinearProgressIndicator(progress = { progress }, modifier = Modifier.fillMaxWidth().height(2.dp))
            }
        }
        if (item.active) ArcoIconButton("close", "取消下载", onClick = onCancel)
        else {
            if (item.completed) TextButton(onClick = { openDownload(context, item, share = false) }, contentPadding = PaddingValues(horizontal = 8.dp)) { Text("打开") }
            else ArcoIconButton("refresh", "重新下载", onClick = onRetry)
            Box {
                ArcoIconButton("more", "更多下载操作") { menu = true }
                DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                    if (item.completed) DropdownMenuItem(text = { Text("分享文件") }, onClick = { menu = false; openDownload(context, item, share = true) })
                    if (item.completed) DropdownMenuItem(text = { Text("重新下载") }, onClick = { menu = false; onRetry() })
                    DropdownMenuItem(text = { Text("移除记录") }, onClick = { menu = false; onRemove() })
                }
            }
        }
    }
}

private fun openDownload(context: Context, item: DeviceDownload, share: Boolean) {
    if (item.localUri.isBlank()) {
        Toast.makeText(context, "文件已被移除", Toast.LENGTH_SHORT).show()
        return
    }
    val uri = Uri.parse(item.localUri)
    runCatching {
        context.contentResolver.openFileDescriptor(uri, "r")?.use { } ?: error("文件不可用")
        val intent = if (share) Intent(Intent.ACTION_SEND).apply {
            type = item.mimeType.substringBefore(';')
            putExtra(Intent.EXTRA_STREAM, uri)
            clipData = ClipData.newRawUri(item.name, uri)
        } else Intent(Intent.ACTION_VIEW).setDataAndType(uri, item.mimeType.substringBefore(';'))
        intent.addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
        context.startActivity(Intent.createChooser(intent, if (share) "分享文件" else "打开文件"))
    }.onFailure { Toast.makeText(context, "文件不可用或没有可打开的应用", Toast.LENGTH_SHORT).show() }
}
