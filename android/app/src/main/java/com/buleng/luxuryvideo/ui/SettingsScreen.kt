package com.buleng.luxuryvideo.ui

import android.Manifest
import android.app.DownloadManager
import android.app.NotificationManager
import android.content.Intent
import android.content.Context
import android.content.ContextWrapper
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.provider.Settings
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.core.content.ContextCompat
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.LifecycleOwner
import com.buleng.luxuryvideo.CaptureUiState
import com.buleng.luxuryvideo.CaptureViewModel
import com.buleng.luxuryvideo.data.formatTime
import com.buleng.luxuryvideo.data.text
import org.json.JSONObject

@Composable
fun SettingsScreen(state: CaptureUiState, vm: CaptureViewModel, onDownloads: () -> Unit = {}, onSettingsSection: (String) -> Unit = {}) {
    val context = LocalContext.current
    val preferences = state.preferences
    var server by rememberSaveable(preferences.serverUrl) { mutableStateOf(preferences.serverUrl) }
    var passwordDialog by remember { mutableStateOf(false) }
    var sessionsDialog by remember { mutableStateOf(false) }
    var confirm by remember { mutableStateOf<String?>(null) }
    fun saveTheme(theme: String) {
        vm.savePreferences(preferences.serverUrl, theme, preferences.motionEnabled, preferences.notificationsEnabled)
    }
    PageColumn {

        EnterContent(1) {
            ArcoCard {
                Row(horizontalArrangement = Arrangement.spacedBy(14.dp), verticalAlignment = Alignment.CenterVertically) {
                    Box(Modifier.size(58.dp).background(MaterialTheme.colorScheme.primaryContainer, CircleShape), contentAlignment = Alignment.Center) {
                        Text(state.session?.displayName?.take(1).orEmpty().uppercase(), style = MaterialTheme.typography.headlineSmall, color = MaterialTheme.colorScheme.primary)
                    }
                    Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                        Text(state.session?.displayName.orEmpty(), style = MaterialTheme.typography.titleLarge)
                        ArcoTag(if (state.session?.isCdk == true) "CDK 账号" else if (state.session?.role == "admin") "管理员" else "普通用户")
                    }
                    ArcoIcon("shield", tint = MaterialTheme.colorScheme.primary, size = 26.dp)
                }
                HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
                SettingsAction("登录设备", "", "monitor") { vm.loadSessions(); sessionsDialog = true }
                if (state.session?.isCdk != true) SettingsAction("修改密码", "", "lock") { passwordDialog = true }
            }
        }
        ArcoCard {
            SectionHeading("服务与管理")
            SettingsAction("下载管理", "下载进度与本机文件", "download", onDownloads)
            HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
            SettingsAction("使用设置", if (state.session?.role == "admin") "本机体验与服务端权限" else "页面动效与任务通知", "settings") { onSettingsSection("usage") }
            HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
            SettingsAction("检查更新", "当前版本 " + vm.appUpdates.state.value.currentVersion.name, "refresh") { vm.appUpdates.check(manual = true) }
            if (state.session?.role == "admin") {
                HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
                SettingsAction("转写服务", "服务配置与默认服务", "wave") { onSettingsSection("providers") }
                HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
                SettingsAction("API 目录", "接口说明与调用示例", "code") { onSettingsSection("api") }
            }
        }
        EnterContent(2) {
            ArcoCard {
                SectionHeading("外观")
                Text("主题模式", style = MaterialTheme.typography.titleSmall)
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    ChoicePill("跟随系统", preferences.themeMode == "system", Modifier.weight(1f), icon = "monitor", centerContent = true) { saveTheme("system") }
                    ChoicePill("浅色", preferences.themeMode == "light", Modifier.weight(1f), icon = "sun", centerContent = true) { saveTheme("light") }
                    ChoicePill("深色", preferences.themeMode == "dark", Modifier.weight(1f), icon = "moon", centerContent = true) { saveTheme("dark") }
                }
            }
        }
        EnterContent(3) {
            ArcoCard {
                SectionHeading("服务器")
                ArcoInput(server, { server = it }, "服务器地址", hint = "https://capture.example.com", icon = "server", keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri))
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    ArcoButton("测试连接", icon = "refresh", kind = ArcoButtonKind.Secondary, enabled = server.isNotBlank() && !state.busy, modifier = Modifier.weight(1f)) { vm.checkConnection(server) }
                    ArcoButton("保存地址", icon = "check", enabled = server.isNotBlank() && server.trimEnd('/') != preferences.serverUrl.trimEnd('/'), modifier = Modifier.weight(1f)) { confirm = "server" }
                }
                if (state.serverOnline != null) {
                    Row(horizontalArrangement = Arrangement.spacedBy(7.dp), verticalAlignment = Alignment.CenterVertically) {
                        Box(Modifier.size(6.dp).background(if (state.serverOnline) com.buleng.luxuryvideo.ui.theme.ArcoGreen else MaterialTheme.colorScheme.error, CircleShape))
                        Text(if (state.serverOnline) "服务器可连接${if (state.serverVersion.isBlank()) "" else " · ${state.serverVersion}"}" else "暂时无法连接服务器", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                }
            }
        }
        EnterContent(4) {
            ArcoCard {
                SectionHeading("本地存储")
                SettingsAction("系统下载目录", "Downloads / LuxuryCapture", "document") {
                    runCatching { context.startActivity(Intent(DownloadManager.ACTION_VIEW_DOWNLOADS)) }
                }
            }
        }
        ArcoButton("退出当前账号", Modifier.fillMaxWidth(), icon = "logout", kind = ArcoButtonKind.Secondary, enabled = !state.busy) { confirm = "logout" }
        Spacer(Modifier.height(6.dp))
    }
    if (confirm != null) {
        val switching = confirm == "server"
        AlertDialog(
            onDismissRequest = { confirm = null },
            title = { Text(if (switching) "切换服务器？" else "退出登录？") },
            text = { Text(if (switching) "切换后将退出当前账号并清除本机的账号历史缓存；服务器中的任务和已下载文件会保留。" else "账号会话将从本机移除，服务器中的任务会保留。") },
            confirmButton = { TextButton(onClick = {
                confirm = null
                if (switching) vm.savePreferences(server, preferences.themeMode, preferences.motionEnabled, preferences.notificationsEnabled) else vm.logout()
            }) { Text(if (switching) "切换并重新登录" else "退出登录") } },
            dismissButton = { TextButton(onClick = { confirm = null }) { Text("取消") } },
        )
    }
    if (passwordDialog) PasswordDialog(state.busy, onDismiss = { passwordDialog = false }, onSubmit = { current, next -> vm.changePassword(current, next); passwordDialog = false })
    if (sessionsDialog) SessionsDialog(state, vm, onDismiss = { sessionsDialog = false })
}

