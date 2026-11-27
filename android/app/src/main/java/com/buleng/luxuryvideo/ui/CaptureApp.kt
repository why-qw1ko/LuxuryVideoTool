package com.buleng.luxuryvideo.ui

import androidx.activity.compose.BackHandler
import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.background
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.consumeWindowInsets
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FilterChip
import androidx.compose.material3.FilterChipDefaults
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.NavigationBarItemDefaults
import androidx.compose.material3.NavigationRail
import androidx.compose.material3.NavigationRailItem
import androidx.compose.material3.NavigationRailItemDefaults
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Snackbar
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.saveable.rememberSaveableStateHolder
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import com.buleng.luxuryvideo.CaptureUiState
import com.buleng.luxuryvideo.CaptureViewModel
import com.buleng.luxuryvideo.ui.theme.ArcoBlue
import com.buleng.luxuryvideo.ui.theme.LocalMotionEnabled
import com.buleng.luxuryvideo.ui.theme.LuxuryVideoAppTheme

private data class WorkspaceTab(val label: String, val icon: String, val id: Int)

@Composable
fun CaptureApp(state: CaptureUiState, vm: CaptureViewModel) {
    val updateState by vm.appUpdates.state.collectAsState()
    var selectedTab by rememberSaveable(state.session?.id) { mutableIntStateOf(0) }
    var accountSection by rememberSaveable(state.session?.id) { mutableStateOf<String?>(null) }
    val pageState = rememberSaveableStateHolder()
    var consumedShareId by rememberSaveable { androidx.compose.runtime.mutableLongStateOf(0L) }
    LaunchedEffect(state.externalShareId) {
        if (state.externalShareId != 0L && state.externalShareId != consumedShareId) {
            consumedShareId = state.externalShareId
            selectedTab = 0
            accountSection = null
        }
    }
    val tabs = buildList {
        add(WorkspaceTab("提取", "spark", 0))
        add(WorkspaceTab("任务", "history", 1))
        if (state.session?.role == "admin") add(WorkspaceTab("管理", "grid", 2))
        add(WorkspaceTab("我的", "user", 3))
    }
    val snackbar = remember { SnackbarHostState() }
    val dark = when (state.preferences.themeMode) {
        "light" -> false
        "dark" -> true
        else -> isSystemInDarkTheme()
    }
    val openDownloads = {
        vm.closeJob()
        selectedTab = 3
        accountSection = "downloads"
        vm.refreshDownloads()
    }
    val selectTab: (Int) -> Unit = { tab ->
        vm.closeJob()
        accountSection = null
        selectedTab = tab
        if (tab == 2 && state.adminSection !in setOf("overview", "jobs", "users", "cdks")) vm.loadAdmin("overview")
    }
    LaunchedEffect(state.error, state.notice) {
        val message = state.error ?: state.notice
        if (message != null) {
            snackbar.showSnackbar(message, withDismissAction = true)
            vm.clearMessage()
        }
    }
    LaunchedEffect(accountSection, state.session?.id) {
        if (state.session?.role == "admin" && accountSection in setOf("providers", "usage", "api") && state.adminSection != accountSection) {
            vm.loadAdmin(accountSection ?: return@LaunchedEffect)
        }
    }
    BackHandler(state.selectedJob != null) { vm.closeJob() }
    BackHandler(state.selectedJob == null && state.session != null && accountSection != null) { accountSection = null }
    BackHandler(state.selectedJob == null && state.session != null && accountSection == null && selectedTab != 0) { selectedTab = 0 }
    BoxWithConstraints(Modifier.fillMaxSize()) {
        val wide = maxWidth >= 840.dp
        Scaffold(
            containerColor = MaterialTheme.colorScheme.background,
            snackbarHost = {
                SnackbarHost(snackbar) { data ->
                    Snackbar(data, shape = RoundedCornerShape(12.dp), containerColor = MaterialTheme.colorScheme.inverseSurface)
                }
            },
            topBar = {
                if (!state.initializing && state.session != null) WorkspaceHeader(
                    title = when {
                        state.selectedJob != null -> "任务详情"
                        accountSection == "downloads" -> "下载管理"
                        accountSection == "providers" -> "转写服务"
                        accountSection == "usage" -> "使用设置"
                        accountSection == "api" -> "API 目录"
                        selectedTab == 1 -> "任务"
                        selectedTab == 2 -> "管理"
                        selectedTab == 3 -> "我的"
                        else -> "提取"
                    },
                    detail = state.selectedJob != null || accountSection != null,
                    serverOnline = state.serverOnline,
                    dark = dark,
                    onBack = { if (state.selectedJob != null) vm.closeJob() else accountSection = null },
                    onToggleTheme = {
                        val preferences = state.preferences
                        vm.savePreferences(preferences.serverUrl, if (dark) "light" else "dark", preferences.motionEnabled, preferences.notificationsEnabled)
                    },
                    onRefresh = when {
                        state.selectedJob != null -> ({ vm.openJob(state.selectedJob.id, state.selectedJobAdmin) })
                        accountSection == "downloads" -> vm::refreshDownloads
                        state.session.role == "admin" && accountSection in setOf("providers", "usage", "api") -> ({ accountSection?.let { vm.loadAdmin(it) }; Unit })
                        else -> null
                    },
                    refreshing = when {
                        state.selectedJob != null -> state.busy
                        accountSection in setOf("providers", "usage", "api") -> state.adminLoading
                        else -> false
                    },
                )
            },
            bottomBar = {
                Column {
                    if (state.session != null && accountSection != "downloads") ActiveDownloadsBar(vm, openDownloads)
                    if (!wide && state.session != null && state.selectedJob == null && accountSection == null) {
                        HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
                        NavigationBar(containerColor = MaterialTheme.colorScheme.surface, tonalElevation = 0.dp) {
                            tabs.forEach { tab ->
                                NavigationBarItem(
                                    selected = selectedTab == tab.id, onClick = { selectTab(tab.id) },
                                    icon = { ArcoIcon(tab.icon, tint = if (selectedTab == tab.id) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurfaceVariant) },
                                    label = { Text(tab.label, style = MaterialTheme.typography.labelMedium) },
                                    colors = NavigationBarItemDefaults.colors(
                                        selectedTextColor = MaterialTheme.colorScheme.primary,
                                        indicatorColor = MaterialTheme.colorScheme.primaryContainer,
                                    ),
                                )
                            }
                        }
                    }
                }
            },
        ) { padding ->
            Row(Modifier.fillMaxSize().padding(padding).consumeWindowInsets(padding)) {
                if (wide && state.session != null) {
                    NavigationRail(containerColor = MaterialTheme.colorScheme.surface, modifier = Modifier.fillMaxHeight().width(100.dp)) {
                        Spacer(Modifier.height(20.dp))
                        tabs.forEach { tab ->
                            NavigationRailItem(
                                selected = selectedTab == tab.id && state.selectedJob == null,
                                onClick = { selectTab(tab.id) },
                                icon = { ArcoIcon(tab.icon, tint = if (selectedTab == tab.id) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurfaceVariant) },
                                label = { Text(tab.label) },
                                colors = NavigationRailItemDefaults.colors(indicatorColor = MaterialTheme.colorScheme.primaryContainer),
                                modifier = Modifier.padding(vertical = 8.dp),
                            )
                        }
                    }
                }
                val screen = when {
                    state.initializing -> "initializing"
                    state.session == null -> "login"
                    state.selectedJob != null -> "job-${state.selectedJob.id}"
                    accountSection != null -> "account-$accountSection"
                    else -> "tab-$selectedTab"
                }
                Box(Modifier.weight(1f).fillMaxHeight()) {
                    when {
                        screen == "initializing" -> StartupScreen()
                        screen == "login" -> LoginScreen(state, vm)
                        screen.startsWith("job-") -> state.selectedJob?.let { JobDetailScreen(it, state, vm) }
                        screen == "account-downloads" -> DownloadsScreen(vm)
                        screen == "account-usage" && state.session?.role != "admin" -> PageColumn { AppUsageSettings(state, vm) }
                        screen.startsWith("account-") -> if (state.adminSection == accountSection) AdminPanel(state, vm, showNavigation = false) else LoadingCards()
                        else -> pageState.SaveableStateProvider("${state.session?.id}-$screen") {
                            when (selectedTab) {
                                0 -> CaptureScreen(state, vm, onHistory = { selectTab(1) })
                                1 -> HistoryScreen(state, vm)
                                2 -> AdminPanel(state, vm)
                                3 -> SettingsScreen(state, vm, onDownloads = openDownloads, onSettingsSection = { section -> accountSection = section })
                            }
                        }
                    }
                }
            }
        }
    }
    state.pendingDownload?.takeUnless { updateState.forced }?.let { selection ->
        DownloadConfirmation(selection, state.downloadPreparing, onDismiss = vm::dismissDownload, onConfirm = vm::confirmDownload)
    }
}

