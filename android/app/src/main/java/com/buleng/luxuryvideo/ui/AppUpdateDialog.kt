package com.buleng.luxuryvideo.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.DialogProperties
import com.buleng.luxuryvideo.data.formatBytes
import com.buleng.luxuryvideo.platform.AppUpdateController
import com.buleng.luxuryvideo.platform.AppUpdateState

@Composable
fun AppUpdateDialog(state: AppUpdateState, controller: AppUpdateController) {
    if (!state.visible && !state.forced) return
    val context = LocalContext.current
    val release = state.release
    val busy = state.downloading || state.installing
    AlertDialog(
        onDismissRequest = controller::dismiss,
        properties = DialogProperties(dismissOnBackPress = !state.forced, dismissOnClickOutside = !state.forced),
        title = {
            Text(when {
                release != null -> (if (state.forced) "请更新至 " else "发现新版本 ") + release.versionName
                else -> "检查更新"
            })
        },
        text = {
            Column(
                Modifier.heightIn(max = 440.dp).verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                Text("当前版本 " + state.currentVersion.name + "（" + state.currentVersion.code + "）",
                    style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                if (state.forced) Text("请安装新版后继续使用。")
                if (release != null) {
                    Text("新版 " + release.versionName + " · " + formatBytes(release.size),
                        style = MaterialTheme.typography.bodyMedium)
                    if (release.changelog.isNotBlank()) Text(release.changelog)
                }
                if (state.checking) {
                    LinearProgressIndicator(Modifier.fillMaxWidth())
                    Text("正在检查更新…", style = MaterialTheme.typography.bodySmall)
                }
                if (state.downloading && release != null) {
                    LinearProgressIndicator(progress = { (state.downloadedBytes.toFloat() / release.size).coerceIn(0f, 1f) },
                        modifier = Modifier.fillMaxWidth())
                    Text("正在下载 " + formatBytes(state.downloadedBytes) + " / " + formatBytes(release.size),
                        style = MaterialTheme.typography.bodySmall)
                }
                if (state.installing) {
                    LinearProgressIndicator(Modifier.fillMaxWidth())
                    Text("正在校验安装包…", style = MaterialTheme.typography.bodySmall)
                }
                state.message?.let { Text(it, style = MaterialTheme.typography.bodySmall) }
                state.error?.let { Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error) }
                TextButton(onClick = { controller.check(manual = true) }, enabled = !state.checking && !state.installing) { Text("重新检查") }
            }
        },
        confirmButton = {
            if (release != null) {
                TextButton(
                    onClick = { if (state.apkReady) controller.install(context) else controller.download() },
                    enabled = !busy && !state.checking && !state.awaitingInstallPermission,
                ) { Text(if (state.apkReady) "安装新版" else "下载新版") }
            } else TextButton(onClick = controller::dismiss, enabled = !state.checking) { Text("知道了") }
        },
        dismissButton = {
            if (!state.forced && release != null) TextButton(onClick = controller::dismiss) { Text("取消更新") }
        },
    )
}