@Composable
fun AppUsageSettings(state: CaptureUiState, vm: CaptureViewModel) {
    val context = LocalContext.current
    val preferences = state.preferences
    val permissionHistory = remember(context) { context.getSharedPreferences("capture_permissions", Context.MODE_PRIVATE) }
    var notificationDenied by remember { mutableStateOf(false) }
    fun notificationsGranted(): Boolean {
        val enabledBySystem = context.getSystemService(NotificationManager::class.java).areNotificationsEnabled()
        val permissionGranted = Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU ||
            ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED
        return enabledBySystem && permissionGranted
    }
    var notificationGranted by remember(context) { mutableStateOf(notificationsGranted()) }
    DisposableEffect(context) {
        val lifecycle = context.settingsLifecycleOwner()?.lifecycle
        val observer = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_RESUME) {
                notificationGranted = notificationsGranted()
                if (notificationGranted) notificationDenied = false
            }
        }
        lifecycle?.addObserver(observer)
        onDispose { lifecycle?.removeObserver(observer) }
    }
    fun save(motion: Boolean = preferences.motionEnabled, notifications: Boolean = preferences.notificationsEnabled) {
        vm.savePreferences(preferences.serverUrl, preferences.themeMode, motion, notifications)
    }
    val notificationPermission = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
        notificationGranted = granted && notificationsGranted()
        notificationDenied = !notificationGranted
        save(notifications = granted)
    }
    ArcoCard(modifier = Modifier.fillMaxWidth()) {
        SectionHeading("本机设置", "仅影响当前设备，修改后立即生效")
        SettingSwitch("页面动效", "页面与操作的动画反馈", preferences.motionEnabled, { save(motion = it) })
        HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
        SettingSwitch("任务通知", "任务完成或失败时提醒", preferences.notificationsEnabled && notificationGranted, { enabled ->
            notificationGranted = notificationsGranted()
            when {
                !enabled -> { notificationDenied = false; save(notifications = false) }
                Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
                    ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED -> {
                    val activity = context.settingsLifecycleOwner() as? android.app.Activity
                    if (permissionHistory.getBoolean("notification_requested", false) && activity?.shouldShowRequestPermissionRationale(Manifest.permission.POST_NOTIFICATIONS) == false) {
                        notificationDenied = true
                        save(notifications = true)
                        context.openNotificationSettings()
                    } else {
                        permissionHistory.edit().putBoolean("notification_requested", true).apply()
                        notificationPermission.launch(Manifest.permission.POST_NOTIFICATIONS)
                    }
                }
                !notificationGranted -> {
                    notificationDenied = true
                    save(notifications = true)
                    context.openNotificationSettings()
                }
                else -> { notificationDenied = false; save(notifications = true) }
            }
        })
        if (notificationDenied || preferences.notificationsEnabled && !notificationGranted) {
            InfoBanner("系统通知尚未开启，允许通知后才能接收任务提醒。")
            TextButton(onClick = { context.openNotificationSettings() }) { Text("打开系统通知设置") }
        }
    }
}