@Composable
private fun WorkspaceHeader(
    title: String,
    detail: Boolean,
    serverOnline: Boolean?,
    dark: Boolean,
    onBack: () -> Unit,
    onToggleTheme: () -> Unit,
    onRefresh: (() -> Unit)?,
    refreshing: Boolean,
) {
    Surface(color = MaterialTheme.colorScheme.surface) {
        Row(
            Modifier.fillMaxWidth().statusBarsPadding().heightIn(min = 72.dp).padding(start = if (detail) 6.dp else 20.dp, end = 8.dp, top = 10.dp, bottom = 10.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically,
        ) {
            if (detail) ArcoIconButton("back", "返回", onClick = onBack)
            Text(title, Modifier.weight(1f), style = MaterialTheme.typography.titleLarge)
            if (serverOnline == false) Box(Modifier.size(6.dp).background(MaterialTheme.colorScheme.error, CircleShape))
            if (onRefresh != null) ArcoIconButton("refresh", "刷新", enabled = !refreshing, onClick = onRefresh)
            else if (!detail) ArcoIconButton(if (dark) "sun" else "moon", if (dark) "切换为浅色主题" else "切换为深色主题", onClick = onToggleTheme)
        }
    }
}

@Composable
fun BrandMark(size: androidx.compose.ui.unit.Dp = 48.dp) {
    Box(
        Modifier.size(size).background(Brush.linearGradient(listOf(ArcoBlue, Color(0xFF4B89FF))), RoundedCornerShape(size * .29f)),
        contentAlignment = Alignment.Center,
    ) { ArcoIcon("brand", tint = Color.White, size = size * .64f) }
}

@Composable
fun PageColumn(modifier: Modifier = Modifier, content: @Composable ColumnScope.() -> Unit) {
    Box(modifier.fillMaxSize(), contentAlignment = Alignment.TopCenter) {
        Column(
            Modifier.widthIn(max = 960.dp).fillMaxWidth().verticalScroll(rememberScrollState()).imePadding()
                .padding(horizontal = 16.dp, vertical = 16.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp), content = content,
        )
    }
}

@Composable
fun ArcoInput(
    value: String,
    onValueChange: (String) -> Unit,
    label: String,
    modifier: Modifier = Modifier,
    hint: String = "",
    icon: String? = null,
    lines: Int = 1,
    secret: Boolean = false,
    enabled: Boolean = true,
    keyboardOptions: KeyboardOptions = KeyboardOptions.Default,
    keyboardActions: KeyboardActions = KeyboardActions.Default,
) {
    var revealed by remember { mutableStateOf(false) }
    OutlinedTextField(
        value = value, onValueChange = onValueChange, modifier = modifier.fillMaxWidth(), enabled = enabled,
        label = { Text(label) }, placeholder = { Text(hint) },
        leadingIcon = if (icon != null) ({ ArcoIcon(icon, size = 19.dp) }) else null,
        trailingIcon = if (secret) ({ ArcoIconButton("eye", if (revealed) "隐藏内容" else "显示内容") { revealed = !revealed } }) else null,
        visualTransformation = if (secret && !revealed) PasswordVisualTransformation() else VisualTransformation.None,
        singleLine = lines == 1, minLines = lines, maxLines = if (lines == 1) 1 else 8,
        textStyle = MaterialTheme.typography.bodyLarge, shape = RoundedCornerShape(10.dp),
        keyboardOptions = keyboardOptions, keyboardActions = keyboardActions,
        colors = OutlinedTextFieldDefaults.colors(
            focusedContainerColor = MaterialTheme.colorScheme.surface,
            unfocusedContainerColor = MaterialTheme.colorScheme.surfaceContainerLow,
            unfocusedBorderColor = MaterialTheme.colorScheme.outlineVariant,
            focusedBorderColor = MaterialTheme.colorScheme.primary,
            focusedLabelColor = MaterialTheme.colorScheme.primary,
        ),
    )
}

@Composable
fun ChoicePill(label: String, selected: Boolean, modifier: Modifier = Modifier, enabled: Boolean = true, icon: String? = null, centerContent: Boolean = false, onClick: () -> Unit) {
    FilterChip(
        selected = selected, onClick = onClick, enabled = enabled, modifier = modifier.height(if (centerContent) 64.dp else 42.dp),
        label = {
            if (centerContent) Column(Modifier.fillMaxWidth(), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(4.dp)) {
                if (icon != null) ArcoIcon(icon, size = 18.dp, tint = if (selected) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurfaceVariant)
                Text(label, style = MaterialTheme.typography.labelMedium, maxLines = 1)
            } else Text(label, style = MaterialTheme.typography.labelLarge)
        },
        leadingIcon = if (!centerContent && icon != null) ({ ArcoIcon(icon, size = 17.dp, tint = if (selected) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurfaceVariant) }) else null,
        shape = RoundedCornerShape(9.dp),
        colors = FilterChipDefaults.filterChipColors(
            containerColor = MaterialTheme.colorScheme.surface,
            selectedContainerColor = MaterialTheme.colorScheme.primaryContainer,
            selectedLabelColor = MaterialTheme.colorScheme.primary,
            labelColor = MaterialTheme.colorScheme.onSurfaceVariant,
        ),
        border = FilterChipDefaults.filterChipBorder(
            enabled = enabled, selected = selected,
            borderColor = MaterialTheme.colorScheme.outlineVariant,
            selectedBorderColor = MaterialTheme.colorScheme.primary.copy(alpha = .25f),
        ),
    )
}

@Composable
private fun StartupScreen() {
    val motion = LocalMotionEnabled.current
    Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(20.dp)) {
            Box(Modifier.size(72.dp), contentAlignment = Alignment.Center) {
                if (motion) CircularProgressIndicator(Modifier.size(64.dp), strokeWidth = 3.dp, trackColor = MaterialTheme.colorScheme.primaryContainer)
                else CircularProgressIndicator(progress = { .72f }, modifier = Modifier.size(64.dp), strokeWidth = 3.dp, trackColor = MaterialTheme.colorScheme.primaryContainer)
                ArcoIcon("spark", size = 26.dp, tint = MaterialTheme.colorScheme.primary)
            }
            Text("Luxury Capture", style = MaterialTheme.typography.headlineSmall)
            Text("正在准备工作空间", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}

@Composable
private fun LoginScreen(state: CaptureUiState, vm: CaptureViewModel) {
    val motion = LocalMotionEnabled.current
    var cdkLogin by rememberSaveable { mutableStateOf(false) }
    var server by rememberSaveable(state.preferences.serverUrl) { mutableStateOf(state.preferences.serverUrl) }
    var username by rememberSaveable { mutableStateOf("") }
    var password by remember { mutableStateOf("") }
    var code by remember { mutableStateOf("") }
    val submit = { vm.login(server, username.trim(), password, if (cdkLogin) code.trim() else "") }
    BoxWithConstraints(Modifier.fillMaxSize().statusBarsPadding().navigationBarsPadding().imePadding(), contentAlignment = Alignment.Center) {
        val viewportHeight = maxHeight
        Column(
            Modifier.widthIn(max = 500.dp).fillMaxWidth().verticalScroll(rememberScrollState()).heightIn(min = viewportHeight).padding(horizontal = 24.dp, vertical = 24.dp),
            verticalArrangement = Arrangement.spacedBy(24.dp, Alignment.CenterVertically),
        ) {
            EnterContent {
                Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(12.dp, Alignment.CenterHorizontally), verticalAlignment = Alignment.CenterVertically) {
                    BrandMark(48.dp)
                    Text("Luxury Capture", style = MaterialTheme.typography.headlineSmall)
                }
            }
            EnterContent(1) {
                ArcoCard {
                    SectionHeading("登录")
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        ChoicePill("账号登录", !cdkLogin, Modifier.weight(1f), icon = "user") { cdkLogin = false }
                        ChoicePill("CDK 登录", cdkLogin, Modifier.weight(1f), icon = "key") { cdkLogin = true }
                    }
                    ArcoInput(server, { server = it }, "服务器地址", hint = "https://capture.example.com", icon = "server", enabled = !state.authenticating, keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri))
                    AnimatedContent(
                        targetState = cdkLogin,
                        transitionSpec = { fadeIn(tween(if (motion) 220 else 0)).togetherWith(fadeOut(tween(if (motion) 120 else 0))) },
                        label = "login method",
                    ) { isCdk ->
                        Column(verticalArrangement = Arrangement.spacedBy(14.dp)) {
                            if (isCdk) ArcoInput(code, { code = it }, "CDK 密钥", hint = "输入管理员提供的 CDK", icon = "key", secret = true, enabled = !state.authenticating, keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done), keyboardActions = KeyboardActions(onDone = { submit() }))
                            else {
                                ArcoInput(username, { username = it }, "用户名", hint = "输入用户名", icon = "user", enabled = !state.authenticating, keyboardOptions = KeyboardOptions(imeAction = ImeAction.Next))
                                ArcoInput(password, { password = it }, "密码", hint = "输入登录密码", icon = "lock", secret = true, enabled = !state.authenticating, keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password, imeAction = ImeAction.Done), keyboardActions = KeyboardActions(onDone = { submit() }))
                            }
                        }
                    }
                    ArcoButton(
                        "登录", Modifier.fillMaxWidth(), icon = "arrow", loading = state.authenticating,
                        enabled = server.isNotBlank() && (if (cdkLogin) code.isNotBlank() else username.isNotBlank() && password.isNotEmpty()),
                        onClick = submit,
                    )
                    if (state.error != null) InfoBanner(state.error, error = true)

                }
            }

        }
    }
}

@Preview(showBackground = true, widthDp = 390, heightDp = 500)
@Composable
private fun BrandPreview() {
    LuxuryVideoAppTheme {
        Surface(color = MaterialTheme.colorScheme.background) {
            PageColumn {
                BrandMark(56.dp)
                Text("提取", style = MaterialTheme.typography.headlineSmall)
                ArcoCard {
                    SectionHeading("新建任务")
                    ArcoInput("", {}, "分享链接或完整分享文本", lines = 3)
                    ArcoButton("开始提取", Modifier.fillMaxWidth(), icon = "spark") {}
                }
            }
        }
    }
}
