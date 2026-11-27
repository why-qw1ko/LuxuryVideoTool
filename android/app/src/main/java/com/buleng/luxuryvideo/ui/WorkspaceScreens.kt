package com.buleng.luxuryvideo.ui

import android.content.ClipboardManager
import android.content.Context
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.expandVertically
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.shrinkVertically
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.TextButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.buleng.luxuryvideo.CaptureUiState
import com.buleng.luxuryvideo.CaptureViewModel
import com.buleng.luxuryvideo.data.CaptureJob
import com.buleng.luxuryvideo.data.formatTime
import com.buleng.luxuryvideo.data.objects
import com.buleng.luxuryvideo.data.text
import com.buleng.luxuryvideo.ui.theme.LocalMotionEnabled

fun providerName(id: String): String = when (id) {
    "siliconflow", "siliconflow_sensevoice" -> "硅基流动"
    "aliyun", "aliyun_paraformer" -> "阿里云"
    "deepseek", "deepseek_asr" -> "DeepSeek"
    else -> id
}

@Composable
fun CaptureScreen(state: CaptureUiState, vm: CaptureViewModel, onHistory: () -> Unit) {
    var action by rememberSaveable { mutableStateOf("download") }
    var force by rememberSaveable { mutableStateOf(false) }
    var advanced by rememberSaveable { mutableStateOf(false) }
    var provider by rememberSaveable { mutableStateOf("") }
    var languages by rememberSaveable { mutableStateOf("zh, en") }
    var hotwords by rememberSaveable { mutableStateOf("") }
    var clipboardEmpty by remember { mutableStateOf(false) }
    val context = LocalContext.current
    val motion = LocalMotionEnabled.current
    val usage = state.usage
    val asr = usage?.optJSONObject("asr")
    val providers = asr?.objects("providers").orEmpty().map { it.text("id") }
    val defaultProvider = asr?.text("defaultProvider").orEmpty()
    val allowChoice = asr?.optBoolean("allowUserSelection") == true
    val transcriptionAvailable = defaultProvider.isNotBlank() || allowChoice && providers.isNotEmpty()
    val allowedActions = buildList {
        if (usage?.optBoolean("allowInfo") == true) add("info")
        if (usage?.optBoolean("allowDownload") == true) add("download")
        if (usage?.optBoolean("allowTranscribe") == true && transcriptionAvailable) {
            add("full")
        }
    }
    LaunchedEffect(allowedActions) {
        if (action !in allowedActions && allowedActions.isNotEmpty()) action = if ("download" in allowedActions) "download" else allowedActions.first()
    }
    LaunchedEffect(providers, allowChoice) { if (!allowChoice || provider !in providers) provider = "" }
    val transcribing = action == "transcribe" || action == "full"
    val limit = usage?.optInt("limit") ?: 0
    val used = usage?.optInt("used") ?: 0
    val cdk = usage?.optJSONObject("cdk")
    val cdkLimit = cdk?.optInt("limit") ?: 0
    val cdkUsed = cdk?.optInt("used") ?: 0
    val quotaExhausted = limit > 0 && used >= limit || cdk != null && cdkLimit > 0 && cdkUsed >= cdkLimit
    Box(Modifier.fillMaxSize(), contentAlignment = Alignment.TopCenter) {
        LazyColumn(
            modifier = Modifier.widthIn(max = 960.dp).fillMaxSize().imePadding(),
            contentPadding = PaddingValues(horizontal = 16.dp, vertical = 20.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            item(key = "capture_form") {
                Column(Modifier.fillMaxWidth().clip(RoundedCornerShape(14.dp)).background(MaterialTheme.colorScheme.surface).padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    SectionHeading("作品链接") {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            ArcoIconButton("clipboard", "从剪贴板粘贴", enabled = !state.busy) {
                                val clipboard = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
                                val text = clipboard.primaryClip?.getItemAt(0)?.coerceToText(context)?.toString().orEmpty()
                                clipboardEmpty = text.isBlank()
                                if (text.isNotBlank()) vm.setShareText(text)
                            }
                            ArcoIconButton("clear", "清空输入", enabled = !state.busy && state.shareText.isNotEmpty()) {
                                clipboardEmpty = false
                                vm.setShareText("")
                            }
                        }
                    }
                    ArcoInput(
                        state.shareText, { clipboardEmpty = false; vm.setShareText(it) }, "分享链接或文本",
                        hint = "在抖音中复制分享链接，粘贴到这里", lines = 4, enabled = !state.busy,
                    )
                    val textBytes = state.shareText.toByteArray(Charsets.UTF_8).size
                    if (clipboardEmpty || textBytes > 3500) Row(Modifier.fillMaxWidth()) {
                        if (clipboardEmpty) Text("剪贴板中没有文本", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error)
                        Spacer(Modifier.weight(1f))
                        if (textBytes > 3500) Text("$textBytes/4096 字节", style = MaterialTheme.typography.bodySmall, color = if (textBytes > 4096) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                    Text("提取方式", style = MaterialTheme.typography.titleSmall)
                    val modes = listOf(
                        Triple("info", "info", "解析信息"),
                        Triple("download", "download", "图文 / 视频"),
                        Triple("full", "wave", "提取背景音文案"),
                    )
                    Row(
                        Modifier.fillMaxWidth().clip(RoundedCornerShape(12.dp))
                            .background(MaterialTheme.colorScheme.surfaceContainerLow).padding(4.dp).selectableGroup(),
                        horizontalArrangement = Arrangement.spacedBy(4.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        modes.forEach { (id, icon, label) ->
                            val selected = action == id
                            val enabled = id in allowedActions && !state.busy
                            val color = if (!enabled) MaterialTheme.colorScheme.outline
                                else if (selected) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurfaceVariant
                            Row(
                                (if (selected) Modifier.weight(1f) else Modifier.width(48.dp))
                                    .heightIn(min = 48.dp).clip(RoundedCornerShape(9.dp))
                                    .background(if (selected) MaterialTheme.colorScheme.surface else androidx.compose.ui.graphics.Color.Transparent)
                                    .selectable(selected, enabled = enabled, role = Role.RadioButton) { action = id }
                                    .semantics(mergeDescendants = true) { contentDescription = label }
                                    .padding(horizontal = if (selected) 8.dp else 0.dp, vertical = 10.dp),
                                horizontalArrangement = Arrangement.Center,
                                verticalAlignment = Alignment.CenterVertically,
                            ) {
                                ArcoIcon(icon, size = 19.dp, tint = color)
                                if (selected) {
                                    Spacer(Modifier.width(6.dp))
                                    Text(label, style = MaterialTheme.typography.labelMedium, color = color, maxLines = 2, overflow = TextOverflow.Ellipsis)
                                }
                            }
                        }
                    }
                    Text(
                        when (action) {
                            "info" -> "查看作品信息、作者和封面。"
                            "full" -> "识别背景音中的语音并生成文案，同时保留媒体。"
                            else -> "下载作品图片、视频和配乐。"
                        },
                        style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    if (usage == null) {
                        InfoBanner("尚未读取任务权限，请刷新后再提交。")
                        ArcoButton("重新读取权限", icon = "refresh", kind = ArcoButtonKind.Secondary, onClick = vm::refresh)
                    } else if (allowedActions.isEmpty()) InfoBanner("当前没有可用的提取方式，请联系管理员检查权限或转写服务。")
                    AnimatedVisibility(
                        visible = transcribing,
                        enter = fadeIn(tween(if (motion) 120 else 0)) + expandVertically(tween(if (motion) 180 else 0)),
                        exit = fadeOut(tween(if (motion) 90 else 0)) + shrinkVertically(tween(if (motion) 140 else 0)),
                    ) {
                        Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                            HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
                            if (allowChoice) {
                                Text("转写服务", style = MaterialTheme.typography.titleSmall)
                                Row(Modifier.horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                    if (defaultProvider.isNotBlank()) ChoicePill("默认 · ${providerName(defaultProvider)}", provider.isEmpty()) { provider = "" }
                                    providers.forEach { id -> ChoicePill(providerName(id), provider == id) { provider = id } }
                                }
                            } else if (defaultProvider.isNotBlank()) {
                                Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                                    ArcoIcon("wave", size = 18.dp, tint = MaterialTheme.colorScheme.primary)
                                    Text("转写服务：${providerName(defaultProvider)}", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                }
                            }
                            Text("转写会自动下载并保留媒体，完成后可下载媒体和文案。", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                    }
                    Row(
                        Modifier.fillMaxWidth().clip(RoundedCornerShape(10.dp)).clickable { advanced = !advanced }
                            .heightIn(min = 48.dp).padding(horizontal = 10.dp, vertical = 8.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        ArcoIcon("settings", size = 17.dp)
                        Spacer(Modifier.width(7.dp))
                        Text("高级选项", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        Spacer(Modifier.weight(1f))
                        ArcoIcon(if (advanced) "close" else "plus", size = 17.dp)
                    }
                    AnimatedVisibility(
                        visible = advanced,
                        enter = fadeIn(tween(if (motion) 120 else 0)) + expandVertically(tween(if (motion) 180 else 0)),
                        exit = fadeOut(tween(if (motion) 90 else 0)) + shrinkVertically(tween(if (motion) 140 else 0)),
                    ) {
                        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                            SettingSwitch("强制创建新任务", "不复用已有结果，将重新处理并计入额度", force, { force = it }, enabled = !state.busy)
                            if (transcribing) {
                                ArcoInput(languages, { languages = it }, "语言提示", hint = "使用逗号分隔，如 zh, en", enabled = !state.busy)
                                ArcoInput(hotwords, { hotwords = it }, "识别热词", hint = "多个热词使用逗号或换行分隔", lines = 2, enabled = !state.busy)
                            }
                        }
                    }
                    if (quotaExhausted) InfoBanner(if (cdk != null && cdkLimit > 0 && cdkUsed >= cdkLimit) "CDK 次数已用完，请联系管理员。" else "今日任务额度已用完，请在额度重置后再试。", error = true)
                    ArcoButton(
                        if (action == "info") "解析作品信息" else "开始提取", Modifier.fillMaxWidth(), icon = "spark", loading = state.busy,
                        enabled = !quotaExhausted && action in allowedActions && state.shareText.isNotBlank() && state.shareText.toByteArray(Charsets.UTF_8).size <= 4096 && (!transcribing || defaultProvider.isNotEmpty() || provider.isNotEmpty()),
                    ) { vm.createJob(action, transcribing, force, if (transcribing && allowChoice) provider else "", if (transcribing) languages else "", if (transcribing) hotwords else "") }
                }
            }
            if (usage != null) item(key = "capture_quota") {
                Column(Modifier.fillMaxWidth().padding(horizontal = 2.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                    Row(Modifier.fillMaxWidth()) {
                        Text(if (cdk == null) "今日用量" else "CDK 剩余", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        Spacer(Modifier.weight(1f))
                        val quotaText = if (cdk != null) {
                            if (cdkLimit > 0) "${(cdkLimit - cdkUsed).coerceAtLeast(0)} / $cdkLimit 次" else "不限次数"
                        } else if (limit > 0) "$used / $limit 次" else "已用 $used 次 · 不限额"
                        Text(quotaText, style = MaterialTheme.typography.bodySmall)
                    }
                    if (cdk == null && limit > 0) LinearProgressIndicator(progress = { (used.toFloat() / limit).coerceIn(0f, 1f) }, modifier = Modifier.fillMaxWidth(), color = MaterialTheme.colorScheme.primary, trackColor = MaterialTheme.colorScheme.primaryContainer)
                    if (cdk != null && limit > 0) Text("今日用量 $used / $limit 次", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    if (limit > 0 && usage.text("resetAt").isNotBlank()) Text("重置时间 ${formatTime(usage.text("resetAt"))}", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
            if (state.jobs.isNotEmpty()) {
                item(key = "recent_heading") {
                    SectionHeading("最近任务") { ArcoIconButton("arrow", "查看全部历史", onClick = onHistory) }
                }
                itemsIndexed(state.jobs.take(3), key = { _, job -> "recent_${job.id}" }) { _, job ->
                    JobCard(job, state.preferences.serverUrl) { vm.openJob(job.id) }
                }
            }
            item { Spacer(Modifier.height(4.dp)) }
        }
    }
}

@Composable
fun SettingSwitch(title: String, description: String, value: Boolean, onValueChange: (Boolean) -> Unit, enabled: Boolean = true) {
    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(14.dp), verticalAlignment = Alignment.CenterVertically) {
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(3.dp)) {
            Text(title, style = MaterialTheme.typography.bodyMedium, fontWeight = FontWeight.Medium)
            if (description.isNotBlank()) Text(description, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        Switch(value, onValueChange, enabled = enabled)
    }
}

@Composable
fun HistoryScreen(state: CaptureUiState, vm: CaptureViewModel) {
    var deleteCandidate by remember { mutableStateOf<CaptureJob?>(null) }
    var query by rememberSaveable(state.query) { mutableStateOf(state.query) }
    val statuses = listOf("" to "全部", "processing" to "处理中", "completed" to "已完成", "failed" to "失败", "queued" to "排队中", "retry_wait" to "等待重试", "cancelled" to "已取消")
    Box(Modifier.fillMaxSize(), contentAlignment = Alignment.TopCenter) {
        LazyColumn(
            Modifier.widthIn(max = 960.dp).fillMaxSize().imePadding(), contentPadding = PaddingValues(20.dp),
            verticalArrangement = Arrangement.spacedBy(0.dp),
        ) {
            item {
                Row(Modifier.fillMaxWidth().padding(bottom = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                    Text("共 ${state.totalJobs} 个任务", Modifier.weight(1f), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    TextButton(onClick = { vm.loadJobs(query = query) }, enabled = !state.loading) {
                        if (state.loading) CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp)
                        else ArcoIcon("refresh", size = 16.dp, tint = MaterialTheme.colorScheme.primary)
                        Spacer(Modifier.width(6.dp))
                        Text(if (state.loading) "同步中" else "刷新")
                    }
                }
            }
            item {
                Row(Modifier.padding(bottom = 12.dp), horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                    ArcoInput(query, { query = it }, "搜索标题、作者或链接", Modifier.weight(1f), icon = "search", keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search), keyboardActions = KeyboardActions(onSearch = { vm.loadJobs(query = query) }))
                    ArcoIconButton("search", "搜索任务", enabled = !state.loading) { vm.loadJobs(query = query) }
                }
            }
            item {
                Row(Modifier.padding(bottom = 8.dp).horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    statuses.forEach { (id, label) -> ChoicePill(label, state.statusFilter == id) { vm.loadJobs(query = query, status = id) } }
                }
            }
            item {
                Row(Modifier.padding(bottom = 12.dp).horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    ChoicePill("全部类型", state.actionFilter.isBlank(), icon = "filter") { vm.loadJobs(query = query, action = "") }
                    listOf("info" to "仅解析", "download" to "下载媒体", "transcription" to "文案转写").forEach { (id, label) -> ChoicePill(label, state.actionFilter == id) { vm.loadJobs(query = query, action = id) } }
                }
            }
            if (state.offline) item { InfoBanner("当前展示本机保存的历史。连接恢复后刷新即可同步。", icon = "server") }
            if (state.loading && state.jobs.isEmpty()) item { LoadingCards() }
            else if (state.jobs.isEmpty()) item {
                val unfiltered = query.isBlank() && state.statusFilter.isBlank() && state.actionFilter.isBlank()
                EmptyState(if (unfiltered) "暂无任务" else "没有匹配的任务", if (unfiltered) "粘贴作品链接以创建任务。" else "调整筛选条件后重试。", icon = "search")
            }
            itemsIndexed(state.jobs, key = { _, job -> job.id }, contentType = { _, _ -> "job" }) { index, job ->
                val corners = RoundedCornerShape(
                    topStart = if (index == 0) 12.dp else 0.dp, topEnd = if (index == 0) 12.dp else 0.dp,
                    bottomStart = if (index == state.jobs.lastIndex) 12.dp else 0.dp, bottomEnd = if (index == state.jobs.lastIndex) 12.dp else 0.dp,
                )
                Surface(shape = corners, color = MaterialTheme.colorScheme.surface) {
                    Column {
                        JobCard(job, state.preferences.serverUrl, onLongClick = { deleteCandidate = job }) { vm.openJob(job.id) }
                        if (index < state.jobs.lastIndex) HorizontalDivider(Modifier.padding(start = 76.dp), color = MaterialTheme.colorScheme.outlineVariant)
                    }
                }
            }
            item {
                Column(Modifier.fillMaxWidth().padding(top = 16.dp), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(10.dp)) {
                    if (state.hasMore) Surface(shape = RoundedCornerShape(12.dp), color = MaterialTheme.colorScheme.surface, modifier = Modifier.fillMaxWidth()) {
                        TextButton(onClick = { vm.loadJobs(append = true) }, enabled = !state.loading, modifier = Modifier.fillMaxWidth().height(52.dp), shape = RoundedCornerShape(12.dp)) {
                            if (state.loading) CircularProgressIndicator(Modifier.size(17.dp), strokeWidth = 2.dp)
                            else ArcoIcon("down", size = 18.dp, tint = MaterialTheme.colorScheme.primary)
                            Spacer(Modifier.width(8.dp))
                            Text(if (state.loading) "正在加载" else "加载更多 · 还剩 ${(state.totalJobs - state.jobs.size).coerceAtLeast(0)} 条")
                        }
                    }
                    else if (state.jobs.isNotEmpty()) Row(Modifier.fillMaxWidth().padding(vertical = 8.dp), horizontalArrangement = Arrangement.spacedBy(12.dp), verticalAlignment = Alignment.CenterVertically) {
                        HorizontalDivider(Modifier.weight(1f), color = MaterialTheme.colorScheme.outlineVariant)
                        Text("已显示全部 ${state.jobs.size} 条记录", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        HorizontalDivider(Modifier.weight(1f), color = MaterialTheme.colorScheme.outlineVariant)
                    }
                    Spacer(Modifier.height(12.dp))
                }
            }
        }
    }
    deleteCandidate?.let { selected ->
        val current = state.jobs.firstOrNull { it.id == selected.id } ?: selected
        AlertDialog(
            onDismissRequest = { deleteCandidate = null },
            title = { Text(if (current.terminal) "删除任务？" else "任务仍在处理中") },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text(current.title, maxLines = 3, overflow = TextOverflow.Ellipsis)
                    Text(if (current.terminal) "任务记录和服务器中的关联文件将被删除，已下载到手机的文件会保留。" else "请先进入任务详情取消处理，再删除任务。")
                }
            },
            confirmButton = {
                TextButton(enabled = !state.busy, onClick = {
                    deleteCandidate = null
                    if (current.terminal) vm.jobAction(current.id, "delete") else vm.openJob(current.id)
                }) { Text(if (current.terminal) "删除" else "查看任务", color = if (current.terminal) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.primary) }
            },
            dismissButton = { TextButton(onClick = { deleteCandidate = null }) { Text("取消") } },
        )
    }
}

@OptIn(ExperimentalFoundationApi::class)
@Composable
fun JobCard(job: CaptureJob, server: String, onLongClick: (() -> Unit)? = null, onClick: () -> Unit) {
    val motion = LocalMotionEnabled.current
    val progress = if (job.terminal) null else animateFloatAsState(job.progress / 100f, tween(if (motion) 180 else 0), label = "job progress")
    val createdAt = remember(job.createdAt) { formatTime(job.createdAt) }
    val coverUrl = remember(server, job.coverUrl) { mediaUrl(server, job.coverUrl) }
    val statusColor = when (job.status) {
        "completed" -> com.buleng.luxuryvideo.ui.theme.ArcoGreen
        "failed" -> MaterialTheme.colorScheme.error
        "cancelled" -> MaterialTheme.colorScheme.onSurfaceVariant
        else -> MaterialTheme.colorScheme.primary
    }
    Surface(modifier = Modifier.fillMaxWidth().padding(2.dp).heightIn(min = 80.dp), shape = RoundedCornerShape(10.dp), color = MaterialTheme.colorScheme.surface) {
        Row(Modifier.fillMaxWidth().combinedClickable(onClick = onClick, onLongClick = onLongClick, onLongClickLabel = if (onLongClick != null) "删除任务" else null).padding(horizontal = 14.dp, vertical = 12.dp), horizontalArrangement = Arrangement.spacedBy(12.dp), verticalAlignment = Alignment.CenterVertically) {
            if (job.coverUrl.isNotBlank()) RemoteImage(coverUrl, Modifier.size(48.dp).clip(RoundedCornerShape(8.dp)), description = "作品封面", maxDimension = 160)
            else Box(Modifier.size(48.dp).background(MaterialTheme.colorScheme.surfaceContainerHigh, RoundedCornerShape(8.dp)), contentAlignment = Alignment.Center) {
                ArcoIcon(if (job.workType == "note") "image" else "video", tint = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(job.title.ifBlank { "正在获取作品信息" }, style = MaterialTheme.typography.bodyMedium, fontWeight = FontWeight.Medium, maxLines = 2, overflow = TextOverflow.Ellipsis)
                Text(if (job.authorName.isBlank()) actionLabel(job.action) else "@${job.authorName} · ${actionLabel(job.action)}", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
                Text(createdAt, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                if (progress != null) LinearProgressIndicator(progress = { progress.value.coerceIn(0f, 1f) }, modifier = Modifier.fillMaxWidth().height(2.dp), color = MaterialTheme.colorScheme.primary, trackColor = MaterialTheme.colorScheme.surfaceContainerHigh)
            }
            Column(horizontalAlignment = Alignment.End, verticalArrangement = Arrangement.spacedBy(5.dp)) {
                Text(statusLabel(job.status), style = MaterialTheme.typography.labelSmall, color = statusColor)
                if (!job.terminal) Text("${job.progress}%", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
    }
}