private tailrec fun Context.settingsLifecycleOwner(): LifecycleOwner? = when (this) {
    is LifecycleOwner -> this
    is ContextWrapper -> if (baseContext === this) null else baseContext.settingsLifecycleOwner()
    else -> null
}

private fun Context.openNotificationSettings() {
    runCatching {
        startActivity(Intent(Settings.ACTION_APP_NOTIFICATION_SETTINGS).apply {
            putExtra(Settings.EXTRA_APP_PACKAGE, packageName)
            addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        })
    }.recoverCatching {
        startActivity(Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS, Uri.parse("package:$packageName")).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
    }
}

@Composable
private fun SettingsAction(title: String, description: String, icon: String, onClick: () -> Unit) {
    Surface(onClick = onClick, color = MaterialTheme.colorScheme.surface, shape = RoundedCornerShape(8.dp)) {
        Row(Modifier.fillMaxWidth().heightIn(min = 56.dp).padding(horizontal = 12.dp, vertical = 10.dp), horizontalArrangement = Arrangement.spacedBy(11.dp), verticalAlignment = Alignment.CenterVertically) {
            ArcoIcon(icon, size = 20.dp, tint = MaterialTheme.colorScheme.primary)
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(title, style = MaterialTheme.typography.bodyMedium)
                if (description.isNotBlank()) Text(description, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            ArcoIcon("chevron", size = 16.dp)
        }
    }
}

@Composable
fun PasswordDialog(busy: Boolean, onDismiss: () -> Unit, onSubmit: (String, String) -> Unit) {
    var current by remember { mutableStateOf("") }
    var next by remember { mutableStateOf("") }
    var repeat by remember { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = { if (!busy) onDismiss() },
        title = { Text("修改登录密码") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(14.dp)) {
                Text("新密码长度为 6–12 个字符。", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                ArcoInput(current, { current = it }, "当前密码", secret = true, enabled = !busy)
                ArcoInput(next, { next = it }, "新密码", secret = true, enabled = !busy)
                ArcoInput(repeat, { repeat = it }, "再次输入新密码", secret = true, enabled = !busy)
                if (repeat.isNotEmpty() && repeat != next) Text("两次输入的新密码不一致", color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
            }
        },
        confirmButton = { TextButton(onClick = { onSubmit(current, next) }, enabled = !busy && current.isNotEmpty() && next.length in 6..12 && next == repeat) { Text(if (busy) "正在保存" else "保存新密码") } },
        dismissButton = { TextButton(onClick = onDismiss, enabled = !busy) { Text("取消") } },
    )
}

@Composable
private fun SessionsDialog(state: CaptureUiState, vm: CaptureViewModel, onDismiss: () -> Unit) {
    var revoke by remember { mutableStateOf<JSONObject?>(null) }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { SectionHeading("登录设备", "移除设备后，其登录状态会失效") { ArcoIconButton("refresh", "刷新登录设备") { vm.loadSessions() } } },
        text = {
            Column(Modifier.fillMaxWidth().heightIn(max = 430.dp).verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(16.dp)) {
                if (state.sessions.isEmpty()) Text(if (state.busy) "正在读取设备会话…" else "没有可显示的设备会话", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                state.sessions.forEachIndexed { index, session ->
                    if (index > 0) HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
                    val device = session.optJSONObject("device")
                    Row(verticalAlignment = Alignment.Top, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                        ArcoIcon("monitor", size = 20.dp, tint = MaterialTheme.colorScheme.primary)
                        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                            Text(device?.text("name").orEmpty().ifBlank { "未命名设备" }, style = MaterialTheme.typography.titleSmall, maxLines = 2, overflow = TextOverflow.Ellipsis)
                            Text(device?.text("platform").orEmpty(), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                            if (session.optBoolean("current")) ArcoTag("当前设备")
                            Text("最近使用 ${formatTime(session.text("lastUsedAt"))}", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                            Text("登录时间 ${formatTime(session.text("createdAt"))}", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                        ArcoIconButton("logout", "移除此设备", enabled = !state.busy) { revoke = session }
                    }
                }
            }
        },
        confirmButton = { TextButton(onClick = onDismiss) { Text("完成") } },
    )
    revoke?.let { session ->
        AlertDialog(
            onDismissRequest = { revoke = null },
            title = { Text("移除这个登录设备？") },
            text = { Text(if (session.optBoolean("current")) "这是当前设备，移除后将退出登录。" else "这个设备需要重新登录才能继续访问。") },
            confirmButton = { TextButton(onClick = { revoke = null; vm.revokeSession(session.text("id")); if (session.optBoolean("current")) onDismiss() }) { Text("移除设备", color = MaterialTheme.colorScheme.error) } },
            dismissButton = { TextButton(onClick = { revoke = null }) { Text("取消") } },
        )
    }
}
