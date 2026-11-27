package com.buleng.luxuryvideo

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import android.provider.Settings
import androidx.activity.ComponentActivity
import androidx.activity.SystemBarStyle
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.lifecycle.ViewModelProvider
import com.buleng.luxuryvideo.platform.MediaPreview
import com.buleng.luxuryvideo.ui.CaptureApp
import com.buleng.luxuryvideo.ui.AppUpdateDialog
import com.buleng.luxuryvideo.ui.theme.LuxuryVideoAppTheme

class MainActivity : ComponentActivity() {
    private val viewModel by lazy { ViewModelProvider(this)[CaptureViewModel::class.java] }
    private var pendingNotificationJob: String? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        if (savedInstanceState == null) consumeIntent(intent)
        else pendingNotificationJob = savedInstanceState.getString("pending_notification_job")
        setContent {
            val state by viewModel.state.collectAsState()
            val updateState by viewModel.appUpdates.state.collectAsState()
            val updatePromptActive = !state.initializing && state.session != null
            val updateBlocksBusiness = updatePromptActive && updateState.forced
            var preview by remember { mutableStateOf<NativeEvent.Preview?>(null) }
            LaunchedEffect(state.preferences.serverUrl) { viewModel.appUpdates.configureServer(state.preferences.serverUrl) }
            LaunchedEffect(updateBlocksBusiness) {
                if (updateBlocksBusiness) {
                    preview = null
                    viewModel.dismissPreview()
                }
            }
            val permissionHistory = remember { getSharedPreferences("capture_permissions", MODE_PRIVATE) }
            val notificationPermission = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
                if (!granted) {
                    val preferences = viewModel.state.value.preferences
                    viewModel.savePreferences(preferences.serverUrl, preferences.themeMode, preferences.motionEnabled, false)
                }
            }
            LaunchedEffect(state.session?.id, state.preferences.notificationsEnabled) {
                if (state.session != null && state.preferences.notificationsEnabled &&
                    Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
                    checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED &&
                    !permissionHistory.getBoolean("notification_requested", false)
                ) {
                    permissionHistory.edit().putBoolean("notification_requested", true).apply()
                    notificationPermission.launch(Manifest.permission.POST_NOTIFICATIONS)
                }
            }
            LaunchedEffect(viewModel) {
                viewModel.events.collect { event ->
                    when (event) {
                        is NativeEvent.Preview -> if (!viewModel.appUpdates.state.value.forced && viewModel.isCurrentPreview(event)) preview = event
                        NativeEvent.RequestNotifications -> if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
                            checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
                        ) {
                            if (permissionHistory.getBoolean("notification_requested", false) && !shouldShowRequestPermissionRationale(Manifest.permission.POST_NOTIFICATIONS)) {
                                startActivity(Intent(Settings.ACTION_APP_NOTIFICATION_SETTINGS).putExtra(Settings.EXTRA_APP_PACKAGE, packageName))
                            } else {
                                permissionHistory.edit().putBoolean("notification_requested", true).apply()
                                notificationPermission.launch(Manifest.permission.POST_NOTIFICATIONS)
                            }
                        }
                    }
                }
            }
            LaunchedEffect(state.session?.id) {
                if (state.session != null) {
                    pendingNotificationJob?.let { id ->
                        pendingNotificationJob = null
                        viewModel.openJob(id)
                    }
                } else preview = null
            }
            val dark = when (state.preferences.themeMode) {
                "light" -> false
                "dark" -> true
                else -> isSystemInDarkTheme()
            }
            DisposableEffect(dark) {
                val style = if (dark) SystemBarStyle.dark(android.graphics.Color.TRANSPARENT)
                    else SystemBarStyle.light(android.graphics.Color.TRANSPARENT, android.graphics.Color.TRANSPARENT)
                enableEdgeToEdge(statusBarStyle = style, navigationBarStyle = style)
                onDispose { }
            }
            LuxuryVideoAppTheme(darkTheme = dark, dynamicColor = false, motionEnabled = state.preferences.motionEnabled) {
                CaptureApp(state, viewModel)
                preview?.takeIf { !updateBlocksBusiness && viewModel.isPreviewSessionCurrent(it) }?.let { event ->
                    MediaPreview(
                        event,
                        onDismiss = { preview = null; viewModel.dismissPreview() },
                        onRetry = { active ->
                            if (event.gallery.isNotEmpty()) viewModel.previewGallery(active.gallery.map { it.file }, active.galleryIndex)
                            else viewModel.preview(active.file, refreshMusic = active.file.kind == "music")
                        },
                        onDownload = { active -> preview = null; viewModel.dismissPreview(); viewModel.download(active.file, state.selectedJobAdmin) },
                    )
                }
                if (updatePromptActive) AppUpdateDialog(updateState, viewModel.appUpdates)
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        consumeIntent(intent)
    }

    override fun onSaveInstanceState(outState: Bundle) {
        outState.putString("pending_notification_job", pendingNotificationJob)
        super.onSaveInstanceState(outState)
    }

    override fun onStart() {
        super.onStart()
        viewModel.setForeground(true)
    }

    override fun onStop() {
        viewModel.setForeground(false)
        super.onStop()
    }

    override fun onResume() {
        super.onResume()
        viewModel.appUpdates.onResume(this)
    }

    private fun consumeIntent(intent: Intent) {
        if (intent.action == Intent.ACTION_SEND && intent.type?.startsWith("text/") == true) {
            intent.getCharSequenceExtra(Intent.EXTRA_TEXT)?.toString()?.takeIf { it.isNotBlank() }?.let(viewModel::acceptSharedText)
        }
        intent.getStringExtra("job_id")?.let { id ->
            if (viewModel.state.value.session != null) viewModel.openJob(id) else pendingNotificationJob = id
        }
    }
}
