package com.buleng.luxuryvideo.ui

import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.util.LruCache
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsPressedAsState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.defaultMinSize
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.buleng.luxuryvideo.ui.theme.ArcoGreen
import com.buleng.luxuryvideo.ui.theme.ArcoOrange
import com.buleng.luxuryvideo.ui.theme.LocalMotionEnabled
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.sync.Semaphore
import kotlinx.coroutines.sync.withPermit
import kotlinx.coroutines.withContext
import java.net.HttpURLConnection
import java.net.URI
import compose.icons.FeatherIcons
import compose.icons.feathericons.*

@Composable
fun ArcoCard(
    modifier: Modifier = Modifier,
    padding: Dp = 18.dp,
    content: @Composable ColumnScope.() -> Unit,
) {
    Surface(
        modifier = modifier,
        color = MaterialTheme.colorScheme.surface,
        shape = RoundedCornerShape(14.dp),
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant.copy(alpha = .75f)),
    ) {
        Column(Modifier.padding(padding), verticalArrangement = Arrangement.spacedBy(12.dp), content = content)
    }
}

@Composable
fun EnterContent(index: Int = 0, modifier: Modifier = Modifier, content: @Composable () -> Unit) {
    val motion = LocalMotionEnabled.current
    if (!motion || index != 0) {
        Box(modifier) { content() }
        return
    }
    var visible by remember { mutableStateOf(false) }
    val opacity = animateFloatAsState(if (visible) 1f else 0f, tween(160), label = "content fade")
    LaunchedEffect(Unit) { visible = true }
    Box(modifier.graphicsLayer { alpha = opacity.value }) { content() }
}

enum class ArcoButtonKind { Primary, Secondary, Danger }

@Composable
fun ArcoButton(
    text: String,
    modifier: Modifier = Modifier,
    icon: String? = null,
    enabled: Boolean = true,
    loading: Boolean = false,
    kind: ArcoButtonKind = ArcoButtonKind.Primary,
    onClick: () -> Unit,
) {
    val interaction = remember { MutableInteractionSource() }
    val pressed by interaction.collectIsPressedAsState()
    val motion = LocalMotionEnabled.current
    val scale = animateFloatAsState(
        if (pressed && motion) .98f else 1f,
        tween(if (motion) 100 else 0),
        label = "button press",
    )
    val buttonModifier = modifier.graphicsLayer {
        scaleX = scale.value
        scaleY = scale.value
    }.defaultMinSize(minHeight = 48.dp)
    val content: @Composable androidx.compose.foundation.layout.RowScope.() -> Unit = {
        if (loading) CircularProgressIndicator(Modifier.size(17.dp), strokeWidth = 2.dp, color = androidx.compose.material3.LocalContentColor.current)
        else if (icon != null) ArcoIcon(icon, size = 18.dp, tint = androidx.compose.material3.LocalContentColor.current)
        if (loading || icon != null) Spacer(Modifier.width(8.dp))
        Text(text, style = MaterialTheme.typography.labelLarge)
    }
    if (kind == ArcoButtonKind.Secondary) {
        OutlinedButton(
            onClick = onClick, modifier = buttonModifier, enabled = enabled && !loading,
            interactionSource = interaction, shape = RoundedCornerShape(10.dp),
            border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
            colors = ButtonDefaults.outlinedButtonColors(contentColor = MaterialTheme.colorScheme.onSurface),
            contentPadding = PaddingValues(horizontal = 16.dp, vertical = 12.dp), content = content,
        )
    } else {
        Button(
            onClick = onClick, modifier = buttonModifier, enabled = enabled && !loading,
            interactionSource = interaction, shape = RoundedCornerShape(10.dp),
            colors = ButtonDefaults.buttonColors(
                containerColor = if (kind == ArcoButtonKind.Danger) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.primary,
                contentColor = if (kind == ArcoButtonKind.Danger) MaterialTheme.colorScheme.onError else MaterialTheme.colorScheme.onPrimary,
            ),
            contentPadding = PaddingValues(horizontal = 16.dp, vertical = 12.dp), content = content,
        )
    }
}

