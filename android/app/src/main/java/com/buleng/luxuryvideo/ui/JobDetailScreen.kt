package com.buleng.luxuryvideo.ui

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.net.Uri
import androidx.compose.animation.animateContentSize
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.itemsIndexed
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.nestedscroll.NestedScrollConnection
import androidx.compose.ui.input.nestedscroll.NestedScrollSource
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.Velocity
import com.buleng.luxuryvideo.CaptureUiState
import com.buleng.luxuryvideo.CaptureViewModel
import com.buleng.luxuryvideo.data.CaptureJob
import com.buleng.luxuryvideo.data.JobFile
import com.buleng.luxuryvideo.data.formatBytes
import com.buleng.luxuryvideo.data.formatTime
import com.buleng.luxuryvideo.data.objects
import com.buleng.luxuryvideo.data.text
import com.buleng.luxuryvideo.ui.theme.ArcoGreen
import com.buleng.luxuryvideo.ui.theme.LocalMotionEnabled
import kotlinx.coroutines.delay
import java.net.URI
import java.time.Duration
import java.time.Instant

private data class GalleryMedia(val index: Int, val source: String, val thumbnail: String, val animated: Boolean, val file: JobFile?)

@Composable
fun JobDetailScreen(job: CaptureJob, state: CaptureUiState, vm: CaptureViewModel) {
    val context = LocalContext.current
    val motion = LocalMotionEnabled.current
    var confirmation by remember(job.id) { mutableStateOf<String?>(null) }
    val galleryScrollBoundary = remember {
        object : NestedScrollConnection {
            override fun onPostScroll(consumed: Offset, available: Offset, source: NestedScrollSource): Offset = Offset(0f, available.y)
            override suspend fun onPostFling(consumed: Velocity, available: Velocity): Velocity = Velocity(0f, available.y)
        }
    }
    val admin = state.selectedJobAdmin
    val mediaExpiresAt = job.mediaRetentionExpiresAt
    var mediaExpired by remember(job.id, mediaExpiresAt) { mutableStateOf(job.mediaRetentionExpired) }
    LaunchedEffect(job.id, mediaExpiresAt) {
        if (mediaExpired || mediaExpiresAt.isBlank()) return@LaunchedEffect
        val delayMillis = runCatching { Duration.between(Instant.now(), Instant.parse(mediaExpiresAt)).toMillis() }.getOrNull() ?: return@LaunchedEffect
        delay(delayMillis.coerceAtLeast(0L) + 1L)
        mediaExpired = true
    }
    val work = remember(job.raw) { job.raw.optJSONObject("work") }
    val images = remember(work) { work?.objects("images").orEmpty() }
    val gallery = remember(images, job.files, state.preferences.serverUrl, mediaExpired) {
        images.mapIndexed { index, image ->
            val animated = image.text("animatedUrl").isNotBlank()
            val expectedKind = if (animated) "animated" else "image"
            val prefix = "image-${(index + 1).toString().padStart(2, '0')}."
            val file = job.files.find { it.kind == expectedKind && it.name.startsWith(prefix) }
            val source = if (file != null && !file.isExpired && signedUrlUsable(file.previewUrl)) file.previewUrl else image.text(if (animated) "animatedUrl" else "url")
            GalleryMedia(index, mediaUrl(state.preferences.serverUrl, source), mediaUrl(state.preferences.serverUrl, file?.takeUnless { it.isExpired }?.thumbnailUrl?.takeIf { signedUrlUsable(it) } ?: image.text("url")), animated, file)
        }
    }
    val video = job.files.firstOrNull { it.kind == "video" }
    val coverUrl = job.coverUrl.ifBlank { video?.thumbnailUrl.orEmpty() }
    val music = job.music
    val availableMusic = music.file
    val canUseMusic = availableMusic != null
    val showVideoCard = (coverUrl.isNotBlank() || video != null) && gallery.isEmpty()
    val showMusicCard = availableMusic != null || job.files.any { it.kind == "music" }
    val progress by animateFloatAsState(job.progress / 100f, tween(if (motion) 850 else 0), label = "detail progress")
    PageColumn {
        EnterContent {
            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                Row(Modifier.horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                    StatusBadge(job.status)
                    ArcoTag(actionLabel(job.action), color = MaterialTheme.colorScheme.onSurfaceVariant)
                    if (job.workType == "note") ArcoTag(if (gallery.any { it.animated }) "图文 / 动图" else "图文作品")
                }
                Text(job.title.ifBlank { "任务 ${job.id.take(8)}" }, style = MaterialTheme.typography.headlineSmall)
                Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                    Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(3.dp)) {
                        if (job.authorName.isNotBlank()) Text("@${job.authorName}", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        Text(formatTime(job.createdAt), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                    if (job.canonicalUrl.isNotBlank()) Row(verticalAlignment = Alignment.CenterVertically) {
                        ArcoIconButton("share", "分享作品") {
                            shareText(context, listOf(job.title, job.canonicalUrl).filter { it.isNotBlank() }.joinToString("\n"))
                        }
                        ArcoIconButton("external", "跳转原作品") { confirmation = "external" }
                    }
                }
                if (mediaExpiresAt.isNotBlank()) InfoBanner(
                    if (mediaExpired) "服务器媒体已到期（${formatTime(mediaExpiresAt)}），任务文案仍可查看。"
                    else "服务器媒体保留至 ${formatTime(mediaExpiresAt)}，到期后自动清理。任务文案仍可在历史中查看。",
                    error = mediaExpired,
                )
                if (admin) InfoBanner("正在以管理员身份查看任务。", icon = "shield")
            }
        }
        if (!job.terminal || job.errorMessage.isNotBlank()) EnterContent(1) {
            ArcoCard {
                Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                    Column(Modifier.weight(1f)) {
                        Text(statusLabel(job.status), style = MaterialTheme.typography.titleMedium)
                        if (job.statusMessage.isNotBlank()) Text(job.statusMessage, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                    Text("${job.progress}%", style = MaterialTheme.typography.headlineSmall, color = MaterialTheme.colorScheme.primary)
                }
                LinearProgressIndicator(progress = { progress.coerceIn(0f, 1f) }, modifier = Modifier.fillMaxWidth().height(6.dp), color = MaterialTheme.colorScheme.primary, trackColor = MaterialTheme.colorScheme.primaryContainer)
                JobTimeline(job)
                if (job.errorMessage.isNotBlank()) InfoBanner("${job.errorMessage}${if (job.errorCode.isBlank()) "" else "\n错误代码：${job.errorCode}"}", error = true)
                if (!job.terminal) Text("进度由服务器同步；离开页面后任务仍会继续。", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
        if (showVideoCard) EnterContent(1) {
            ArcoCard(modifier = Modifier.fillMaxWidth(), padding = 0.dp) {
                Box(
                    Modifier.fillMaxWidth().height(240.dp).background(Color.Black)
                        .then(if (video != null && !video.isExpired) Modifier.clickable { vm.preview(video) } else Modifier),
                    contentAlignment = Alignment.Center,
                ) {
                    if (coverUrl.isNotBlank()) RemoteImage(mediaUrl(state.preferences.serverUrl, coverUrl), Modifier.fillMaxSize(), "作品封面", contentScale = ContentScale.Crop)
                    if (video != null && !video.isExpired) Box(Modifier.size(58.dp).background(Color.Black.copy(alpha = .48f), CircleShape), contentAlignment = Alignment.Center) { ArcoIcon("play", size = 25.dp, tint = Color.White) }
                }
                if (video != null) Row(Modifier.fillMaxWidth().padding(start = 16.dp, end = 8.dp, bottom = 10.dp), verticalAlignment = Alignment.CenterVertically) {
                    Column(Modifier.weight(1f)) {
                        Text("视频预览", style = MaterialTheme.typography.titleSmall)
                        Text(if (video.isExpired) "服务器媒体已到期" else formatBytes(video.sizeBytes), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        ArcoIconButton("play", "播放视频", enabled = !video.isExpired) { vm.preview(video) }
                        ArcoIconButton("download", "下载视频", enabled = !video.isExpired && !state.busy) { vm.download(video, admin) }
                    }
                }
            }
        }
        if (gallery.isNotEmpty()) EnterContent(2) {
            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                SectionHeading("作品画廊", "${gallery.size} 项")
                val galleryItem: @Composable (Int, GalleryMedia) -> Unit = { index, media ->
                    Surface(color = MaterialTheme.colorScheme.surface, shape = RoundedCornerShape(13.dp)) {
                        Column(Modifier.fillMaxWidth()) {
                            Box(
                                Modifier.fillMaxWidth().aspectRatio(1f).clickable {
                                    vm.previewGallery(gallery.mapIndexed { galleryIndex, item ->
                                        item.file?.takeUnless { it.isExpired } ?: previewOnlyFile(
                                            if (item.animated) "animated" else "image",
                                            "${if (item.animated) "动图" else "作品图片"} ${galleryIndex + 1}",
                                            item.source,
                                        ).copy(thumbnailUrl = item.thumbnail)
                                    }, index)
                                },
                                contentAlignment = Alignment.Center,
                            ) {
                                RemoteImage(media.thumbnail, Modifier.fillMaxSize(), "第 ${index + 1} 张作品图片")
                                if (media.animated) Box(Modifier.size(38.dp).background(Color.Black.copy(alpha = .45f), CircleShape), contentAlignment = Alignment.Center) { ArcoIcon("play", size = 18.dp, tint = Color.White) }
                            }
                            Row(Modifier.fillMaxWidth().padding(start = 12.dp, end = 3.dp), verticalAlignment = Alignment.CenterVertically) {
                                Text("${if (media.animated) "动图" else "图片"} ${index + 1}", style = MaterialTheme.typography.labelMedium, modifier = Modifier.weight(1f))
                                if (media.file?.isExpired == true || (media.file == null && mediaExpired)) {
                                    ArcoTag("已到期", color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(vertical = 12.dp))
                                } else if (media.file != null) {
                                    ArcoIconButton("download", "下载第 ${index + 1} 项", enabled = !state.busy) { vm.download(media.file, admin) }
                                } else ArcoTag("未保存", color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(vertical = 12.dp))
                            }
                        }
                    }
                }
                if (gallery.size <= 2) {
                    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                        gallery.forEachIndexed { index, media -> Box(Modifier.weight(1f)) { galleryItem(index, media) } }
                        if (gallery.size == 1) Spacer(Modifier.weight(1f))
                    }
                } else {
                    LazyVerticalGrid(
                        columns = GridCells.Fixed(2),
                        modifier = Modifier.fillMaxWidth().height(420.dp).nestedScroll(galleryScrollBoundary),
                        horizontalArrangement = Arrangement.spacedBy(10.dp),
                        verticalArrangement = Arrangement.spacedBy(10.dp),
                    ) {
                        itemsIndexed(gallery, key = { _, item -> item.index }) { index, media -> galleryItem(index, media) }
                    }
                }
                val savedCount = gallery.count { it.file != null && !it.file.isExpired }
                if (job.action != "info" && job.status == "completed") Text(
                    when {
                        mediaExpired && savedCount > 0 -> "媒体已到期，当前可下载 $savedCount / ${gallery.size} 项"
                        mediaExpired -> "媒体已到期"
                        else -> "可下载 $savedCount / ${gallery.size} 项"
                    },
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                if (savedCount > 0 && job.status == "completed") ArcoButton("打包下载 ZIP", icon = "archive", kind = ArcoButtonKind.Secondary, loading = state.busy) { vm.downloadArchive(job, admin) }
            }
        }
        if (showMusicCard) EnterContent(2) {
            ArcoCard(padding = 16.dp) {
                Row(horizontalArrangement = Arrangement.spacedBy(12.dp), verticalAlignment = Alignment.CenterVertically) {
                    Box(Modifier.size(45.dp).background(MaterialTheme.colorScheme.tertiaryContainer, RoundedCornerShape(12.dp)), contentAlignment = Alignment.Center) { ArcoIcon("music", tint = MaterialTheme.colorScheme.tertiary) }
                    Column(Modifier.weight(1f)) {
                        Text(job.musicTitle.ifBlank { "作品原声" }, style = MaterialTheme.typography.titleSmall, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        Text(job.musicArtist.ifBlank { "原声 / 背景音乐" }, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        ArcoIconButton("play", "播放原声", enabled = canUseMusic) {
                            availableMusic?.let { vm.preview(it) }
                        }
                        ArcoIconButton("download", "下载原声", enabled = canUseMusic && !state.busy && !state.downloadPreparing) {
                            availableMusic?.let { file ->
                                if (music.source == "server") vm.download(file, admin)
                                else vm.downloadSource(file.previewUrl, "${job.musicTitle.ifBlank { "作品原声" }}.mp3", "audio/mpeg")
                            }
                        }
                    }
                }
            }
        }
        if (job.description.isNotBlank()) EnterContent(3) { ResultTextCard("发布文案", job.description) }
        if (job.transcript.isNotBlank() && job.transcript != job.description) EnterContent(4) { ResultTextCard(if (job.workType == "note") "图文配文" else "转写文案", job.transcript) }
        if (job.rawText.isNotBlank() && job.rawText != job.transcript) EnterContent(5) { ResultTextCard("原始识别结果", job.rawText, collapsed = true) }
        val tags = work?.optJSONArray("hashtags")
        if (tags != null && tags.length() > 0) Row(Modifier.horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(7.dp)) {
            repeat(tags.length()) { index -> ArcoTag("#${tags.optString(index)}", color = MaterialTheme.colorScheme.onSurfaceVariant) }
        }
        val fileResults = job.files.filter {
            it.kind !in setOf("image", "animated") &&
                !(showVideoCard && it.id == video?.id) && !(showMusicCard && it.kind == "music")
        }
        if (fileResults.isNotEmpty()) EnterContent(5) {
            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                SectionHeading("结果文件", "${fileResults.size} 项")
                Surface(color = MaterialTheme.colorScheme.surface, shape = RoundedCornerShape(15.dp)) {
                    Column {
                        fileResults.forEachIndexed { index, file ->
                            if (index > 0) HorizontalDivider(modifier = Modifier.padding(start = 54.dp), color = MaterialTheme.colorScheme.outlineVariant.copy(alpha = .65f), thickness = .5.dp)
                            Row(Modifier.fillMaxWidth().padding(start = 14.dp, end = 4.dp, top = 4.dp, bottom = 4.dp), horizontalArrangement = Arrangement.spacedBy(10.dp), verticalAlignment = Alignment.CenterVertically) {
                                Box(Modifier.size(30.dp).background(MaterialTheme.colorScheme.surfaceContainerHigh, RoundedCornerShape(7.dp)), contentAlignment = Alignment.Center) { ArcoIcon(fileIcon(file.kind), size = 17.dp) }
                                Column(Modifier.weight(1f)) {
                                    Text(fileLabel(file.kind), style = MaterialTheme.typography.bodyMedium)
                                    Text(file.name, maxLines = 1, overflow = TextOverflow.Ellipsis, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                    Text(if (file.isExpired) "已到期" else formatBytes(file.sizeBytes), style = MaterialTheme.typography.labelSmall, color = if (file.isExpired) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant)
                                }
                                Row(verticalAlignment = Alignment.CenterVertically) {
                                    if (file.kind in setOf("video", "music")) ArcoIconButton("play", "预览${file.name}", enabled = !file.isExpired) { vm.preview(file) }
                                    ArcoIconButton("download", "下载${file.name}", enabled = !file.isExpired && !state.busy) { vm.download(file, admin) }
                                }
                            }
                        }
                    }
                }
            }
        }
        EnterContent(6) {
            ArcoCard(padding = 16.dp) {
                SectionHeading("任务信息")
                MetaLine("任务编号", job.id, copyable = true)
                if (job.asrProvider.isNotBlank()) MetaLine("转写服务", providerName(job.asrProvider))
                if (work != null) {
                    val width = work.optInt("width")
                    val height = work.optInt("height")
                    if (width > 0 && height > 0) MetaLine("作品分辨率", "$width × $height")
                    val duration = work.optLong("durationMs") / 1000
                    if (duration > 0) MetaLine("作品时长", "${duration / 60}:${(duration % 60).toString().padStart(2, '0')}")
                    if (work.text("publishedAt").isNotBlank()) MetaLine("发布时间", formatTime(work.text("publishedAt")))
                }
                MetaLine("最近更新", formatTime(job.updatedAt))
            }
        }
        Row(Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            if (job.canCancel) ArcoButton("取消任务", icon = "stop", kind = ArcoButtonKind.Secondary, enabled = !state.busy) { confirmation = "cancel" }
            if (job.canRetry) ArcoButton("重新尝试", icon = "refresh", enabled = !state.busy) { vm.jobAction(job.id, "retry", admin) }
            if (job.terminal) ArcoButton("删除任务", icon = "trash", kind = ArcoButtonKind.Danger, enabled = !state.busy) { confirmation = "delete" }
        }
        Spacer(Modifier.height(8.dp))
    }
    if (confirmation == "external") {
        AlertDialog(
            onDismissRequest = { confirmation = null },
            title = { Text("跳转原作品？") },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text("将通过浏览器或其他应用打开原作品链接。")
                    Text(job.canonicalUrl, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 4, overflow = TextOverflow.Ellipsis)
                }
            },
            confirmButton = { TextButton(onClick = {
                confirmation = null
                runCatching {
                    val uri = Uri.parse(job.canonicalUrl)
                    require(uri.scheme.equals("https", true) || uri.scheme.equals("http", true))
                    context.startActivity(Intent(Intent.ACTION_VIEW, uri))
                }
            }) { Text("确认跳转") } },
            dismissButton = { TextButton(onClick = { confirmation = null }) { Text("取消") } },
        )
    } else if (confirmation != null) {
        val deleting = confirmation == "delete"
        AlertDialog(
            onDismissRequest = { confirmation = null },
            title = { Text(if (deleting) "删除这个任务？" else "取消当前任务？") },
            text = { Text(if (deleting) "任务记录和服务器上的相关文件会一起删除，此操作无法撤销。" else "服务器将停止这个任务，之后可以在任务详情重新尝试。") },
            confirmButton = { TextButton(onClick = { val operation = confirmation!!; confirmation = null; vm.jobAction(job.id, operation, admin) }) { Text(if (deleting) "确认删除" else "取消任务", color = MaterialTheme.colorScheme.error) } },
            dismissButton = { TextButton(onClick = { confirmation = null }) { Text("返回") } },
        )
    }
}

@Composable
private fun JobTimeline(job: CaptureJob) {
    val stages = when (job.action) {
        "info" -> listOf("queued", "resolving", "completed")
        "download" -> listOf("queued", "resolving", "downloading", "completed")
        else -> listOf("queued", "resolving", "downloading", "extracting", "transcribing", "postprocessing", "completed")
    }
    val current = stages.indexOf(job.status)
    Row(Modifier.horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
        stages.forEachIndexed { index, stage ->
            if (index > 0) Box(Modifier.width(15.dp).height(1.dp).background(MaterialTheme.colorScheme.outlineVariant))
            val done = job.status == "completed" || current > index
            val active = current == index
            Row(horizontalArrangement = Arrangement.spacedBy(5.dp), verticalAlignment = Alignment.CenterVertically) {
                val color = if (done) ArcoGreen else if (active) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.outline
                Box(Modifier.size(17.dp).background(color.copy(alpha = .1f), CircleShape), contentAlignment = Alignment.Center) {
                    if (done) ArcoIcon("check", size = 11.dp, tint = color)
                    else Box(Modifier.size(if (active) 6.dp else 4.dp).background(color, CircleShape))
                }
                Text(when (stage) { "queued" -> "排队"; "resolving" -> "解析"; "downloading" -> "下载"; "extracting" -> "音频"; "transcribing" -> "转写"; "postprocessing" -> "整理"; else -> "完成" }, style = MaterialTheme.typography.labelSmall, color = color)
            }
        }
    }
}

@Composable
fun ResultTextCard(title: String, text: String, collapsed: Boolean = false) {
    val context = LocalContext.current
    val motion = LocalMotionEnabled.current
    var expanded by remember(text) { mutableStateOf(!collapsed && text.length < 1200) }
    ArcoCard(modifier = Modifier.animateContentSize(tween(if (motion) 280 else 0))) {
        SectionHeading(title, "${text.length} 字") {
            Row {
                ArcoIconButton("copy", "复制$title") { copyText(context, title, text) }
                ArcoIconButton("send", "分享$title") { shareText(context, text) }
            }
        }
        SelectionContainer {
            Text(text, style = MaterialTheme.typography.bodyLarge, maxLines = if (expanded) Int.MAX_VALUE else 7, overflow = TextOverflow.Ellipsis)
        }
        if (text.length > 300 || collapsed) TextButton(onClick = { expanded = !expanded }) { Text(if (expanded) "收起内容" else "展开全文") }
    }
}

@Composable
private fun MetaLine(label: String, value: String, copyable: Boolean = false) {
    val context = LocalContext.current
    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(12.dp), verticalAlignment = Alignment.CenterVertically) {
        Text(label, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.width(75.dp))
        Text(value, modifier = Modifier.weight(1f), style = MaterialTheme.typography.bodySmall)
        if (copyable) ArcoIconButton("copy", "复制$label") { copyText(context, label, value) }
    }
}

private fun previewOnlyFile(kind: String, name: String, source: String) = JobFile(
    id = "", kind = kind, name = name, mimeType = when (kind) { "music" -> "audio/mpeg"; "animated" -> "video/mp4"; else -> "image/jpeg" },
    sizeBytes = 0L, expiresAt = "", previewUrl = source, thumbnailUrl = "",
)

private fun signedUrlUsable(value: String): Boolean = runCatching {
    if (value.isBlank()) false else {
        val expires = URI(value).rawQuery.orEmpty().split('&').firstOrNull { it.startsWith("expires=") }?.substringAfter('=')?.toLongOrNull()
        expires == null || expires > System.currentTimeMillis() / 1000 + 10
    }
}.getOrDefault(false)

private fun fileIcon(kind: String) = when (kind) { "video", "animated" -> "video"; "image" -> "image"; "music" -> "music"; else -> "document" }
private fun fileLabel(kind: String) = when (kind) { "video" -> "作品视频"; "animated" -> "动态图片"; "image" -> "作品原图"; "music" -> "背景音乐"; "result_text" -> "TXT 文案"; "result_markdown" -> "Markdown 文案"; "result_meta" -> "作品元数据 JSON"; else -> "结果文件" }

fun copyText(context: Context, label: String, text: String) {
    (context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager).setPrimaryClip(ClipData.newPlainText(label, text))
}

fun shareText(context: Context, text: String) {
    runCatching { context.startActivity(Intent.createChooser(Intent(Intent.ACTION_SEND).apply { type = "text/plain"; putExtra(Intent.EXTRA_TEXT, text) }, "分享内容")) }
}
