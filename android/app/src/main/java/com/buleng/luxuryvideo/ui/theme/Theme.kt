package com.buleng.luxuryvideo.ui.theme

import android.database.ContentObserver
import android.os.Handler
import android.os.Looper
import android.provider.Settings
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Shapes
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp

val LocalMotionEnabled = staticCompositionLocalOf { true }

private val LightColors = lightColorScheme(
    primary = ArcoBlue, onPrimary = Color.White,
    primaryContainer = ArcoBlueLight, onPrimaryContainer = Color(0xFF0E42D2),
    secondary = ArcoSlate, onSecondary = Color.White,
    secondaryContainer = Color(0xFFF2F3F5), onSecondaryContainer = ArcoSlate,
    tertiary = ArcoPurple, tertiaryContainer = Color(0xFFF5E8FF),
    background = ArcoCanvas, onBackground = ArcoInk,
    surface = Color.White, onSurface = ArcoInk,
    surfaceVariant = Color(0xFFF8F8FA), onSurfaceVariant = ArcoSlate,
    surfaceContainer = Color.White, surfaceContainerLow = Color(0xFFF8F8FA),
    surfaceContainerHigh = Color(0xFFF2F3F5), surfaceContainerHighest = Color(0xFFEDEEF1),
    outline = Color(0xFFC9CDD4), outlineVariant = ArcoLine,
    error = ArcoRed, errorContainer = Color(0xFFFFECE8), onErrorContainer = Color(0xFFCB2634),
)

private val DarkColors = darkColorScheme(
    primary = Color(0xFF6AA1FF), onPrimary = Color(0xFF002F91),
    primaryContainer = Color(0xFF142A4F), onPrimaryContainer = Color(0xFFB8D5FF),
    secondary = Color(0xFFB7BEC8), secondaryContainer = Color(0xFF2A2C32),
    onSecondaryContainer = Color(0xFFCED3DC),
    tertiary = Color(0xFFB88AF1), tertiaryContainer = Color(0xFF30213E),
    background = Color(0xFF000000), onBackground = Color(0xFFF2F3F5),
    surface = Color(0xFF1C1C1E), onSurface = Color(0xFFF2F3F5),
    surfaceVariant = Color(0xFF2C2C2E), onSurfaceVariant = Color(0xFFABB2BF),
    surfaceContainer = Color(0xFF1C1C1E), surfaceContainerLow = Color(0xFF23252C),
    surfaceContainerHigh = Color(0xFF2B2D35), surfaceContainerHighest = Color(0xFF34363F),
    outline = Color(0xFF555A67), outlineVariant = Color(0xFF38383A),
    error = Color(0xFFFF8480), errorContainer = Color(0xFF492224), onErrorContainer = Color(0xFFFFDAD8),
)

@Composable
fun LuxuryVideoAppTheme(
    darkTheme: Boolean = isSystemInDarkTheme(),
    @Suppress("UNUSED_PARAMETER") dynamicColor: Boolean = false,
    motionEnabled: Boolean = true,
    content: @Composable () -> Unit,
) {
    val resolver = LocalContext.current.contentResolver
    fun systemMotion(): Boolean = runCatching {
        Settings.Global.getFloat(resolver, Settings.Global.ANIMATOR_DURATION_SCALE, 1f) > 0f
    }.getOrDefault(true)
    var systemAllowsMotion by remember(resolver) { mutableStateOf(systemMotion()) }
    DisposableEffect(resolver) {
        val observer = object : ContentObserver(Handler(Looper.getMainLooper())) {
            override fun onChange(selfChange: Boolean) { systemAllowsMotion = systemMotion() }
        }
        resolver.registerContentObserver(Settings.Global.getUriFor(Settings.Global.ANIMATOR_DURATION_SCALE), false, observer)
        onDispose { resolver.unregisterContentObserver(observer) }
    }
    CompositionLocalProvider(LocalMotionEnabled provides (motionEnabled && systemAllowsMotion)) {
        MaterialTheme(
            colorScheme = if (darkTheme) DarkColors else LightColors,
            typography = Typography,
            shapes = Shapes(
                extraSmall = RoundedCornerShape(6.dp), small = RoundedCornerShape(8.dp),
                medium = RoundedCornerShape(12.dp), large = RoundedCornerShape(18.dp),
                extraLarge = RoundedCornerShape(24.dp),
            ),
            content = content,
        )
    }
}