@Composable
fun ArcoIconButton(icon: String, label: String, enabled: Boolean = true, onClick: () -> Unit) {
    IconButton(onClick = onClick, enabled = enabled, modifier = Modifier.size(48.dp).semantics { contentDescription = label }) {
        ArcoIcon(icon, tint = if (enabled) MaterialTheme.colorScheme.onSurfaceVariant else MaterialTheme.colorScheme.outline)
    }
}

@Composable
fun SectionHeading(title: String, subtitle: String? = null, trailing: (@Composable () -> Unit)? = null) {
    Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(3.dp)) {
            Text(title, style = MaterialTheme.typography.titleMedium)
            if (!subtitle.isNullOrBlank()) Text(subtitle, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        trailing?.invoke()
    }
}

@Composable
fun InfoBanner(text: String, error: Boolean = false, icon: String = if (error) "alert" else "info") {
    val colors = MaterialTheme.colorScheme
    val tone = if (error) colors.error else colors.primary
    Row(
        Modifier.fillMaxWidth().clip(RoundedCornerShape(10.dp)).background(if (error) colors.errorContainer else colors.primaryContainer)
            .padding(13.dp),
        horizontalArrangement = Arrangement.spacedBy(9.dp),
        verticalAlignment = Alignment.Top,
    ) {
        ArcoIcon(icon, size = 18.dp, tint = tone)
        Text(text, style = MaterialTheme.typography.bodySmall, color = if (error) colors.onErrorContainer else colors.onPrimaryContainer)
    }
}

@Composable
fun ArcoTag(text: String, color: Color = MaterialTheme.colorScheme.primary, modifier: Modifier = Modifier) {
    Text(
        text, modifier.clip(RoundedCornerShape(5.dp)).background(color.copy(alpha = .09f)).padding(horizontal = 8.dp, vertical = 4.dp),
        color = color, style = MaterialTheme.typography.labelMedium,
    )
}

fun statusLabel(status: String): String = when (status) {
    "queued" -> "排队中"
    "resolving" -> "正在解析"
    "downloading" -> "下载媒体"
    "extracting" -> "提取音频"
    "transcribing" -> "识别文案"
    "postprocessing" -> "生成结果"
    "retry_wait" -> "等待重试"
    "completed" -> "已完成"
    "failed" -> "失败"
    "cancelled" -> "已取消"
    "processing" -> "处理中"
    else -> status
}

fun actionLabel(action: String): String = when (action) {
    "info" -> "仅解析"
    "download" -> "下载媒体"
    "transcribe" -> "文案转写"
    "full" -> "文案转写"
    else -> action
}

@Composable
fun StatusBadge(status: String) {
    val color = when (status) {
        "completed" -> ArcoGreen
        "failed" -> MaterialTheme.colorScheme.error
        "cancelled" -> MaterialTheme.colorScheme.onSurfaceVariant
        "retry_wait" -> ArcoOrange
        else -> MaterialTheme.colorScheme.primary
    }
    Row(
        Modifier.clip(RoundedCornerShape(6.dp)).background(color.copy(alpha = .08f)).padding(horizontal = 9.dp, vertical = 5.dp),
        horizontalArrangement = Arrangement.spacedBy(5.dp), verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(Modifier.size(5.dp).background(color, CircleShape))
        Text(statusLabel(status), style = MaterialTheme.typography.labelMedium, color = color)
    }
}

