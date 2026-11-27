package com.buleng.luxuryvideo.platform

import android.app.Activity
import android.content.Context
import android.content.pm.ActivityInfo
import android.content.res.ColorStateList
import android.content.ContextWrapper
import android.graphics.Bitmap
import android.graphics.ImageDecoder
import android.graphics.SurfaceTexture
import android.graphics.drawable.ClipDrawable
import android.graphics.drawable.GradientDrawable
import android.graphics.drawable.LayerDrawable
import android.net.Uri
import android.os.Handler
import android.os.HandlerThread
import android.os.Looper
import android.view.Gravity
import android.view.Surface
import android.view.TextureView
import android.widget.SeekBar
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.foundation.gestures.awaitFirstDown
import androidx.compose.foundation.gestures.calculateCentroid
import androidx.compose.foundation.gestures.calculatePan
import androidx.compose.foundation.gestures.calculateZoom
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.pager.HorizontalPager
import androidx.compose.foundation.pager.rememberPagerState
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clipToBounds
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.input.pointer.positionChanged
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.IntSize
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.compose.ui.window.DialogWindowProvider
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat
import androidx.annotation.OptIn
import androidx.media3.common.AudioAttributes
import androidx.media3.common.C
import androidx.media3.common.MediaItem
import androidx.media3.common.PlaybackException
import androidx.media3.common.Player
import androidx.media3.common.VideoSize
import androidx.media3.common.util.UnstableApi
import androidx.media3.datasource.BaseDataSource
import androidx.media3.datasource.DataSource
import androidx.media3.datasource.DataSpec
import androidx.media3.exoplayer.DefaultLoadControl
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.exoplayer.source.DefaultMediaSourceFactory
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.LifecycleOwner
import com.buleng.luxuryvideo.NativeEvent
import com.buleng.luxuryvideo.PreviewItem
import com.buleng.luxuryvideo.data.formatBytes
import com.buleng.luxuryvideo.ui.ArcoIcon
import com.buleng.luxuryvideo.ui.RemoteImage
import com.buleng.luxuryvideo.ui.theme.LocalMotionEnabled
import java.io.ByteArrayOutputStream
import java.io.IOException
import java.io.EOFException
import java.io.InputStream
import java.net.HttpURLConnection
import java.net.SocketTimeoutException
import java.net.URI
import java.nio.ByteBuffer
import java.util.Locale
import kotlin.math.abs
import kotlin.math.sin
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.delay
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

@Composable
fun MediaPreview(event: NativeEvent.Preview, onDismiss: () -> Unit, onRetry: (NativeEvent.Preview) -> Unit, onDownload: (NativeEvent.Preview) -> Unit) {
    val gallery = remember(event) { event.gallery.ifEmpty { listOf(PreviewItem(event.file, event.url, event.headers)) } }
    val initialPage = event.galleryIndex.coerceIn(0, gallery.lastIndex)
    val pager = rememberPagerState(initialPage = initialPage) { gallery.size }
    val scope = rememberCoroutineScope()
    var zoomedPage by remember(event) { mutableStateOf<Int?>(null) }
    var fullscreen by remember { mutableStateOf(false) }
    val showGalleryNavigation = gallery.size > 1 && !fullscreen
    val context = LocalContext.current
    val activity = remember(context) { context.previewActivity() }
    val originalOrientation = remember(activity) { activity?.requestedOrientation }
    LaunchedEffect(fullscreen, activity) {
        activity?.requestedOrientation = if (fullscreen) ActivityInfo.SCREEN_ORIENTATION_SENSOR_LANDSCAPE
            else originalOrientation ?: ActivityInfo.SCREEN_ORIENTATION_UNSPECIFIED
    }
    DisposableEffect(activity) {
        onDispose { if (originalOrientation != null) activity?.requestedOrientation = originalOrientation }
    }
    LaunchedEffect(pager.settledPage) { fullscreen = false; zoomedPage = null }
    Dialog(
        onDismissRequest = { if (fullscreen) fullscreen = false else onDismiss() },
        properties = DialogProperties(
            dismissOnBackPress = true,
            dismissOnClickOutside = false,
            usePlatformDefaultWidth = false,
            decorFitsSystemWindows = false,
        ),
    ) {
        val view = LocalView.current
        LaunchedEffect(view, fullscreen) {
            val window = (view.parent as? DialogWindowProvider)?.window
            window?.let {
                val controller = WindowCompat.getInsetsController(it, view)
                controller.systemBarsBehavior = WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
                if (fullscreen) controller.hide(WindowInsetsCompat.Type.systemBars())
                else controller.show(WindowInsetsCompat.Type.systemBars())
            }
        }
        DisposableEffect(view) {
            val window = (view.parent as? DialogWindowProvider)?.window
            val controller = window?.let { WindowCompat.getInsetsController(it, view) }
            val lightStatus = controller?.isAppearanceLightStatusBars
            val lightNavigation = controller?.isAppearanceLightNavigationBars
            controller?.isAppearanceLightStatusBars = false
            controller?.isAppearanceLightNavigationBars = false
            onDispose {
                controller?.show(WindowInsetsCompat.Type.systemBars())
                if (lightStatus != null) controller?.isAppearanceLightStatusBars = lightStatus
                if (lightNavigation != null) controller?.isAppearanceLightNavigationBars = lightNavigation
            }
        }
        Surface(modifier = Modifier.fillMaxSize(), color = Color.Black) {
            Column(Modifier.fillMaxSize()) {
                HorizontalPager(
                    state = pager,
                    modifier = Modifier.weight(1f).fillMaxWidth(),
                    userScrollEnabled = !fullscreen && zoomedPage == null,
                ) { index ->
                    val item = gallery[index]
                    val page = event.copy(file = item.file, url = item.url, headers = item.headers, gallery = gallery, galleryIndex = index)
                    var attempt by remember(item.url, item.headers) { mutableIntStateOf(0) }
                    val retry = { attempt++; onRetry(page) }
                    val download = { onDownload(page) }
                    val audio = item.file.mimeType.startsWith("audio/") || item.file.kind == "music"
                    val video = item.file.mimeType.startsWith("video/") || item.file.kind in setOf("video", "animated")
                    key(item.url, item.headers, attempt) {
                        if (audio || video) {
                            if (index == pager.settledPage) {
                                PlayerPreview(page, audio, retry, onDismiss, download, fullscreen, !showGalleryNavigation) { fullscreen = !fullscreen }
                            } else {
                                Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                                    if (item.file.thumbnailUrl.isNotBlank()) RemoteImage(item.file.thumbnailUrl, Modifier.fillMaxSize(), item.file.name, contentScale = ContentScale.Fit)
                                    ArcoIcon(if (audio) "music" else "play", size = 42.dp, tint = Color.White)
                                }
                            }
                        } else {
                            Column(Modifier.fillMaxSize().statusBarsPadding().then(if (showGalleryNavigation) Modifier else Modifier.navigationBarsPadding())) {
                                PreviewTopBar(page, "图片预览", retry, onDismiss, download)
                                ImagePreview(page, retry, Modifier.weight(1f), active = index == pager.settledPage) { zoomed ->
                                    zoomedPage = if (zoomed) index else zoomedPage.takeUnless { it == index }
                                }
                            }
                        }
                    }
                }
                if (showGalleryNavigation) {
                    Row(
                        Modifier.fillMaxWidth().navigationBarsPadding().padding(horizontal = 16.dp, vertical = 4.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        PreviewIconButton("back", "上一项", enabled = pager.currentPage > 0 && !pager.isScrollInProgress) {
                            scope.launch { pager.animateScrollToPage(pager.currentPage - 1) }
                        }
                        Text("${pager.currentPage + 1} / ${gallery.size}", modifier = Modifier.weight(1f), textAlign = TextAlign.Center, color = Color.White.copy(alpha = .8f), style = MaterialTheme.typography.labelLarge)
                        PreviewIconButton("arrow", "下一项", enabled = pager.currentPage < gallery.lastIndex && !pager.isScrollInProgress) {
                            scope.launch { pager.animateScrollToPage(pager.currentPage + 1) }
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun PreviewTopBar(
    event: NativeEvent.Preview,
    kind: String,
    retry: () -> Unit,
    onDismiss: () -> Unit,
    onDownload: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Row(modifier.fillMaxWidth().padding(start = 4.dp, end = 8.dp, top = 6.dp, bottom = 8.dp), verticalAlignment = Alignment.CenterVertically) {
        PreviewIconButton("close", "关闭预览", onClick = onDismiss)
        Column(Modifier.weight(1f).padding(horizontal = 8.dp)) {
            Text(event.file.name.ifBlank { "媒体预览" }, color = Color.White, style = MaterialTheme.typography.titleSmall, maxLines = 1, overflow = TextOverflow.Ellipsis)
            Text(
                kind + if (event.file.sizeBytes > 0) " · ${formatBytes(event.file.sizeBytes)}" else "",
                color = Color.White.copy(alpha = .6f), style = MaterialTheme.typography.labelSmall,
            )
        }
        PreviewIconButton("refresh", "刷新预览链接", onClick = retry)
        if (event.file.id.isNotBlank()) PreviewIconButton("download", "下载媒体", enabled = !event.file.isExpired, onClick = onDownload)
    }
}

@Composable
private fun PreviewIconButton(icon: String, label: String, enabled: Boolean = true, onClick: () -> Unit) {
    PreviewControlButton(label, enabled = enabled, onClick = onClick) {
        ArcoIcon(icon, size = 22.dp, tint = Color.White.copy(alpha = if (enabled) .95f else .35f))
    }
}

@Composable
private fun PreviewControlButton(
    label: String,
    modifier: Modifier = Modifier.size(48.dp),
    enabled: Boolean = true,
    onClick: () -> Unit,
    content: @Composable BoxScope.() -> Unit,
) {
    Box(
        modifier.clip(CircleShape).clickable(enabled = enabled, role = Role.Button, onClick = onClick)
            .semantics { contentDescription = label },
        contentAlignment = Alignment.Center,
        content = content,
    )
}

@Composable
private fun ImagePreview(event: NativeEvent.Preview, retry: () -> Unit, modifier: Modifier, active: Boolean, onZoomChanged: (Boolean) -> Unit) {
    val loader = remember(event.url, event.headers) { PreviewImageLoader(event.url, event.headers) }
    var bitmap by remember(loader) { mutableStateOf<Bitmap?>(null) }
    var error by remember(loader) { mutableStateOf<String?>(null) }
    var scale by remember(loader) { mutableFloatStateOf(1f) }
    var offset by remember(loader) { mutableStateOf(Offset.Zero) }
    var viewport by remember(loader) { mutableStateOf(IntSize.Zero) }

    LaunchedEffect(active) {
        if (!active) { scale = 1f; offset = Offset.Zero }
    }
    LaunchedEffect(scale > 1.001f) { onZoomChanged(scale > 1.001f) }
    DisposableEffect(loader) { onDispose { loader.close() } }
    LaunchedEffect(loader) {
        try {
            bitmap = withContext(Dispatchers.IO) { loader.load() }
        } catch (cancelled: CancellationException) {
            throw cancelled
        } catch (_: OutOfMemoryError) {
            error = "图片超过设备可预览大小，建议下载后查看。"
        } catch (failure: Exception) {
            error = when (failure) {
                is PreviewFailure -> failure.message
                is SocketTimeoutException -> "图片加载超时，请检查网络后重试。"
                else -> "暂时无法加载图片，请检查网络或刷新预览链接。"
            }
        }
    }
    Column(modifier.fillMaxWidth()) {
        Box(
            Modifier.weight(1f).fillMaxWidth().clipToBounds().onSizeChanged { viewport = it }
                .pointerInput(loader, bitmap) {
                    awaitEachGesture {
                        awaitFirstDown(requireUnconsumed = false)
                        do {
                            val change = awaitPointerEvent()
                            val image = bitmap
                            if (image != null && viewport.width > 0 && viewport.height > 0 && change.changes.any { it.pressed && it.previousPressed } && (scale > 1.001f || change.changes.count { it.pressed } > 1)) {
                                val nextScale = (scale * change.calculateZoom()).coerceIn(1f, 6f)
                                val relativeCentroid = change.calculateCentroid() - Offset(viewport.width / 2f, viewport.height / 2f)
                                val nextOffset = (offset - relativeCentroid) * (nextScale / scale) + relativeCentroid + change.calculatePan()
                                val fit = minOf(viewport.width.toFloat() / image.width, viewport.height.toFloat() / image.height)
                                val maxX = ((image.width * fit * nextScale - viewport.width) / 2f).coerceAtLeast(0f)
                                val maxY = ((image.height * fit * nextScale - viewport.height) / 2f).coerceAtLeast(0f)
                                scale = nextScale
                                offset = Offset(nextOffset.x.coerceIn(-maxX, maxX), nextOffset.y.coerceIn(-maxY, maxY))
                                change.changes.forEach { if (it.positionChanged()) it.consume() }
                            }
                        } while (change.changes.any { it.pressed })
                    }
                }
                .pointerInput(loader) {
                    detectTapGestures(onDoubleTap = { scale = 1f; offset = Offset.Zero })
                },
            contentAlignment = Alignment.Center,
        ) {
            val image = bitmap
            when {
                error != null -> PreviewError(requireNotNull(error), retry)
                image == null -> CircularProgressIndicator(Modifier.size(32.dp), strokeWidth = 3.dp)
                else -> Image(
                    bitmap = image.asImageBitmap(),
                    contentDescription = event.file.name,
                    contentScale = ContentScale.Fit,
                    modifier = Modifier.fillMaxSize().graphicsLayer {
                        scaleX = scale
                        scaleY = scale
                        translationX = offset.x
                        translationY = offset.y
                    },
                )
            }
        }
        if (bitmap != null) {
            Text(
                when {
                    scale > 1.001f && event.gallery.size > 1 -> "拖动查看 · 双击复位后切换图片"
                    scale > 1.001f -> "拖动查看 · 双击复位"
                    event.gallery.size > 1 -> "左右滑动切换 · 双指缩放 · 双击复位"
                    else -> "双指缩放 · 拖动查看 · 双击复位"
                },
                Modifier.fillMaxWidth().padding(16.dp),
                textAlign = TextAlign.Center,
                style = MaterialTheme.typography.labelMedium,
                color = Color.White.copy(alpha = .55f),
            )
        }
    }
}

@Composable
private fun PlayerPreview(event: NativeEvent.Preview, audio: Boolean, retry: () -> Unit, onDismiss: () -> Unit, onDownload: () -> Unit, fullscreen: Boolean, bottomInsets: Boolean, onToggleFullscreen: () -> Unit) {
    val context = LocalContext.current
    val motion = LocalMotionEnabled.current
    val owner = remember(context) { context.lifecycleOwner() }
    val playback = remember(event.url, event.headers, audio) { PreviewPlayback(context, event.url, event.headers, audio) }
    var seeking by remember(playback) { mutableStateOf(false) }
    var seekFraction by remember(playback) { mutableFloatStateOf(0f) }
    var controlsShown by remember(playback) { mutableStateOf(true) }
    var interaction by remember(playback) { mutableIntStateOf(0) }
    val controlsVisible = audio || controlsShown || !playback.playing || playback.loading || playback.buffering || seeking || playback.error != null
    val showControls = { controlsShown = true; interaction++ }

    DisposableEffect(playback, owner) {
        playback.setForeground(owner?.lifecycle?.currentState?.isAtLeast(Lifecycle.State.STARTED) != false)
        val observer = LifecycleEventObserver { _, change ->
            when (change) {
                Lifecycle.Event.ON_START, Lifecycle.Event.ON_RESUME -> playback.setForeground(true)
                Lifecycle.Event.ON_PAUSE, Lifecycle.Event.ON_STOP, Lifecycle.Event.ON_DESTROY -> playback.setForeground(false)
                else -> Unit
            }
        }
        owner?.lifecycle?.addObserver(observer)
        playback.load()
        onDispose {
            owner?.lifecycle?.removeObserver(observer)
            playback.close()
        }
    }
    LaunchedEffect(playback) {
        while (isActive) {
            playback.updatePosition()
            delay(300)
        }
    }
    LaunchedEffect(controlsShown, playback.playing, playback.loading, playback.buffering, seeking, interaction) {
        if (!audio && controlsShown && playback.playing && !playback.loading && !playback.buffering && !seeking) {
            delay(3_000)
            controlsShown = false
        }
    }
    Box(Modifier.fillMaxSize().background(Color.Black), contentAlignment = Alignment.Center) {
        when {
            playback.error != null -> PreviewError(requireNotNull(playback.error), retry)
            audio -> Box(Modifier.fillMaxSize().padding(bottom = 112.dp), contentAlignment = Alignment.Center) {
                AudioArtwork(event.file.name, playback.position, playback.duration)
            }
            else -> VideoSurface(playback)
        }
        if (playback.error == null) {
            Box(Modifier.fillMaxSize().pointerInput(playback, controlsVisible) {
                detectTapGestures(
                    onTap = { controlsShown = !controlsVisible; interaction++ },
                    onDoubleTap = { point ->
                        playback.seekBy(if (point.x < size.width / 2f) -10_000 else 10_000)
                        showControls()
                    },
                )
            })
        }
        AnimatedVisibility(
            visible = controlsVisible,
            modifier = Modifier.align(Alignment.TopCenter),
            enter = fadeIn(tween(if (motion) 180 else 0)),
            exit = fadeOut(tween(if (motion) 180 else 0)),
        ) {
            PreviewTopBar(
                event, if (audio) "音频预览" else "视频预览", retry, onDismiss,
                onDownload = { showControls(); onDownload() },
                modifier = Modifier.background(Brush.verticalGradient(listOf(Color.Black.copy(alpha = .85f), Color.Transparent))).statusBarsPadding(),
            )
        }
        if (playback.error == null && (playback.loading || playback.buffering)) {
            Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(12.dp)) {
                CircularProgressIndicator(Modifier.size(34.dp), color = Color.White, strokeWidth = 2.dp)
                Text(if (playback.loading) "正在加载" else "正在缓冲", color = Color.White.copy(alpha = .75f), style = MaterialTheme.typography.labelMedium)
            }
        }
        AnimatedVisibility(
            visible = controlsVisible && playback.ready && !playback.loading && !playback.buffering && playback.error == null,
            modifier = Modifier.align(if (audio) Alignment.BottomCenter else Alignment.Center).padding(bottom = if (audio) 150.dp else 0.dp),
            enter = fadeIn(tween(if (motion) 180 else 0)),
            exit = fadeOut(tween(if (motion) 180 else 0)),
        ) {
            Row(horizontalArrangement = Arrangement.spacedBy(28.dp), verticalAlignment = Alignment.CenterVertically) {
                PreviewSeekButton(forward = false, enabled = playback.duration > 0) { playback.seekBy(-10_000); showControls() }
                PreviewControlButton(
                    label = if (playback.playing) "暂停" else "播放",
                    onClick = { if (playback.playing) playback.pause() else playback.play(); showControls() },
                    modifier = Modifier.size(72.dp).background(Color.Black.copy(alpha = .4f), CircleShape),
                ) {
                    ArcoIcon(if (playback.playing) "pause" else "play", size = 38.dp, tint = Color.White)
                }
                PreviewSeekButton(forward = true, enabled = playback.duration > 0) { playback.seekBy(10_000); showControls() }
            }
        }
        AnimatedVisibility(
            visible = controlsVisible && playback.error == null,
            modifier = Modifier.align(Alignment.BottomCenter),
            enter = fadeIn(tween(if (motion) 180 else 0)),
            exit = fadeOut(tween(if (motion) 180 else 0)),
        ) {
            Column(
                Modifier.fillMaxWidth().background(Brush.verticalGradient(listOf(Color.Transparent, Color.Black.copy(alpha = .9f))))
                    .then(if (bottomInsets) Modifier.navigationBarsPadding() else Modifier).padding(start = 20.dp, end = 20.dp, top = 28.dp, bottom = 18.dp),
            ) {
                val progress = if (seeking) seekFraction else if (playback.duration > 0) {
                    (playback.position.toFloat() / playback.duration).coerceIn(0f, 1f)
                } else 0f
                PlaybackSeekBar(
                    progress = progress,
                    buffered = playback.bufferedFraction,
                    enabled = playback.ready && playback.duration > 0,
                    onSeek = { seeking = true; seekFraction = it; showControls() },
                    onSeekFinished = {
                        playback.seekTo((seekFraction * playback.duration).toInt())
                        seeking = false
                        showControls()
                    },
                )
                Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                    Text(playbackTime(if (seeking) (seekFraction * playback.duration).toInt() else playback.position), style = MaterialTheme.typography.labelMedium, color = Color.White)
                    Text(" / ${playbackTime(playback.duration)}", style = MaterialTheme.typography.labelMedium, color = Color.White.copy(alpha = .65f))
                    Spacer(Modifier.weight(1f))
                    if (!audio) PreviewIconButton(
                        if (fullscreen) "fullscreen_exit" else "fullscreen",
                        if (fullscreen) "退出全屏" else "全屏播放",
                        onClick = { showControls(); onToggleFullscreen() },
                    )
                }
            }
        }
    }
}

@Composable
private fun PlaybackSeekBar(progress: Float, buffered: Float, enabled: Boolean, onSeek: (Float) -> Unit, onSeekFinished: () -> Unit) {
    val latestOnSeek by androidx.compose.runtime.rememberUpdatedState(onSeek)
    val latestOnSeekFinished by androidx.compose.runtime.rememberUpdatedState(onSeekFinished)
    AndroidView(
        factory = { context ->
            val density = context.resources.displayMetrics.density
            val trackHeight = (3 * density).toInt().coerceAtLeast(1)
            fun track(color: Int) = GradientDrawable().apply {
                shape = GradientDrawable.RECTANGLE
                cornerRadius = trackHeight / 2f
                setColor(color)
            }
            SeekBar(context).apply {
                max = 10_000
                contentDescription = "播放进度"
                splitTrack = false
                progressDrawable = LayerDrawable(arrayOf(
                    track(android.graphics.Color.argb(70, 255, 255, 255)),
                    ClipDrawable(track(android.graphics.Color.argb(135, 255, 255, 255)), Gravity.LEFT, ClipDrawable.HORIZONTAL),
                    ClipDrawable(track(android.graphics.Color.WHITE), Gravity.LEFT, ClipDrawable.HORIZONTAL),
                )).apply {
                    setId(0, android.R.id.background)
                    setId(1, android.R.id.secondaryProgress)
                    setId(2, android.R.id.progress)
                    repeat(3) { index -> setLayerHeight(index, trackHeight); setLayerGravity(index, Gravity.CENTER_VERTICAL) }
                }
                thumb = GradientDrawable().apply {
                    shape = GradientDrawable.OVAL
                    setColor(android.graphics.Color.WHITE)
                    setSize((12 * density).toInt(), (12 * density).toInt())
                }
                thumbTintList = ColorStateList.valueOf(android.graphics.Color.WHITE)
                progressTintList = ColorStateList.valueOf(android.graphics.Color.WHITE)
                secondaryProgressTintList = ColorStateList.valueOf(android.graphics.Color.argb(135, 255, 255, 255))
                progressBackgroundTintList = ColorStateList.valueOf(android.graphics.Color.argb(70, 255, 255, 255))
                thumbOffset = (6 * density).toInt()
                setPadding((8 * density).toInt(), 0, (8 * density).toInt(), 0)
                setOnSeekBarChangeListener(object : SeekBar.OnSeekBarChangeListener {
                    override fun onProgressChanged(seekBar: SeekBar, value: Int, fromUser: Boolean) {
                        if (fromUser) {
                            latestOnSeek(value / 10_000f)
                            if (!seekBar.isPressed) latestOnSeekFinished()
                        }
                    }
                    override fun onStartTrackingTouch(seekBar: SeekBar) { latestOnSeek(seekBar.progress / 10_000f) }
                    override fun onStopTrackingTouch(seekBar: SeekBar) { latestOnSeekFinished() }
                })
            }
        },
        update = { view ->
            view.isEnabled = enabled
            view.alpha = if (enabled) 1f else .45f
            view.secondaryProgress = (buffered.coerceIn(0f, 1f) * view.max).toInt()
            if (!view.isPressed) view.progress = (progress.coerceIn(0f, 1f) * view.max).toInt()
        },
        modifier = Modifier.fillMaxWidth().height(40.dp),
    )
}

@Composable
private fun PreviewSeekButton(forward: Boolean, enabled: Boolean, onClick: () -> Unit) {
    val color = Color.White.copy(alpha = if (enabled) .95f else .35f)
    PreviewControlButton(
        label = if (forward) "前进 10 秒" else "后退 10 秒",
        onClick = onClick, enabled = enabled,
        modifier = Modifier.size(56.dp).background(Color.Black.copy(alpha = .25f), CircleShape),
    ) {
        Box(Modifier.size(34.dp), contentAlignment = Alignment.Center) {
            ArcoIcon(if (forward) "forward10" else "replay10", size = 32.dp, tint = color)
        }
    }
}

@Composable
private fun VideoSurface(playback: PreviewPlayback) {
    val context = LocalContext.current
    val view = remember(playback) {
        TextureView(context).apply {
            surfaceTextureListener = object : TextureView.SurfaceTextureListener {
                private var output: PreviewVideoOutput? = null

                override fun onSurfaceTextureAvailable(texture: SurfaceTexture, width: Int, height: Int) {
                    val next = PreviewVideoOutput(texture)
                    output = next
                    playback.attachVideoOutput(next)
                }

                override fun onSurfaceTextureSizeChanged(texture: SurfaceTexture, width: Int, height: Int) = Unit
                override fun onSurfaceTextureUpdated(texture: SurfaceTexture) = Unit

                override fun onSurfaceTextureDestroyed(texture: SurfaceTexture): Boolean {
                    val previous = output
                    output = null
                    if (previous != null) playback.detachVideoOutput(previous)
                    else texture.release()
                    // 保留纹理，等待播放线程解除引用后再释放。
                    return false
                }
            }
        }
    }
    BoxWithConstraints(Modifier.fillMaxSize().background(Color.Black), contentAlignment = Alignment.Center) {
        val ratio = if (playback.videoWidth > 0 && playback.videoHeight > 0) playback.videoWidth.toFloat() / playback.videoHeight else 16f / 9f
        val viewportRatio = if (maxHeight.value > 0f) maxWidth / maxHeight else ratio
        val fitted = if (ratio >= viewportRatio) Modifier.width(maxWidth).height(maxWidth / ratio)
            else Modifier.width(maxHeight * ratio).height(maxHeight)
        AndroidView(factory = { view }, modifier = fitted, update = { it.keepScreenOn = playback.playing })
    }
}

private class PreviewVideoOutput(val texture: SurfaceTexture) {
    val surface = Surface(texture)
    private var viewDestroyed = false
    private var playerDetached = false
    private var released = false

    val canAttach: Boolean
        get() = synchronized(this) { !viewDestroyed && !released && surface.isValid }

    @Synchronized
    fun onViewDestroyed() {
        viewDestroyed = true
        releaseWhenUnused()
    }

    @Synchronized
    fun onPlayerDetached() {
        playerDetached = true
        releaseWhenUnused()
    }

    private fun releaseWhenUnused() {
        if (released || !viewDestroyed || !playerDetached) return
        released = true
        runCatching { surface.release() }
        runCatching { texture.release() }
    }
}

@Composable
private fun AudioArtwork(name: String, position: Int, duration: Int) {
    val primary = MaterialTheme.colorScheme.primary
    val background = Color(0xFF14213D)
    Column(Modifier.fillMaxWidth().verticalScroll(rememberScrollState()).padding(32.dp), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(24.dp)) {
        Box(Modifier.size(150.dp), contentAlignment = Alignment.Center) {
            Canvas(Modifier.fillMaxSize()) {
                drawCircle(background)
                drawCircle(primary.copy(alpha = .13f), radius = size.minDimension * .35f)
                drawCircle(primary.copy(alpha = .2f), radius = size.minDimension * .2f)
            }
            ArcoIcon("music", size = 50.dp, tint = Color.White.copy(alpha = .9f))
        }
        Text(name, color = Color.White, style = MaterialTheme.typography.titleLarge, maxLines = 2, overflow = TextOverflow.Ellipsis, textAlign = TextAlign.Center)
        Canvas(Modifier.fillMaxWidth().height(44.dp)) {
            val progress = if (duration > 0) position.toFloat() / duration else 0f
            repeat(48) { index ->
                val x = size.width * (index + .5f) / 48f
                val height = size.height * (.2f + .8f * abs(sin(index * 1.7f)))
                drawLine(
                    if (index / 48f <= progress) primary else primary.copy(alpha = .2f),
                    Offset(x, (size.height - height) / 2f), Offset(x, (size.height + height) / 2f),
                    strokeWidth = (size.width / 100f).coerceAtLeast(2f), cap = StrokeCap.Round,
                )
            }
        }
    }
}

@Composable
private fun PreviewError(message: String, retry: () -> Unit) {
    Column(Modifier.fillMaxWidth().verticalScroll(rememberScrollState()).padding(32.dp), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(14.dp)) {
        Text("暂时无法预览", color = Color.White, style = MaterialTheme.typography.titleLarge)
        Text(message, textAlign = TextAlign.Center, color = Color.White.copy(alpha = .65f), style = MaterialTheme.typography.bodyMedium)
        Box(
            Modifier.clip(RoundedCornerShape(12.dp)).background(MaterialTheme.colorScheme.primary)
                .clickable(role = Role.Button, onClick = retry).padding(horizontal = 18.dp, vertical = 14.dp),
            contentAlignment = Alignment.Center,
        ) { Text("刷新链接重试", color = MaterialTheme.colorScheme.onPrimary, style = MaterialTheme.typography.labelLarge) }
    }
}

@OptIn(UnstableApi::class)
private class PreviewPlayback(
    private val context: Context,
    private val url: String,
    private val headers: Map<String, String>,
    private val audio: Boolean,
) {
    var loading by mutableStateOf(true)
        private set
    var buffering by mutableStateOf(false)
        private set
    var ready by mutableStateOf(false)
        private set
    var playing by mutableStateOf(false)
        private set
    var error by mutableStateOf<String?>(null)
        private set
    var duration by mutableIntStateOf(0)
        private set
    var position by mutableIntStateOf(0)
        private set
    var videoWidth by mutableIntStateOf(0)
        private set
    var videoHeight by mutableIntStateOf(0)
        private set
    var bufferedFraction by mutableFloatStateOf(0f)
        private set
    private val workerThread = HandlerThread("LuxuryMediaPreview").apply { start() }
    private val worker = Handler(workerThread.looper)
    private val main = Handler(Looper.getMainLooper())
    private var player: ExoPlayer? = null
    private var videoOutput: PreviewVideoOutput? = null
    private var prepared = false
    private var mediaDuration = 0
    private var mediaPosition = 0
    private var foreground = true
    private var playWhenReady = true
    @Volatile
    private var closed = false

    fun load() = dispatch {
        if (player != null) return@dispatch
        try {
            previewUri(url)
            val dataSourceFactory = DataSource.Factory { PreviewHttpDataSource(url, headers) }
            val loadControl = DefaultLoadControl.Builder()
                .setBufferDurationsMs(3_000, 15_000, 500, 1_000)
                .setTargetBufferBytes(8 * 1024 * 1024)
                .setPrioritizeTimeOverSizeThresholds(true)
                .build()
            val current = ExoPlayer.Builder(context.applicationContext)
                .setLooper(workerThread.looper)
                .setLoadControl(loadControl)
                .setMediaSourceFactory(DefaultMediaSourceFactory(dataSourceFactory))
                .build()
            player = current
            current.setAudioAttributes(
                AudioAttributes.Builder().setUsage(C.USAGE_MEDIA)
                    .setContentType(if (audio) C.AUDIO_CONTENT_TYPE_MUSIC else C.AUDIO_CONTENT_TYPE_MOVIE).build(),
                true,
            )
            current.addListener(object : Player.Listener {
                override fun onPlaybackStateChanged(playbackState: Int) { syncState(current) }
                override fun onIsPlayingChanged(isPlaying: Boolean) { syncState(current) }
                override fun onVideoSizeChanged(videoSize: VideoSize) {
                    if (closed || player !== current) return
                    publish {
                        videoWidth = videoSize.width.coerceAtLeast(0)
                        videoHeight = videoSize.height.coerceAtLeast(0)
                    }
                }
                override fun onPlayerError(failure: PlaybackException) {
                    if (!closed && player === current) fail("链接可能已失效、服务器未提供分段读取，或设备不支持该媒体格式。请刷新链接重试。")
                }
            })
            videoOutput?.takeIf { it.canAttach }?.let { current.setVideoSurface(it.surface) }
            current.setMediaItem(MediaItem.fromUri(url))
            current.playWhenReady = foreground && playWhenReady && (audio || videoOutput?.canAttach == true)
            current.prepare()
        } catch (_: Exception) {
            fail("媒体暂时无法加载，请检查网络或刷新预览链接。")
        }
    }

    private fun syncState(current: ExoPlayer) {
        if (closed || player !== current) return
        val state = current.playbackState
        if (state == Player.STATE_READY || state == Player.STATE_ENDED) prepared = true
        mediaDuration = current.duration.coerceIn(0L, Int.MAX_VALUE.toLong()).toInt()
        mediaPosition = current.currentPosition.coerceIn(0L, Int.MAX_VALUE.toLong()).toInt()
        val currentDuration = mediaDuration
        val currentPosition = mediaPosition
        val currentPlaying = current.isPlaying
        val currentBuffer = if (currentDuration > 0) (current.bufferedPosition.toFloat() / currentDuration).coerceIn(0f, 1f) else 0f
        val currentReady = prepared
        publish {
            duration = currentDuration
            position = currentPosition
            bufferedFraction = currentBuffer
            ready = currentReady
            loading = !currentReady && state == Player.STATE_BUFFERING
            buffering = currentReady && state == Player.STATE_BUFFERING
            playing = currentPlaying
        }
    }

    fun attachVideoOutput(output: PreviewVideoOutput) {
        if (!worker.post {
            if (closed || !output.canAttach) {
                output.onPlayerDetached()
                return@post
            }
            val previous = videoOutput
            try {
                player?.setVideoSurface(output.surface)
                videoOutput = output
                previous?.onPlayerDetached()
                if (foreground && playWhenReady) startPlayback()
            } catch (_: Exception) {
                output.onPlayerDetached()
                fail("视频画面暂时不可用，请刷新链接重试。")
            }
        }) output.onPlayerDetached()
    }

    fun detachVideoOutput(output: PreviewVideoOutput) {
        output.onViewDestroyed()
        if (!worker.post {
            if (videoOutput === output) {
                pausePlayback(clearPlayIntent = false)
                try {
                    player?.clearVideoSurface()
                } catch (_: Exception) {
                    runCatching { player?.release() }
                    player = null
                    prepared = false
                    fail("视频画面已关闭，请刷新链接重试。")
                }
                videoOutput = null
            }
            output.onPlayerDetached()
        }) output.onPlayerDetached()
    }

    fun setForeground(value: Boolean) = dispatch {
        foreground = value
        if (!value) pausePlayback(clearPlayIntent = true)
    }

    fun play() = dispatch {
        playWhenReady = true
        startPlayback()
    }

    private fun startPlayback() {
        val current = player ?: return
        if (closed || !foreground || (!audio && videoOutput?.canAttach != true)) return
        if (current.playbackState == Player.STATE_ENDED) current.seekTo(0)
        current.play()
    }

    fun pause() = dispatch { pausePlayback(clearPlayIntent = true) }

    private fun pausePlayback(clearPlayIntent: Boolean) {
        if (clearPlayIntent) playWhenReady = false
        player?.pause()
        publish { playing = false }
    }

    fun seekTo(value: Int) = dispatch { seekPlayback(value) }

    private fun seekPlayback(value: Int) {
        if (!prepared || closed) return
        val target = value.coerceIn(0, mediaDuration)
        player?.seekTo(target.toLong())
        mediaPosition = target
        publish { position = target }
    }

    fun seekBy(milliseconds: Int) = dispatch {
        seekPlayback((mediaPosition.toLong() + milliseconds).coerceIn(0L, mediaDuration.toLong()).toInt())
    }

    fun updatePosition() = dispatch {
        player?.let(::syncState)
    }

    fun close() {
        if (closed) return
        closed = true
        worker.post {
            val current = player
            player = null
            runCatching { current?.clearVideoSurface() }
            runCatching { current?.release() }
            videoOutput?.onPlayerDetached()
            videoOutput = null
            prepared = false
            workerThread.quitSafely()
        }
    }

    private fun fail(message: String) {
        prepared = false
        playWhenReady = false
        runCatching { player?.pause() }
        publish {
            error = message
            ready = false
            loading = false
            buffering = false
            playing = false
        }
    }

    private fun dispatch(action: () -> Unit) {
        if (!closed) worker.post {
            if (!closed) try { action() } catch (_: Exception) {
                fail("媒体暂时无法播放，请刷新链接重试。")
            }
        }
    }

    private fun publish(action: () -> Unit) {
        if (!closed) main.post { if (!closed) action() }
    }
}

@OptIn(UnstableApi::class)
private class PreviewHttpDataSource(
    private val originalUrl: String,
    private val headers: Map<String, String>,
) : BaseDataSource(true) {
    private var connection: HttpURLConnection? = null
    private var input: InputStream? = null
    private var uri: Uri? = null
    private var remaining = C.LENGTH_UNSET.toLong()
    private var opened = false

    override fun open(dataSpec: DataSpec): Long {
        transferInitializing(dataSpec)
        val origin = previewUri(originalUrl)
        var target = previewUri(dataSpec.uri.toString())
        try {
            repeat(6) { redirects ->
                if (!sameOrigin(origin, target)) throw PreviewFailure("媒体地址跳转到了其他服务器，请刷新链接重试。")
                if (Thread.currentThread().isInterrupted) throw IOException("媒体加载已取消")
                val current = (target.toURL().openConnection() as HttpURLConnection).apply {
                    instanceFollowRedirects = false
                    connectTimeout = 15_000
                    readTimeout = 20_000
                    useCaches = false
                    requestMethod = "GET"
                    (headers + dataSpec.httpRequestHeaders).forEach { (name, value) -> setRequestProperty(name, value) }
                    setRequestProperty("Accept-Encoding", "identity")
                    val rangeEnd = if (dataSpec.length == C.LENGTH_UNSET.toLong()) "" else (dataSpec.position + dataSpec.length - 1).toString()
                    setRequestProperty("Range", "bytes=${dataSpec.position}-$rangeEnd")
                }
                connection = current
                val status = current.responseCode
                if (status in setOf(301, 302, 303, 307, 308)) {
                    if (redirects == 5) throw PreviewFailure("媒体地址跳转次数过多，请刷新链接重试。")
                    val location = current.getHeaderField("Location") ?: throw PreviewFailure("媒体跳转地址不可用。")
                    target = previewUri(target.resolve(location).toASCIIString())
                    current.disconnect()
                    connection = null
                    return@repeat
                }
                if (status == 416) {
                    val total = current.getHeaderField("Content-Range")?.substringAfter("*/")?.trim()?.toLongOrNull()
                    if (total == dataSpec.position) {
                        uri = Uri.parse(target.toASCIIString())
                        remaining = 0
                        opened = true
                        transferStarted(dataSpec)
                        return 0
                    }
                }
                if (status !in 200..299) throw PreviewFailure(
                    if (status in setOf(401, 403, 404, 410)) "预览链接已失效或文件已过期，请刷新链接重试。"
                    else "服务器暂时无法提供媒体，请稍后重试。",
                )
                if (dataSpec.position > 0 && status != HttpURLConnection.HTTP_PARTIAL) {
                    throw PreviewFailure("服务器未提供分段读取，暂时无法快速定位该媒体。")
                }
                val contentEncoding = current.getHeaderField("Content-Encoding").orEmpty()
                if (contentEncoding.isNotBlank() && !contentEncoding.equals("identity", ignoreCase = true)) {
                    throw PreviewFailure("服务器返回了不支持定位的媒体编码。")
                }
                val contentRange = current.getHeaderField("Content-Range")?.let {
                    Regex("bytes\\s+(\\d+)-(\\d+)/(\\d+|\\*)", RegexOption.IGNORE_CASE).matchEntire(it.trim())
                }
                var rangeLength = C.LENGTH_UNSET.toLong()
                if (status == HttpURLConnection.HTTP_PARTIAL) {
                    val start = contentRange?.groupValues?.get(1)?.toLongOrNull()
                    val end = contentRange?.groupValues?.get(2)?.toLongOrNull()
                    val total = contentRange?.groupValues?.get(3)?.toLongOrNull()
                    if (start != dataSpec.position || end == null || end < dataSpec.position || total != null && end >= total) {
                        throw PreviewFailure("服务器返回了不正确的媒体分段。")
                    }
                    rangeLength = end - dataSpec.position + 1
                }
                uri = Uri.parse(target.toASCIIString())
                val declaredLength = current.contentLengthLong.takeIf { it >= 0 } ?: C.LENGTH_UNSET.toLong()
                val responseLength = if (rangeLength >= 0 && declaredLength >= 0) minOf(rangeLength, declaredLength)
                    else if (rangeLength >= 0) rangeLength else declaredLength
                remaining = when {
                    dataSpec.length == C.LENGTH_UNSET.toLong() -> responseLength
                    responseLength == C.LENGTH_UNSET.toLong() -> dataSpec.length
                    else -> minOf(dataSpec.length, responseLength)
                }
                input = current.inputStream
                opened = true
                transferStarted(dataSpec)
                return remaining
            }
            throw PreviewFailure("媒体地址不可用。")
        } catch (failure: Exception) {
            close()
            throw failure
        }
    }

    override fun read(buffer: ByteArray, offset: Int, length: Int): Int {
        if (length == 0) return 0
        if (remaining == 0L) return C.RESULT_END_OF_INPUT
        if (Thread.currentThread().isInterrupted) throw IOException("媒体加载已取消")
        val count = input?.read(buffer, offset, if (remaining == C.LENGTH_UNSET.toLong()) length else minOf(length.toLong(), remaining).toInt())
            ?: throw IOException("媒体连接已关闭")
        if (count == -1) {
            if (remaining > 0) throw EOFException("媒体分段提前结束")
            return C.RESULT_END_OF_INPUT
        }
        if (remaining != C.LENGTH_UNSET.toLong()) remaining -= count
        if (count > 0) bytesTransferred(count)
        return count
    }

    override fun getUri(): Uri? = uri

    override fun getResponseHeaders(): Map<String, List<String>> = connection?.headerFields.orEmpty()
        .mapNotNull { (name, values) -> name?.let { it to values.orEmpty() } }.toMap()

    override fun close() {
        runCatching { input?.close() }
        input = null
        connection?.disconnect()
        connection = null
        uri = null
        if (opened) { opened = false; transferEnded() }
    }
}

private class PreviewImageLoader(private val url: String, private val headers: Map<String, String>) {
    private val lock = Any()
    private var connection: HttpURLConnection? = null
    private var bitmap: Bitmap? = null
    private var closed = false

    suspend fun load(): Bitmap {
        val initial = previewUri(url)
        var target = initial
        repeat(6) { redirects ->
            currentCoroutineContext().ensureActive()
            if (!sameOrigin(initial, target)) throw PreviewFailure("预览地址跳转到了其他服务器，请刷新链接重试。")
            val current = (target.toURL().openConnection() as HttpURLConnection).apply {
                instanceFollowRedirects = false
                connectTimeout = 45_000
                readTimeout = 45_000
                useCaches = false
                setRequestProperty("Accept", "image/*")
                headers.forEach { (name, value) -> setRequestProperty(name, value) }
            }
            synchronized(lock) {
                if (closed) {
                    current.disconnect()
                    throw CancellationException("预览已关闭")
                }
                connection = current
            }
            try {
                val status = current.responseCode
                if (status in setOf(301, 302, 303, 307, 308)) {
                    if (redirects == 5) throw PreviewFailure("图片地址跳转次数过多，请刷新链接重试。")
                    val location = current.getHeaderField("Location") ?: throw PreviewFailure("图片跳转地址不可用。")
                    target = previewUri(target.resolve(location).toASCIIString())
                    return@repeat
                }
                if (status !in 200..299) throw PreviewFailure(
                    if (status in setOf(401, 403, 404, 410)) "预览链接可能已失效或文件已过期，请刷新链接重试。"
                    else "服务器暂时无法提供图片，请稍后重试。",
                )
                if (current.contentLengthLong > MAX_IMAGE_BYTES) throw PreviewFailure("图片超过 24 MB，建议下载后查看原图。")
                val bytes = current.inputStream.use { input ->
                    val output = ByteArrayOutputStream()
                    val buffer = ByteArray(DEFAULT_BUFFER_SIZE)
                    var total = 0
                    while (true) {
                        currentCoroutineContext().ensureActive()
                        val count = input.read(buffer)
                        if (count == -1) break
                        total += count
                        if (total > MAX_IMAGE_BYTES) throw PreviewFailure("图片超过 24 MB，建议下载后查看原图。")
                        output.write(buffer, 0, count)
                    }
                    output.toByteArray()
                }
                currentCoroutineContext().ensureActive()
                val decoded = try {
                    ImageDecoder.decodeBitmap(ImageDecoder.createSource(ByteBuffer.wrap(bytes))) { decoder, info, _ ->
                        var sample = 1
                        val width = info.size.width.toLong()
                        val height = info.size.height.toLong()
                        while ((width / sample) * (height / sample) > 6_000_000L || maxOf(width, height) / sample > 4096L) sample *= 2
                        decoder.setTargetSampleSize(sample)
                        decoder.allocator = ImageDecoder.ALLOCATOR_SOFTWARE
                    }
                } catch (_: OutOfMemoryError) {
                    throw PreviewFailure("图片超过设备可预览大小，建议下载后查看。")
                }
                synchronized(lock) {
                    if (closed) {
                        decoded.recycle()
                        throw CancellationException("预览已关闭")
                    }
                    bitmap = decoded
                }
                return decoded
            } finally {
                synchronized(lock) { if (connection === current) connection = null }
                current.disconnect()
            }
        }
        throw PreviewFailure("图片地址不可用，请刷新链接重试。")
    }

    fun close() = synchronized(lock) {
        closed = true
        connection?.disconnect()
        connection = null
        // 已交给 Compose 的位图可能仍由渲染线程引用，交给 GC 回收。
        bitmap = null
    }

    private companion object { const val MAX_IMAGE_BYTES = 24 * 1024 * 1024 }
}

private class PreviewFailure(message: String) : IOException(message)

private fun previewUri(value: String): URI {
    val uri = try { URI(value) } catch (_: Exception) { throw PreviewFailure("预览地址格式不正确，请刷新链接重试。") }
    if (uri.scheme?.lowercase(Locale.ROOT) !in setOf("http", "https") || uri.host.isNullOrBlank() || uri.rawUserInfo != null) {
        throw PreviewFailure("预览地址不可用，请刷新链接重试。")
    }
    return uri
}

private fun sameOrigin(first: URI, second: URI): Boolean {
    fun port(uri: URI) = if (uri.port >= 0) uri.port else if (uri.scheme.equals("https", true)) 443 else 80
    return first.scheme.equals(second.scheme, true) && first.host.equals(second.host, true) && port(first) == port(second)
}

private fun Context.previewActivity(): Activity? {
    var current: Context = this
    while (current is ContextWrapper) {
        if (current is Activity) return current
        val base = current.baseContext
        if (base === current) return null
        current = base
    }
    return current as? Activity
}

private fun Context.lifecycleOwner(): LifecycleOwner? {
    var current: Context = this
    while (true) {
        if (current is LifecycleOwner) return current
        if (current !is ContextWrapper || current.baseContext === current) return null
        current = current.baseContext
    }
}

private fun playbackTime(milliseconds: Int): String {
    val seconds = milliseconds.coerceAtLeast(0) / 1000
    return if (seconds >= 3600) String.format(Locale.ROOT, "%d:%02d:%02d", seconds / 3600, seconds / 60 % 60, seconds % 60)
    else String.format(Locale.ROOT, "%02d:%02d", seconds / 60, seconds % 60)
}