@Composable
fun EmptyState(title: String, message: String, icon: String = "document", action: (@Composable () -> Unit)? = null) {
    Column(
        Modifier.fillMaxWidth().padding(horizontal = 20.dp, vertical = 28.dp),
        horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Box(Modifier.size(48.dp).background(MaterialTheme.colorScheme.primaryContainer, RoundedCornerShape(12.dp)), contentAlignment = Alignment.Center) {
            ArcoIcon(icon, size = 24.dp, tint = MaterialTheme.colorScheme.primary)
        }
        Spacer(Modifier.height(1.dp))
        Text(title, style = MaterialTheme.typography.titleMedium)
        Text(message, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, textAlign = androidx.compose.ui.text.style.TextAlign.Center)
        action?.invoke()
    }
}

@Composable
fun LoadingCards(count: Int = 3) {
    val base = MaterialTheme.colorScheme.surfaceContainerHigh
    Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
        repeat(count) {
            ArcoCard {
                Canvas(Modifier.fillMaxWidth().height(64.dp)) {
                    drawRoundRect(base, size = Size(size.width * .7f, 14.dp.toPx()), cornerRadius = CornerRadius(5.dp.toPx()))
                    drawRoundRect(base, topLeft = Offset(0f, 30.dp.toPx()), size = Size(size.width * .94f, 10.dp.toPx()), cornerRadius = CornerRadius(4.dp.toPx()))
                    drawRoundRect(base, topLeft = Offset(0f, 49.dp.toPx()), size = Size(size.width * .45f, 10.dp.toPx()), cornerRadius = CornerRadius(4.dp.toPx()))
                }
            }
        }
    }
}

fun mediaUrl(server: String, value: String): String = runCatching {
    if (value.isBlank()) "" else URI(server.trimEnd('/') + "/").resolve(value).toString()
}.getOrDefault("")

private val imageLoadSlots = Semaphore(3)

private val imageCache = object : LruCache<String, Bitmap>(20 * 1024 * 1024) {
    override fun sizeOf(key: String, value: Bitmap): Int = value.allocationByteCount
}

@Composable
fun RemoteImage(
    url: String,
    modifier: Modifier = Modifier,
    description: String? = null,
    contentScale: ContentScale = ContentScale.Crop,
    onError: (() -> Unit)? = null,
    maxDimension: Int = 1400,
) {
    val targetSize = maxDimension.coerceIn(64, 4096)
    val cacheKey = "$targetSize:$url"
    val reportError by rememberUpdatedState(onError)
    val bitmap by produceState<Bitmap?>(imageCache.get(cacheKey), cacheKey) {
        value = imageCache.get(cacheKey)
        if (url.isBlank()) {
            reportError?.invoke()
            return@produceState
        }
        value = withContext(Dispatchers.IO) {
            imageLoadSlots.withPermit {
                imageCache.get(cacheKey) ?: try {
                    val uri = URI(url)
                    require(uri.scheme in setOf("http", "https"))
                    val connection = uri.toURL().openConnection() as HttpURLConnection
                    try {
                        connection.connectTimeout = 8_000
                        connection.readTimeout = 10_000
                        check(connection.responseCode in 200..299)
                        val bytes = connection.inputStream.use { input ->
                            val output = java.io.ByteArrayOutputStream()
                            val buffer = ByteArray(8192)
                            var total = 0
                            while (true) {
                                currentCoroutineContext().ensureActive()
                                val count = input.read(buffer)
                                if (count == -1) break
                                total += count
                                require(total <= 12 * 1024 * 1024)
                                output.write(buffer, 0, count)
                            }
                            output.toByteArray()
                        }
                        currentCoroutineContext().ensureActive()
                        val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
                        BitmapFactory.decodeByteArray(bytes, 0, bytes.size, bounds)
                        var sample = 1
                        while (bounds.outWidth / sample > targetSize || bounds.outHeight / sample > targetSize) sample *= 2
                        BitmapFactory.decodeByteArray(bytes, 0, bytes.size, BitmapFactory.Options().apply { inSampleSize = sample })
                            ?.also {
                                it.prepareToDraw()
                                imageCache.put(cacheKey, it)
                            }
                    } finally { connection.disconnect() }
                } catch (cancelled: CancellationException) {
                    throw cancelled
                } catch (_: Exception) {
                    null
                } catch (_: OutOfMemoryError) {
                    null
                }
            }
        }
        if (value == null) reportError?.invoke()
    }
    val imageBitmap = remember(bitmap) { bitmap?.asImageBitmap() }
    Box(modifier.background(MaterialTheme.colorScheme.surfaceContainerHigh), contentAlignment = Alignment.Center) {
        if (imageBitmap != null) Image(imageBitmap, description, Modifier.fillMaxSize(), contentScale = contentScale)
        else ArcoIcon("image", tint = MaterialTheme.colorScheme.outline, size = 25.dp)
    }
}

@Composable
fun ArcoIcon(name: String, modifier: Modifier = Modifier, size: Dp = 22.dp, tint: Color = MaterialTheme.colorScheme.onSurfaceVariant) {
    val icon = when (name) {
        "brand", "video" -> FeatherIcons.Video
        "play" -> FeatherIcons.Play
        "pause" -> FeatherIcons.Pause
        "more" -> FeatherIcons.MoreHorizontal
        "plus" -> FeatherIcons.Plus
        "close" -> FeatherIcons.X
        "clear" -> FeatherIcons.Trash2
        "check" -> FeatherIcons.Check
        "circle" -> FeatherIcons.Circle
        "check_circle" -> FeatherIcons.CheckCircle
        "minus_circle" -> FeatherIcons.MinusCircle
        "right", "chevron" -> FeatherIcons.ChevronRight
        "back" -> FeatherIcons.ArrowLeft
        "arrow" -> FeatherIcons.ArrowRight
        "down" -> FeatherIcons.ChevronDown
        "download" -> FeatherIcons.Download
        "upload" -> FeatherIcons.Upload
        "send" -> FeatherIcons.Send
        "search" -> FeatherIcons.Search
        "history", "clock" -> FeatherIcons.Clock
        "document" -> FeatherIcons.FileText
        "copy" -> FeatherIcons.Copy
        "clipboard" -> FeatherIcons.Clipboard
        "grid", "admin" -> FeatherIcons.Grid
        "user" -> FeatherIcons.User
        "users" -> FeatherIcons.Users
        "shield" -> FeatherIcons.Shield
        "key" -> FeatherIcons.Key
        "lock" -> FeatherIcons.Lock
        "link" -> FeatherIcons.Link
        "external" -> FeatherIcons.ExternalLink
        "image" -> FeatherIcons.Image
        "music" -> FeatherIcons.Music
        "wave" -> FeatherIcons.Mic
        "trash" -> FeatherIcons.Trash2
        "refresh" -> FeatherIcons.RotateCw
        "share" -> FeatherIcons.Share2
        "logout" -> FeatherIcons.LogOut
        "spark" -> FeatherIcons.Zap
        "eye" -> FeatherIcons.Eye
        "bell" -> FeatherIcons.Bell
        "stop" -> FeatherIcons.Square
        "info" -> FeatherIcons.Info
        "alert" -> FeatherIcons.AlertCircle
        "server" -> FeatherIcons.Server
        "filter" -> FeatherIcons.Sliders
        "settings" -> FeatherIcons.Settings
        "sun" -> FeatherIcons.Sun
        "moon" -> FeatherIcons.Moon
        "system", "monitor" -> FeatherIcons.Monitor
        "api", "code" -> FeatherIcons.Code
        "archive" -> FeatherIcons.Archive
        "fullscreen" -> FeatherIcons.Maximize
        "fullscreen_exit" -> FeatherIcons.Minimize
        "replay10" -> FeatherIcons.RotateCcw
        "forward10" -> FeatherIcons.RotateCw
        else -> FeatherIcons.HelpCircle
    }
    Icon(icon, contentDescription = null, modifier = modifier.size(size), tint = tint)
}
