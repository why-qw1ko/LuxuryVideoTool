package com.buleng.luxuryvideo.ui

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.ContextWrapper
import android.os.PersistableBundle
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.EnterTransition
import androidx.compose.animation.ExitTransition
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.snap
import androidx.compose.animation.core.tween
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.selected
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleOwner
import androidx.lifecycle.repeatOnLifecycle
import com.buleng.luxuryvideo.AdminOperation
import com.buleng.luxuryvideo.CaptureUiState
import com.buleng.luxuryvideo.CaptureViewModel
import com.buleng.luxuryvideo.data.objects
import com.buleng.luxuryvideo.data.text
import com.buleng.luxuryvideo.ui.theme.LocalMotionEnabled
import org.json.JSONArray
import org.json.JSONObject
import java.net.URLEncoder
import java.time.OffsetDateTime
import java.time.format.DateTimeFormatter
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlin.math.max

private val adminSections = listOf(
    "overview" to "概览", "jobs" to "全部任务", "users" to "用户",
    "cdks" to "CDK",
)

private data class AdminConfirmation(
    val title: String,
    val message: String,
    val action: () -> Unit,
    val dangerous: Boolean = false,
)

@Composable
fun AdminPanel(state: CaptureUiState, vm: CaptureViewModel, showNavigation: Boolean = true) {
    val section = state.adminSection
    val context = LocalContext.current
    val lifecycle = remember(context) { adminLifecycle(context) }
    val latestState by rememberUpdatedState(state)
    var confirmation by remember { mutableStateOf<AdminConfirmation?>(null) }
    LaunchedEffect(Unit) {
        if (state.adminData.length() == 0 && !state.adminLoading) vm.loadAdmin(section)
    }
    LaunchedEffect(lifecycle, section, state.session?.id) {
        if (lifecycle == null || section !in setOf("overview", "jobs")) return@LaunchedEffect
        lifecycle.repeatOnLifecycle(Lifecycle.State.RESUMED) {
            while (isActive) {
                delay(8_000)
                val current = latestState
                if (!current.busy && !current.adminLoading && current.selectedJob == null) {
                    vm.loadAdmin(section, current.adminQuery, current.adminPage, current.adminData.optJSONObject("_filters"), refreshLoaded = true)
                }
            }
        }
    }
    Column(Modifier.fillMaxSize()) {
        if (showNavigation) Row(
            Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()).padding(horizontal = 16.dp, vertical = 8.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            adminSections.forEach { (id, title) ->
                ChoicePill(title, section == id, enabled = !state.busy) { if (section != id) vm.loadAdmin(id) }
            }
        }
        Box(Modifier.fillMaxWidth().height(4.dp)) {
            if (state.adminLoading) LinearProgressIndicator(Modifier.fillMaxWidth())
        }
        Box(Modifier.weight(1f)) {
            key(section) {
                when (section) {
                    "overview" -> AdminOverview(state, vm)
                    "users" -> AdminUsers(state, vm) { confirmation = it }
                    "jobs" -> AdminJobs(state, vm) { confirmation = it }
                    "cdks" -> AdminCdks(state, vm) { confirmation = it }
                    "providers" -> AdminProviders(state, vm) { confirmation = it }
                    "usage" -> AdminUsage(state, vm)
                    "api" -> AdminApiCatalog(state, vm)
                }
            }
        }
    }
    confirmation?.let { request ->
        AlertDialog(
            onDismissRequest = { if (!state.busy) confirmation = null },
            title = { Text(request.title) },
            text = { Text(request.message) },
            confirmButton = {
                TextButton(
                    enabled = !state.busy,
                    onClick = { confirmation = null; request.action() },
                    colors = ButtonDefaults.textButtonColors(contentColor = if (request.dangerous) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.primary),
                ) { Text("确认") }
            },
            dismissButton = { TextButton(enabled = !state.busy, onClick = { confirmation = null }) { Text("取消") } },
        )
    }
    AdminOperationResult(state, vm)
}

@Composable
private fun AdminOverview(state: CaptureUiState, vm: CaptureViewModel) {
    val motion = LocalMotionEnabled.current
    val stats = state.adminData.optJSONObject("stats") ?: JSONObject()
    val status = stats.optJSONObject("byStatus") ?: JSONObject()
    val active = listOf("queued", "resolving", "downloading", "extracting", "transcribing", "postprocessing", "retry_wait").sumOf { status.optInt(it) }
    val completed = status.optInt("completed")
    val failed = status.optInt("failed")
    LazyColumn(
        Modifier.fillMaxSize(), contentPadding = PaddingValues(20.dp), verticalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        item { AdminToolbar("概览", "") { vm.loadAdmin("overview") } }
        item {
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                AdminStat("今日任务", stats.optInt("todayJobs"), "累计 ${stats.optInt("totalJobs")} 个任务", Modifier.weight(1f))
                AdminStat("待处理", active, "${status.optInt("queued")} 个正在排队", Modifier.weight(1f))
            }
        }
        item {
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                AdminStat("已完成", completed, if (completed + failed > 0) "成功率 ${completed * 100 / (completed + failed)}%" else "暂无完成或失败任务", Modifier.weight(1f))
                AdminStat("启用账号", stats.optInt("activeUsers"), "共 ${stats.optInt("users")} 个账号", Modifier.weight(1f))
            }
        }
        item {
            AdminCard {
                Text("任务趋势", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
                AdminTrend(stats.objects("byDay").takeLast(14), motion)
            }
        }
        item {
            AdminCard {
                Text("状态分布", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
                listOf("completed", "queued", "resolving", "downloading", "extracting", "transcribing", "postprocessing", "retry_wait", "failed", "cancelled")
                    .filter { status.optInt(it) > 0 }.forEach { id ->
                        val count = status.optInt(id)
                        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                            Box(Modifier.size(8.dp).background(adminStatusColor(id), RoundedCornerShape(3.dp)))
                            Text(adminStatusLabel(id), Modifier.weight(1f).padding(start = 10.dp), fontSize = 13.sp)
                            Text(count.toString(), fontWeight = FontWeight.SemiBold)
                        }
                    }
                if (status.length() == 0) Text("尚无任务", color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
    }
}

@Composable
private fun AdminStat(title: String, count: Int, detail: String, modifier: Modifier = Modifier) {
    AdminCard(modifier) {
        Text(title, color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 12.sp)
        Text(count.toString(), fontSize = 30.sp, fontWeight = FontWeight.Bold, color = MaterialTheme.colorScheme.primary)
        Text(detail, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 2)
    }
}

@Composable
private fun AdminTrend(days: List<JSONObject>, motion: Boolean) {
    var visible by remember { mutableStateOf(false) }
    LaunchedEffect(Unit) { visible = true }
    val reveal = animateFloatAsState(if (visible) 1f else 0f, if (motion) tween(220) else snap(), label = "trendReveal")
    val primary = MaterialTheme.colorScheme.primary
    val grid = MaterialTheme.colorScheme.outlineVariant
    if (days.isEmpty()) {
        Text("暂无趋势数据", Modifier.padding(vertical = 40.dp), color = MaterialTheme.colorScheme.onSurfaceVariant)
        return
    }
    Canvas(
        Modifier.fillMaxWidth().height(150.dp).semantics {
            contentDescription = days.joinToString("，") { "${it.text("day")}：${it.optInt("count")}个任务" }
        },
    ) {
        val maxCount = max(1, days.maxOf { it.optInt("count") })
        repeat(4) { index ->
            val y = size.height * index / 3f
            drawLine(grid, Offset(0f, y), Offset(size.width, y), strokeWidth = 1.dp.toPx())
        }
        val segment = size.width / days.size
        days.forEachIndexed { index, day ->
            val barHeight = day.optInt("count").toFloat() / maxCount * (size.height - 6.dp.toPx()) * reveal.value
            drawRoundRect(
                color = primary.copy(alpha = 0.42f + 0.58f * (index + 1) / days.size),
                topLeft = Offset(index * segment + segment * 0.18f, size.height - barHeight),
                size = Size(segment * 0.64f, barHeight), cornerRadius = CornerRadius(4.dp.toPx()),
            )
        }
    }
    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
        Text(days.first().text("day"), fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        Text(days.last().text("day"), fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

@Composable
private fun AdminUsers(state: CaptureUiState, vm: CaptureViewModel, confirm: (AdminConfirmation) -> Unit) {
    var query by rememberSaveable { mutableStateOf("") }
    var role by rememberSaveable { mutableStateOf("") }
    var active by rememberSaveable { mutableStateOf("") }
    var form by remember { mutableStateOf<String?>(null) }
    var selectedUser by remember { mutableStateOf<JSONObject?>(null) }
    var showSessions by remember { mutableStateOf(false) }
    val users = state.adminData.objects("users").filter { user ->
        (query.isBlank() || listOf(user.text("username"), user.text("displayName")).any { it.contains(query.trim(), true) }) &&
            (role.isEmpty() || user.text("role") == role) && (active.isEmpty() || user.optBoolean("isActive") == (active == "true"))
    }
    LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(16.dp), verticalArrangement = Arrangement.spacedBy(0.dp)) {
        item { Box(Modifier.padding(bottom = 12.dp)) { AdminToolbar("用户账号", "${users.size} 个账号") { vm.loadAdmin("users") } } }
        item {
            OutlinedTextField(query, { query = it }, Modifier.fillMaxWidth().padding(bottom = 8.dp), label = { Text("搜索用户名或显示名") }, singleLine = true, shape = RoundedCornerShape(12.dp))
        }
        item {
            Row(Modifier.padding(bottom = 12.dp).horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                AdminFilter("角色", role, listOf("" to "全部角色", "admin" to "管理员", "user" to "普通用户")) { role = it }
                AdminFilter("状态", active, listOf("" to "全部状态", "true" to "已启用", "false" to "已停用")) { active = it }
                Button(enabled = !state.busy, onClick = { selectedUser = null; form = "create" }) { Text("创建用户") }
            }
        }
        if (users.isEmpty() && !state.adminLoading) item { AdminEmpty("没有匹配的用户", "调整搜索或筛选条件，也可以创建新账号。") }
        itemsIndexed(users, key = { _, user -> user.text("id") }) { index, user ->
            val id = user.text("id")
            val self = id == state.session?.id
            val enabled = user.optBoolean("isActive")
            var menu by remember(id) { mutableStateOf(false) }
            AdminListRow(index == 0, index == users.lastIndex) {
                Row(Modifier.fillMaxWidth().padding(start = 14.dp, end = 4.dp, top = 12.dp, bottom = 12.dp), verticalAlignment = Alignment.CenterVertically) {
                    Surface(shape = RoundedCornerShape(9.dp), color = MaterialTheme.colorScheme.primaryContainer) {
                        Box(Modifier.size(38.dp), contentAlignment = Alignment.Center) {
                            Text(user.text("displayName", user.text("username")).take(1), color = MaterialTheme.colorScheme.primary, fontWeight = FontWeight.Medium)
                        }
                    }
                    Column(Modifier.weight(1f).padding(horizontal = 10.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                        Text(user.text("displayName", user.text("username")), fontSize = 14.sp, fontWeight = FontWeight.Medium, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        Text("@${user.text("username")}${if (self) " · 当前账号" else ""}", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        Text("${if (user.text("role") == "admin") "管理员" else "普通用户"} · 每日 ${adminLimit(user, "dailyTaskLimitOverride")}", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                    Text(if (enabled) "已启用" else "已停用", fontSize = 11.sp, color = if (enabled) Color(0xFF00B42A) else MaterialTheme.colorScheme.error)
                    Box {
                        IconButton(onClick = { menu = true }, enabled = !state.busy, modifier = Modifier.size(48.dp).semantics { contentDescription = "用户操作" }) { ArcoIcon("more", size = 20.dp) }
                        DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                            Column(Modifier.padding(horizontal = 16.dp, vertical = 8.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                                Text("最近登录 ${adminDate(user.text("lastLoginAt"))}", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                Text("创建于 ${adminDate(user.text("createdAt"))}", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                            }
                            HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
                            DropdownMenuItem(text = { Text("查看任务") }, onClick = { menu = false; vm.loadAdmin("jobs", extra = JSONObject().put("userId", id)) })
                            DropdownMenuItem(text = { Text("每日额度") }, onClick = { menu = false; selectedUser = user; form = "limit" })
                            DropdownMenuItem(text = { Text("重置密码") }, onClick = { menu = false; selectedUser = user; form = "password" })
                            DropdownMenuItem(text = { Text("登录设备") }, onClick = { menu = false; selectedUser = user; showSessions = true; vm.loadSessions(id) })
                            DropdownMenuItem(enabled = !self, text = { Text(if (enabled) "停用账号" else "启用账号") }, onClick = {
                                menu = false
                                confirm(AdminConfirmation(if (enabled) "停用账号？" else "启用账号？", "${user.text("displayName")} ${if (enabled) "将无法继续登录，已有会话也会失效。" else "将恢复登录和创建任务的权限。"}", {
                                    vm.adminRequest("PATCH", "/api/v1/admin/users/${adminSegment(id)}/active", JSONObject().put("active", !enabled), if (enabled) "账号已停用" else "账号已启用")
                                }, enabled))
                            })
                            DropdownMenuItem(enabled = !self, text = { Text("删除用户", color = MaterialTheme.colorScheme.error) }, onClick = {
                                menu = false
                                confirm(AdminConfirmation("删除用户？", "账号 ${user.text("username")} 将被删除，其会话立即失效。历史任务会保留，此操作无法撤销。", {
                                    vm.adminRequest("DELETE", "/api/v1/admin/users/${adminSegment(id)}", success = "用户已删除")
                                }, true))
                            })
                        }
                    }
                }
            }
        }
    }
    form?.let { type ->
        AdminUserForm(type, selectedUser, state.busy, { form = null }) { body ->
            val id = adminSegment(selectedUser?.text("id").orEmpty())
            when (type) {
                "create" -> vm.adminRequest("POST", "/api/v1/admin/users", body, "用户已创建")
                "password" -> confirm(AdminConfirmation("重置用户密码？", "该用户现有登录会话会全部下线。", { vm.adminRequest("POST", "/api/v1/admin/users/$id/password", body, "密码已重置") }, true))
                "limit" -> vm.adminRequest("PUT", "/api/v1/admin/users/$id/daily-task-limit", body, "用户额度已更新")
            }
            form = null
        }
    }
    if (showSessions) {
        AlertDialog(
            onDismissRequest = { showSessions = false }, title = { Text("${selectedUser?.text("displayName").orEmpty()} 的会话") },
            text = {
                Column(Modifier.heightIn(max = 420.dp).verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    if (state.sessions.isEmpty()) Text("没有活跃会话，或正在加载。")
                    state.sessions.forEach { session ->
                        val device = session.optJSONObject("device") ?: JSONObject()
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Column(Modifier.weight(1f)) {
                                Text(device.text("name", device.text("platform", "未知设备")), fontWeight = FontWeight.Medium)
                                Text("${adminDate(session.text("lastUsedAt"))} 活跃${if (session.optBoolean("current")) " · 当前会话" else ""}", fontSize = 11.sp)
                            }
                            TextButton(enabled = !state.busy && !session.optBoolean("current"), onClick = {
                                confirm(AdminConfirmation("下线设备？", "该设备需要重新登录才能继续使用。", { vm.revokeSession(session.text("id"), admin = true) }, true))
                            }) { Text("下线") }
                        }
                    }
                }
            },
            confirmButton = { TextButton(onClick = { showSessions = false }) { Text("关闭") } },
            dismissButton = { TextButton(onClick = { vm.loadSessions(selectedUser?.text("id")) }) { Text("刷新") } },
        )
    }
}

@Composable
private fun AdminUserForm(type: String, user: JSONObject?, busy: Boolean, dismiss: () -> Unit, submit: (JSONObject) -> Unit) {
    var username by remember { mutableStateOf("") }
    var displayName by remember { mutableStateOf("") }
    var password by remember { mutableStateOf("") }
    var role by remember { mutableStateOf("user") }
    var inherit by remember { mutableStateOf(user == null || user.isNull("dailyTaskLimitOverride")) }
    var limit by remember { mutableStateOf(if (user == null || user.isNull("dailyTaskLimitOverride")) "0" else user.optInt("dailyTaskLimitOverride").toString()) }
    val valid = when (type) {
        "create" -> username.trim().isNotEmpty() && username.length <= 64 && displayName.trim().isNotEmpty() && displayName.length <= 100 && password.length in 6..12
        "password" -> password.length in 6..12
        else -> inherit || limit.toIntOrNull()?.let { it in 0..100000 } == true
    }
    AdminFormDialog(when (type) { "create" -> "创建用户"; "password" -> "重置密码"; else -> "每日任务额度" }, busy, valid, dismiss, {
        submit(when (type) {
            "create" -> JSONObject().put("username", username.trim()).put("displayName", displayName.trim()).put("password", password).put("role", role)
            "password" -> JSONObject().put("password", password)
            else -> JSONObject().put("dailyTaskLimit", if (inherit) JSONObject.NULL else limit.toInt())
        })
    }) {
        if (type == "create") {
            AdminInput("用户名", username, { username = it }, helper = "最多 64 个字符")
            AdminInput("显示名", displayName, { displayName = it }, helper = "最多 100 个字符")
            AdminFilter("角色", role, listOf("user" to "普通用户", "admin" to "管理员")) { role = it }
        }
        if (type == "create" || type == "password") AdminInput("密码", password, { password = it }, secret = true, helper = "6 至 12 个字符")
        if (type == "limit") {
            AdminSwitch("跟随全局默认额度", "关闭后可为该账号设置独立额度", inherit) { inherit = it }
            if (!inherit) AdminInput("每日次数", limit, { limit = it }, numeric = true, helper = "0 表示不限，最大 100000")
            Text("每日次数按北京时间零点重置。", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}

@Composable
private fun AdminJobs(state: CaptureUiState, vm: CaptureViewModel, confirm: (AdminConfirmation) -> Unit) {
    var query by rememberSaveable { mutableStateOf(state.adminQuery) }
    var status by rememberSaveable { mutableStateOf("") }
    var action by rememberSaveable { mutableStateOf("") }
    var userId by rememberSaveable { mutableStateOf("") }
    var pageSize by rememberSaveable { mutableStateOf(10) }
    var selected by remember { mutableStateOf(setOf<String>()) }
    var selecting by rememberSaveable { mutableStateOf(false) }
    val jobs = state.adminData.objects("jobs")
    LaunchedEffect(state.adminData) {
        state.adminData.optJSONObject("_filters")?.let { filters ->
            status = filters.text("status")
            action = filters.text("action")
            userId = filters.text("userId")
            pageSize = filters.optInt("limit", 10)
        }
        selected = selected.intersect(jobs.map { it.text("id") }.toSet())
    }
    val total = state.adminData.optInt("total")
    val locked = state.busy || state.adminLoading
    fun extras() = JSONObject().put("status", status).put("action", action).put("userId", userId).put("limit", pageSize)
    fun load(page: Int = 0) { selected = emptySet(); vm.loadAdmin("jobs", query.trim(), page, extras()) }
    LaunchedEffect(total, state.adminPage, state.adminLoading) {
        val lastPage = max(0, (total + pageSize - 1) / pageSize - 1)
        if (!state.adminLoading && state.adminPage > lastPage) load(lastPage)
    }
    fun batch(op: String) {
        val matching = jobs.filter { it.text("id") in selected && adminJobAllows(it.text("status"), op) }
        val label = when (op) { "cancel" -> "取消"; "retry" -> "重试"; else -> "删除" }
        if (matching.isEmpty()) return
        confirm(AdminConfirmation("批量${label}任务？", "将${label} ${matching.size} 个符合条件的任务。${if (matching.size < selected.size) "另有 ${selected.size - matching.size} 个任务因当前状态不符而跳过。" else ""}${if (op == "delete") "任务及其服务器文件将被永久删除。" else ""}", {
            selected = emptySet()
            vm.batchAdminRequests(matching.map { AdminOperation(if (op == "delete") "DELETE" else "POST", "/api/v1/admin/jobs/${adminSegment(it.text("id"))}${if (op == "delete") "" else "/$op"}") }, "批量${label}完成")
        }, op != "retry"))
    }
    LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(16.dp), verticalArrangement = Arrangement.spacedBy(0.dp)) {
        item { Box(Modifier.padding(bottom = 12.dp)) { AdminToolbar("全部任务", "$total 个任务") { load(state.adminPage) } } }
        item {
            OutlinedTextField(query, { query = it }, Modifier.fillMaxWidth().padding(bottom = 8.dp), label = { Text("搜索作品标题或作者") }, singleLine = true, shape = RoundedCornerShape(12.dp), trailingIcon = { TextButton(enabled = !locked, onClick = { load() }) { Text("搜索") } })
        }
        item {
            Row(Modifier.padding(bottom = 8.dp).horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                AdminFilter("状态", status, adminStatusOptions) { status = it; load() }
                AdminFilter("类型", action, adminActionOptions) { action = it; load() }
                val ownerOptions = listOf("" to "全部用户") + state.adminData.objects("_users").map { it.text("id") to it.text("displayName", it.text("username")) } +
                    state.adminData.objects("_deletedJobOwners").map { it.text("id") to "${it.text("displayName", it.text("username", "已删除用户"))}（已删除）" }
                AdminFilter("用户", userId, ownerOptions) { userId = it; load() }
                AdminFilter("每页", pageSize.toString(), listOf(10, 20, 50, 100).map { it.toString() to "${it}条" }) { pageSize = it.toInt(); load() }
            }
        }
        item {
            AdminSelectionBar(
                total = jobs.size, count = selected.size, selecting = selecting, enabled = !locked,
                onSelectingChange = { selecting = it; if (!it) selected = emptySet() },
                onSelectAll = { selected = if (selected.size == jobs.size) emptySet() else jobs.map { it.text("id") }.toSet() },
                onClear = { selected = emptySet() },
            ) {
                listOf("cancel" to "取消", "retry" to "重试", "delete" to "删除").forEach { (op, label) ->
                    val count = jobs.count { it.text("id") in selected && adminJobAllows(it.text("status"), op) }
                    AdminBatchAction(
                        label, count, icon = when (op) { "cancel" -> "stop"; "retry" -> "refresh"; else -> "trash" },
                        enabled = !locked && count > 0, dangerous = op == "delete",
                    ) { batch(op) }
                }
            }
        }
        if (jobs.isEmpty() && !state.adminLoading) item { AdminEmpty("没有匹配的任务", "试试调整搜索、用户或状态筛选。") }
        itemsIndexed(jobs, key = { _, job -> job.text("id") }) { _, job ->
            val id = job.text("id")
            val work = job.optJSONObject("work") ?: JSONObject()
            val jobStatus = job.text("status")
            var menu by remember(id) { mutableStateOf(false) }
            val chosen = id in selected
            Surface(
                modifier = Modifier.fillMaxWidth().padding(bottom = 8.dp),
                shape = RoundedCornerShape(14.dp),
                color = MaterialTheme.colorScheme.surface,
                border = BorderStroke(1.dp, if (chosen) MaterialTheme.colorScheme.primary.copy(alpha = .55f) else MaterialTheme.colorScheme.outlineVariant.copy(alpha = .55f)),
            ) {
                Row(Modifier.fillMaxWidth().padding(6.dp), verticalAlignment = Alignment.CenterVertically) {
                    if (selecting) AdminSelectionControl(chosen, !locked, if (chosen) "取消选择任务" else "选择任务") {
                        selected = if (chosen) selected - id else selected + id
                    }
                    Row(
                        Modifier.weight(1f).clip(RoundedCornerShape(10.dp))
                            .clickable(enabled = !locked || !selecting) {
                                if (selecting) selected = if (chosen) selected - id else selected + id
                                else vm.openJob(id, admin = true)
                            }.padding(6.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(10.dp),
                    ) {
                        val cover = work.text("coverPreviewUrl").ifBlank { work.text("coverUrl") }
                        if (cover.isNotBlank()) RemoteImage(mediaUrl(state.preferences.serverUrl, cover), Modifier.size(width = 48.dp, height = 60.dp).clip(RoundedCornerShape(8.dp)), description = "作品封面", maxDimension = 240)
                        else Box(Modifier.size(width = 48.dp, height = 60.dp).background(MaterialTheme.colorScheme.surfaceContainerHigh, RoundedCornerShape(8.dp)), contentAlignment = Alignment.Center) { ArcoIcon(if (work.text("type") == "note") "image" else "video", size = 20.dp) }
                        Column(Modifier.weight(1f).heightIn(min = 64.dp), verticalArrangement = Arrangement.spacedBy(4.dp, Alignment.CenterVertically)) {
                            Text(work.text("title", "未命名作品"), fontSize = 14.sp, fontWeight = FontWeight.Medium, maxLines = 2, overflow = TextOverflow.Ellipsis)
                            Text("${job.text("ownerDisplayName", job.text("ownerUsername", "用户"))} · ${adminActionLabel(job.text("action"))}", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
                            Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                                Text(adminStatusLabel(jobStatus), fontSize = 11.sp, color = adminStatusColor(jobStatus))
                                Text(if (adminJobAllows(jobStatus, "cancel")) "${job.optInt("progress")}%" else adminDate(job.text("createdAt")), fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                            }
                        }
                    }
                    Box {
                        IconButton(onClick = { menu = true }, enabled = !locked, modifier = Modifier.size(48.dp).semantics { contentDescription = "任务操作" }) { ArcoIcon("more", size = 20.dp) }
                        DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                            DropdownMenuItem(text = { Text("任务详情") }, onClick = { menu = false; vm.openJob(id, admin = true) })
                            listOf("cancel" to "取消", "retry" to "重试", "delete" to "删除").filter { adminJobAllows(jobStatus, it.first) }.forEach { (op, label) ->
                                DropdownMenuItem(text = { Text(label, color = if (op == "delete") MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurface) }, onClick = {
                                    menu = false
                                    confirm(AdminConfirmation("${label}任务？", if (op == "delete") "任务记录及其服务器文件将被永久删除。" else "确认${label}这个任务？", { vm.jobAction(id, op, admin = true) }, op != "retry"))
                                })
                            }
                        }
                    }
                }
            }
        }
        item { AdminPager(state.adminPage, total, pageSize, !locked) { load(it) } }
    }
}

@Composable
private fun AdminSelectionBar(
    total: Int,
    count: Int,
    selecting: Boolean,
    enabled: Boolean,
    onSelectingChange: (Boolean) -> Unit,
    onSelectAll: () -> Unit,
    onClear: () -> Unit,
    selectionHint: String = "只处理本页选中的任务",
    idleHint: String = "取消、重试或删除多个任务",
    actions: @Composable RowScope.() -> Unit,
) {
    Surface(
        modifier = Modifier.fillMaxWidth().padding(bottom = 12.dp),
        shape = RoundedCornerShape(14.dp), color = MaterialTheme.colorScheme.surface,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant.copy(alpha = .55f)),
    ) {
        Column(Modifier.padding(horizontal = 12.dp, vertical = 8.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                if (selecting) {
                    IconButton(onClick = onSelectAll, enabled = enabled && total > 0, modifier = Modifier.size(40.dp).semantics { contentDescription = if (count == total && total > 0) "取消全选本页" else "全选本页" }) {
                        ArcoIcon(if (count == total && total > 0) "check_circle" else if (count > 0) "minus_circle" else "circle", size = 22.dp, tint = if (count > 0) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                } else ArcoIcon("check_circle", size = 21.dp, tint = MaterialTheme.colorScheme.onSurfaceVariant)
                Column(Modifier.weight(1f).padding(start = 8.dp), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                    Text(if (selecting) "已选 $count 项" else "批量处理", style = MaterialTheme.typography.bodyMedium, fontWeight = FontWeight.Medium)
                    Text(if (selecting) selectionHint else idleHint, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                TextButton(onClick = { onSelectingChange(!selecting) }, enabled = enabled && (selecting || total > 0), contentPadding = PaddingValues(horizontal = 10.dp)) {
                    Text(if (selecting) "完成" else "选择")
                }
            }
            if (selecting) {
                Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                    TextButton(onClick = onSelectAll, enabled = enabled && total > 0, contentPadding = PaddingValues(horizontal = 8.dp)) {
                        Text(if (count == total && total > 0) "取消全选" else "全选本页")
                    }
                    Spacer(Modifier.weight(1f))
                    if (count > 0) TextButton(onClick = onClear, enabled = enabled, contentPadding = PaddingValues(horizontal = 8.dp)) { Text("清空选择") }
                }
                if (count > 0) Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp), content = actions)
            }
        }
    }
}

@Composable
private fun RowScope.AdminBatchAction(label: String, count: Int, icon: String, enabled: Boolean, dangerous: Boolean = false, onClick: () -> Unit) {
    val color = if (dangerous) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurface
    OutlinedButton(
        onClick = onClick, enabled = enabled, modifier = Modifier.weight(1f).height(44.dp),
        shape = RoundedCornerShape(10.dp), contentPadding = PaddingValues(horizontal = 8.dp, vertical = 8.dp),
        border = BorderStroke(1.dp, if (dangerous && enabled) color.copy(alpha = .35f) else MaterialTheme.colorScheme.outlineVariant),
        colors = ButtonDefaults.outlinedButtonColors(contentColor = color),
    ) {
        ArcoIcon(icon, size = 16.dp, tint = androidx.compose.material3.LocalContentColor.current)
        Spacer(Modifier.width(5.dp))
        Text("$label $count", style = MaterialTheme.typography.labelMedium, maxLines = 1)
    }
}

@Composable
private fun AdminSelectionControl(chosen: Boolean, enabled: Boolean, label: String, onClick: () -> Unit) {
    IconButton(
        onClick = onClick, enabled = enabled,
        modifier = Modifier.size(40.dp).semantics { contentDescription = label; selected = chosen },
    ) {
        ArcoIcon(if (chosen) "check_circle" else "circle", size = 22.dp, tint = when {
            !enabled -> MaterialTheme.colorScheme.outline
            chosen -> MaterialTheme.colorScheme.primary
            else -> MaterialTheme.colorScheme.onSurfaceVariant
        })
    }
}

@Composable
private fun AdminCdks(state: CaptureUiState, vm: CaptureViewModel, confirm: (AdminConfirmation) -> Unit) {
    var count by rememberSaveable { mutableStateOf("1") }
    var quota by rememberSaveable { mutableStateOf("10") }
    var selected by remember { mutableStateOf(setOf<String>()) }
    var selecting by rememberSaveable { mutableStateOf(false) }
    var quotaTarget by remember { mutableStateOf<JSONObject?>(null) }
    var pageSize by rememberSaveable { mutableStateOf(20) }
    val items = state.adminData.objects("items")
    val total = state.adminData.optInt("total")
    val locked = state.busy || state.adminLoading
    fun load(page: Int = state.adminPage) = vm.loadAdmin("cdks", page = page, extra = JSONObject().put("limit", pageSize))
    LaunchedEffect(state.adminData) {
        pageSize = state.adminData.optJSONObject("_filters")?.optInt("limit", 20) ?: pageSize
        selected = selected.intersect(items.map { it.text("id") }.toSet())
    }
    LaunchedEffect(total, state.adminPage, state.adminLoading) {
        val lastPage = max(0, (total + pageSize - 1) / pageSize - 1)
        if (!state.adminLoading && state.adminPage > lastPage) load(lastPage)
    }
    fun mutate(op: String, targets: List<JSONObject>) {
        val allowed = targets.filter { if (op == "delete") it.text("status") == "revoked" else it.text("status") != "revoked" }
        if (allowed.isEmpty()) return
        val title = when (op) { "reset" -> "重置已用次数"; "rotate" -> "轮换密钥"; "revoke" -> "吊销密钥"; else -> "删除密钥记录" }
        val details = when (op) {
            "reset" -> "累计已用次数清零，总额度保留。"
            "rotate" -> "旧密钥及其会话立即失效，新密钥仅在本次操作后显示。"
            "revoke" -> "密钥及其登录会话将立即失效。"
            else -> "已吊销的密钥记录将永久删除，相关历史任务保留。"
        }
        confirm(AdminConfirmation(title, "将处理 ${allowed.size} 个密钥。$details${if (allowed.size < targets.size) "其余状态不符的密钥将跳过。" else ""}", {
            selected = emptySet()
            vm.batchAdminRequests(allowed.map {
                val suffix = when (op) { "revoke" -> ""; "delete" -> "/record"; else -> "/$op" }
                AdminOperation(if (op == "revoke" || op == "delete") "DELETE" else "POST", "/api/v1/admin/cdks/${adminSegment(it.text("id"))}$suffix")
            }, "$title 完成")
        }, op != "reset"))
    }
    LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(16.dp), verticalArrangement = Arrangement.spacedBy(0.dp)) {
        item { Box(Modifier.padding(bottom = 12.dp)) { AdminToolbar("CDK 密钥", "$total 个密钥") { load() } } }
        item { Box(Modifier.padding(bottom = 8.dp)) { AdminFilter("每页", pageSize.toString(), listOf(10, 20, 25, 50, 100).map { it.toString() to "${it}条" }) { pageSize = it.toInt(); selected = emptySet(); load(0) } } }
        item {
            AdminCard(Modifier.padding(bottom = 12.dp)) {
                Text("生成访问密钥", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
                Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    AdminInput("生成数量", count, { count = it }, Modifier.weight(1f), numeric = true, helper = "1 至 100 个")
                    AdminInput("每个任务总次数", quota, { quota = it }, Modifier.weight(1f), numeric = true, helper = "1 至 100000 次")
                }
                Text("额度按累计创建任务次数计算。完整密钥仅在生成或轮换后显示。", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                Button(enabled = !locked && count.toIntOrNull()?.let { it in 1..100 } == true && quota.toIntOrNull()?.let { it in 1..100000 } == true, onClick = {
                    vm.adminRequest("POST", "/api/v1/admin/cdks", JSONObject().put("count", count.toInt()).put("taskLimit", quota.toInt()), "密钥已生成，请保存")
                }) { Text("生成密钥") }
            }
        }
        item {
            AdminSelectionBar(
                total = items.size, count = selected.size, selecting = selecting, enabled = !locked,
                onSelectingChange = { selecting = it; if (!it) selected = emptySet() },
                onSelectAll = { selected = if (selected.size == items.size) emptySet() else items.map { it.text("id") }.toSet() },
                onClear = { selected = emptySet() },
                selectionHint = "只处理本页选中的密钥",
                idleHint = "重置、轮换、吊销或删除多个密钥",
            ) {
                Column(Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    listOf("reset" to "重置次数", "rotate" to "轮换", "revoke" to "吊销", "delete" to "删除记录").chunked(2).forEach { operations ->
                        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            operations.forEach { (op, label) ->
                                val eligibleCount = items.count { it.text("id") in selected && (if (op == "delete") it.text("status") == "revoked" else it.text("status") != "revoked") }
                                AdminBatchAction(
                                    label, eligibleCount, icon = when (op) { "reset" -> "refresh"; "rotate" -> "key"; "revoke" -> "stop"; else -> "trash" },
                                    enabled = !locked && eligibleCount > 0, dangerous = op == "revoke" || op == "delete",
                                ) { mutate(op, items.filter { it.text("id") in selected }) }
                            }
                        }
                    }
                }
            }
        }
        if (items.isEmpty() && !state.adminLoading) item { AdminEmpty("暂无密钥", "在上方生成一批 CDK，使用额度独立于每日额度。") }
        itemsIndexed(items, key = { _, item -> item.text("id") }) { _, item ->
            val id = item.text("id")
            val revoked = item.text("status") == "revoked"
            var menu by remember(id) { mutableStateOf(false) }
            val chosen = id in selected
            Surface(
                modifier = Modifier.fillMaxWidth().padding(bottom = 8.dp),
                shape = RoundedCornerShape(14.dp),
                color = MaterialTheme.colorScheme.surface,
                border = BorderStroke(1.dp, if (chosen) MaterialTheme.colorScheme.primary.copy(alpha = .55f) else MaterialTheme.colorScheme.outlineVariant.copy(alpha = .55f)),
            ) {
                Row(Modifier.fillMaxWidth().padding(6.dp), verticalAlignment = Alignment.CenterVertically) {
                    if (selecting) AdminSelectionControl(chosen, !locked, if (chosen) "取消选择密钥" else "选择密钥") {
                        selected = if (chosen) selected - id else selected + id
                    }
                    Row(
                        Modifier.weight(1f).clip(RoundedCornerShape(10.dp))
                            .then(if (selecting) Modifier.clickable(enabled = !locked) { selected = if (chosen) selected - id else selected + id } else Modifier)
                            .padding(6.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(10.dp),
                    ) {
                        Column(Modifier.weight(1f).heightIn(min = 64.dp), verticalArrangement = Arrangement.spacedBy(4.dp, Alignment.CenterVertically)) {
                            Text("${item.text("prefix")}…", fontFamily = FontFamily.Monospace, fontWeight = FontWeight.Medium, fontSize = 14.sp)
                            Text("已用 ${item.optInt("taskCount")} / ${item.optInt("taskLimit")} 次", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                            Text(adminDate(item.text("createdAt")), fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                        Text(if (!revoked && item.optInt("taskCount") >= item.optInt("taskLimit")) "次数用尽" else when (item.text("status")) { "unused" -> "待使用"; "redeemed" -> "已登录"; else -> "已吊销" }, fontSize = 11.sp, color = if (revoked) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                    Box {
                        IconButton(onClick = { menu = true }, enabled = !locked, modifier = Modifier.size(48.dp).semantics { contentDescription = "密钥操作" }) { ArcoIcon("more", size = 20.dp) }
                        DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                            if (!revoked) {
                                DropdownMenuItem(text = { Text("调整额度") }, onClick = { menu = false; quotaTarget = item })
                                DropdownMenuItem(text = { Text("重置次数") }, onClick = { menu = false; mutate("reset", listOf(item)) })
                                DropdownMenuItem(text = { Text("轮换密钥") }, onClick = { menu = false; mutate("rotate", listOf(item)) })
                                DropdownMenuItem(text = { Text("吊销密钥", color = MaterialTheme.colorScheme.error) }, onClick = { menu = false; mutate("revoke", listOf(item)) })
                            } else DropdownMenuItem(text = { Text("删除记录", color = MaterialTheme.colorScheme.error) }, onClick = { menu = false; mutate("delete", listOf(item)) })
                        }
                    }
                }
            }
        }
        item { AdminPager(state.adminPage, total, pageSize, !locked) { selected = emptySet(); load(it) } }
    }
    quotaTarget?.let { target ->
        var newLimit by remember(target) { mutableStateOf(target.optInt("taskLimit").toString()) }
        AdminFormDialog("调整任务总次数", state.busy, newLimit.toIntOrNull()?.let { it in 1..100000 } == true, { quotaTarget = null }, {
            vm.adminRequest("PATCH", "/api/v1/admin/cdks/${adminSegment(target.text("id"))}", JSONObject().put("taskLimit", newLimit.toInt()), "CDK 额度已更新")
            quotaTarget = null
        }) { AdminInput("任务总次数", newLimit, { newLimit = it }, numeric = true, helper = "1 至 100000，不会清零已用次数") }
    }
}

@Composable
private fun AdminProviders(state: CaptureUiState, vm: CaptureViewModel, confirm: (AdminConfirmation) -> Unit) {
    val providers = state.adminData.optJSONObject("providers")
    val providerItems = providers?.objects("items").orEmpty()
    var enabled by remember { mutableStateOf(providerItems.filter { it.optBoolean("enabled") }.map { it.text("id") }.toSet()) }
    var default by remember { mutableStateOf(providers?.text("defaultProvider").orEmpty()) }
    var allowSelection by remember { mutableStateOf(providers?.optBoolean("allowUserSelection") ?: false) }
    var model by remember { mutableStateOf(providerItems.find { it.text("id") == "siliconflow" }?.text("model").orEmpty()) }
    var keys by remember { mutableStateOf(mapOf<String, String>()) }
    var previousProviders by remember { mutableStateOf(providers) }
    var pendingKeysToClear by remember { mutableStateOf<Map<String, String>?>(null) }
    var resultBeforeKeyChange by remember { mutableStateOf<JSONObject?>(null) }
    LaunchedEffect(providers) {
        val incoming = providers ?: return@LaunchedEffect
        val previous = previousProviders
        val previousItems = previous?.objects("items").orEmpty()
        if (previous == null || enabled == previousItems.filter { it.optBoolean("enabled") }.map { it.text("id") }.toSet()) {
            enabled = providerItems.filter { it.optBoolean("enabled") }.map { it.text("id") }.toSet()
        }
        if (previous == null || default == previous.text("defaultProvider")) default = incoming.text("defaultProvider")
        if (previous == null || allowSelection == previous.optBoolean("allowUserSelection")) allowSelection = incoming.optBoolean("allowUserSelection")
        if (previous == null || model == previousItems.find { it.text("id") == "siliconflow" }?.text("model").orEmpty()) {
            model = providerItems.find { it.text("id") == "siliconflow" }?.text("model").orEmpty()
        }
        previousProviders = incoming
    }
    LaunchedEffect(state.adminResult, state.busy) {
        val submitted = pendingKeysToClear
        if (!state.busy && submitted != null) {
            if (state.adminResult !== resultBeforeKeyChange && state.adminResult?.has("providers") == true) {
                keys = keys.filter { (id, value) -> submitted[id] != value }
            }
            pendingKeysToClear = null
        }
    }
    val keyFields = mapOf("siliconflow" to "siliconFlowApiKey", "aliyun" to "aliyunApiKey", "deepseek" to "deepseekApiKey")
    val locked = state.busy || state.adminLoading || providers == null
    LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(20.dp), verticalArrangement = Arrangement.spacedBy(14.dp)) {
        item {
            AdminCard {
                Text("服务分配", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
                val options = listOf("" to "自动选择可用服务") + providerItems.filter { it.text("id") in enabled }.map { it.text("id") to adminProviderLabel(it.text("id")) }
                AdminFilter("默认服务", default, options) { default = it }
                AdminSwitch("允许用户选择服务", "", allowSelection, !locked) { allowSelection = it }
                val effective = providers?.text("effectiveDefaultProvider").orEmpty()
                Text("当前默认：${if (effective.isEmpty()) "暂无可用服务" else adminProviderLabel(effective)}", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
        items(providerItems, key = { it.text("id") }) { provider ->
            val id = provider.text("id")
            AdminCard {
                Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                    Text(adminProviderLabel(id), Modifier.weight(1f), style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
                    AdminBadge(if (provider.optBoolean("available")) "可用" else if (provider.optBoolean("configured")) "待就绪" else "未配置", if (provider.optBoolean("available")) Color(0xFF00B42A) else MaterialTheme.colorScheme.onSurfaceVariant)
                }
                AdminSwitch("启用服务", "", id in enabled, !locked) { checked ->
                    enabled = if (checked) enabled + id else enabled - id
                    if (!checked && default == id) default = ""
                }
                if (id != "siliconflow") Text("模型：${provider.text("model", "由服务端决定")}", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                if (id == "siliconflow") AdminInput("转写模型 ID", model, { model = it }, helper = "例如 FunAudioLLM/SenseVoiceSmall")
                AdminInput(if (provider.optBoolean("configured")) "输入新 API Key 以替换" else "API Key", keys[id].orEmpty(), { keys = keys + (id to it) }, secret = true, helper = "留空保留已有密钥；保存后输入会清空")
                if (id == "aliyun") Text("阿里云需要服务端配置可公开访问的 HTTPS 地址。", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedButton(enabled = !locked && provider.optBoolean("available"), onClick = {
                        confirm(AdminConfirmation("测试服务连接？", "将使用合成音频发起一次真实测试，可能计入服务商用量。", { vm.adminRequest("POST", "/api/v1/admin/settings/providers/$id/test", success = "连接测试完成") }))
                    }) { Text("测试连接") }
                    TextButton(enabled = !locked && provider.optBoolean("configured"), onClick = {
                        confirm(AdminConfirmation("清除 API Key？", "${adminProviderLabel(id)} 将停止接收新任务，直至重新配置。", {
                            pendingKeysToClear = keys.filterKeys { it == id }
                            resultBeforeKeyChange = state.adminResult
                            vm.adminRequest("DELETE", "/api/v1/admin/settings/providers/$id/key", success = "密钥已清除")
                        }, true))
                    }) { Text("清除密钥", color = MaterialTheme.colorScheme.error) }
                }
            }
        }
        item {
            Button(
                modifier = Modifier.fillMaxWidth(), enabled = !locked && model.isNotBlank() && model.length <= 256 && keys.values.all { it.length <= 4096 },
                onClick = {
                    val body = JSONObject().put("defaultProvider", default).put("enabledProviders", JSONArray(enabled.toList())).put("allowUserSelection", allowSelection)
                    if (model.trim() != providerItems.find { it.text("id") == "siliconflow" }?.text("model")) body.put("asrModel", model.trim())
                    keys.forEach { (id, value) -> if (value.isNotBlank()) body.put(keyFields.getValue(id), value.trim()) }
                    pendingKeysToClear = keys
                    resultBeforeKeyChange = state.adminResult
                    vm.adminRequest("PUT", "/api/v1/admin/settings/providers", body, "转写服务设置已保存")
                },
            ) { Text("保存设置") }
        }
    }
}

@Composable
private fun AdminUsage(state: CaptureUiState, vm: CaptureViewModel) {
    val settings = state.adminData.optJSONObject("settings")
    var limit by remember(settings) { mutableStateOf((settings?.optInt("dailyTaskLimit") ?: 0).toString()) }
    var allowInfo by remember(settings) { mutableStateOf(settings?.optBoolean("allowInfo") ?: false) }
    var allowDownload by remember(settings) { mutableStateOf(settings?.optBoolean("allowDownload") ?: false) }
    var allowTranscribe by remember(settings) { mutableStateOf(settings?.optBoolean("allowTranscribe") ?: false) }
    val locked = state.busy || state.adminLoading || settings == null
    LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(20.dp), verticalArrangement = Arrangement.spacedBy(14.dp)) {
        item { AppUsageSettings(state, vm) }
        item {
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text("服务端配置", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
                Text("以下设置对所有客户端生效。", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
        item {
            AdminCard {
                Text("每日任务额度", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
                AdminInput("默认每日次数", limit, { limit = it }, numeric = true, helper = "0 表示不限，范围 0 至 100000")
                Text("北京时间零点重置；用户单独额度优先。", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
        item {
            AdminCard {
                Text("新任务权限", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
                AdminSwitch("允许解析", "获取作品信息与发布文案", allowInfo, !locked) { allowInfo = it }
                HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
                AdminSwitch("允许下载", "保存作品视频、图文与音乐", allowDownload, !locked) { allowDownload = it }
                HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
                AdminSwitch("允许转写", "视频口播转为文字", allowTranscribe, !locked) { allowTranscribe = it }
            }
        }
        item {
            Button(enabled = !locked && limit.toIntOrNull()?.let { it in 0..100000 } == true, modifier = Modifier.fillMaxWidth(), onClick = {
                vm.adminRequest("PUT", "/api/v1/admin/settings/usage", JSONObject().put("dailyTaskLimit", limit.toInt()).put("allowInfo", allowInfo).put("allowDownload", allowDownload).put("allowTranscribe", allowTranscribe), "使用设置已保存")
            }) { Text("保存使用设置") }
        }
    }
}

@Composable
private fun AdminApiCatalog(state: CaptureUiState, vm: CaptureViewModel) {
    val motion = LocalMotionEnabled.current
    var query by rememberSaveable { mutableStateOf("") }
    var expanded by remember { mutableStateOf(setOf<String>()) }
    val context = LocalContext.current
    val terms = query.lowercase().split(Regex("\\s+")).filter { it.isNotEmpty() }
    val operations = state.adminData.objects("operations").filter { operation ->
        val content = listOf("method", "path", "group", "summary", "details", "auth").joinToString(" ") { operation.text(it) }.lowercase()
        terms.all { it in content }
    }
    LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(20.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        item { Text("${operations.size} 个接口", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant) }
        item { OutlinedTextField(query, { query = it }, Modifier.fillMaxWidth(), label = { Text("搜索路径、方法或功能") }, singleLine = true, shape = RoundedCornerShape(12.dp)) }
        if (operations.isEmpty() && !state.adminLoading) item { AdminEmpty("没有匹配的接口", "调整关键词后重试。") }
        operations.groupBy { it.text("group") }.forEach { (group, members) ->
            item(key = "group:$group") { Text(group, Modifier.padding(top = 12.dp), fontWeight = FontWeight.SemiBold, color = MaterialTheme.colorScheme.primary) }
            items(members, key = { "${it.text("method")}:${it.text("path")}" }) { operation ->
                val identity = "${operation.text("method")}:${operation.text("path")}"
                AdminCard {
                    Column(Modifier.fillMaxWidth().clip(RoundedCornerShape(8.dp)).clickable { expanded = if (identity in expanded) expanded - identity else expanded + identity }.padding(8.dp)) {
                        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            AdminBadge(operation.text("method"), adminMethodColor(operation.text("method")))
                            Text(operation.text("path"), fontFamily = FontFamily.Monospace, fontSize = 11.sp, modifier = Modifier.weight(1f))
                            Text(if (identity in expanded) "收起" else "展开", fontSize = 11.sp, color = MaterialTheme.colorScheme.primary)
                        }
                        Text(operation.text("summary"), Modifier.padding(top = 8.dp), fontWeight = FontWeight.Medium)
                    }
                    AnimatedVisibility(
                        visible = identity in expanded,
                        enter = if (motion) fadeIn(tween(140)) else EnterTransition.None,
                        exit = if (motion) fadeOut(tween(100)) else ExitTransition.None,
                    ) {
                        Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                            Text("鉴权：${when (operation.text("auth")) { "public" -> "无需登录"; "bearer" -> "Bearer 访问令牌"; "admin" -> "管理员 Bearer 令牌"; "refresh" -> "刷新令牌或会话 Cookie"; "signed" -> "短期签名"; else -> "以服务端说明为准" }}", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                            if (operation.text("details").isNotBlank()) Text(operation.text("details"), fontSize = 12.sp)
                            TextButton(onClick = { adminCopy(context, state.preferences.serverUrl.trimEnd('/') + operation.text("path")) }) { Text("复制接口地址") }
                            val example = adminCurl(operation, state.preferences.serverUrl)
                            if (example.isNotEmpty()) {
                                Text("PowerShell 示例", fontWeight = FontWeight.Medium, fontSize = 12.sp)
                                Text("令牌、任务 ID 和分享链接从环境变量读取。", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                SelectionContainer { Text(example, Modifier.fillMaxWidth().background(MaterialTheme.colorScheme.surfaceVariant, RoundedCornerShape(8.dp)).padding(12.dp), fontFamily = FontFamily.Monospace, fontSize = 11.sp) }
                                TextButton(onClick = { adminCopy(context, example) }) { Text("复制命令") }
                            }
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun AdminOperationResult(state: CaptureUiState, vm: CaptureViewModel) {
    val result = state.adminResult ?: return
    val context = LocalContext.current
    val codes = (result.objects("items") + listOfNotNull(result.optJSONObject("item")))
        .map { it.text("code") }.filter { it.isNotEmpty() }.distinct()
    if (codes.isNotEmpty()) {
        AlertDialog(
            onDismissRequest = vm::clearAdminResult,
            title = { Text("保存完整 CDK 密钥") },
            text = {
                Column(Modifier.heightIn(max = 420.dp).verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    Text("完整密钥仅在本次显示，关闭前请复制保存。", color = MaterialTheme.colorScheme.onSurfaceVariant)
                    SelectionContainer { Text(codes.joinToString("\n"), fontFamily = FontFamily.Monospace, fontSize = 12.sp) }
                    if (result.optInt("failed") > 0) Text("另有 ${result.optInt("failed")} 项操作失败，请检查列表后重试。", color = MaterialTheme.colorScheme.error)
                }
            },
            confirmButton = { TextButton(onClick = { adminCopy(context, codes.joinToString("\n"), sensitive = true) }) { Text("复制全部") } },
            dismissButton = { TextButton(onClick = vm::clearAdminResult) { Text("已保存，关闭") } },
        )
    } else if (result.has("durationMs")) {
        AlertDialog(onDismissRequest = vm::clearAdminResult, title = { Text("连接正常") }, text = { Text("测试耗时 ${result.optLong("durationMs")} ms") }, confirmButton = { TextButton(onClick = vm::clearAdminResult) { Text("完成") } })
    } else if (result.has("results")) {
        AlertDialog(
            onDismissRequest = vm::clearAdminResult, title = { Text("批量操作结果") },
            text = {
                Column(Modifier.heightIn(max = 400.dp).verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text("成功 ${result.optInt("succeeded")} 项，失败 ${result.optInt("failed")} 项。")
                    result.objects("results").filter { !it.optBoolean("success") }.forEach { item -> Text(item.text("error", "操作失败"), color = MaterialTheme.colorScheme.error, fontSize = 12.sp) }
                }
            }, confirmButton = { TextButton(onClick = vm::clearAdminResult) { Text("完成") } },
        )
    }
}

@Composable
private fun AdminListRow(first: Boolean, last: Boolean, selected: Boolean = false, content: @Composable () -> Unit) {
    val corners = RoundedCornerShape(
        topStart = if (first) 12.dp else 0.dp, topEnd = if (first) 12.dp else 0.dp,
        bottomStart = if (last) 12.dp else 0.dp, bottomEnd = if (last) 12.dp else 0.dp,
    )
    Surface(Modifier.fillMaxWidth(), shape = corners, color = MaterialTheme.colorScheme.surface, border = if (selected) BorderStroke(1.dp, MaterialTheme.colorScheme.primary.copy(alpha = .45f)) else null) {
        Column {
            content()
            if (!last) HorizontalDivider(Modifier.padding(start = 16.dp), color = MaterialTheme.colorScheme.outlineVariant)
        }
    }
}

@Composable
private fun AdminCard(modifier: Modifier = Modifier, content: @Composable ColumnScope.() -> Unit) {
    Surface(modifier.fillMaxWidth(), shape = RoundedCornerShape(12.dp), color = MaterialTheme.colorScheme.surface, border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant.copy(alpha = 0.55f))) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(10.dp), content = content)
    }
}

@Composable
private fun AdminToolbar(title: String, subtitle: String, showTitle: Boolean = true, refresh: () -> Unit) {
    Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        Column(Modifier.weight(1f)) {
            if (showTitle) Text(title, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
            if (subtitle.isNotBlank()) Text(subtitle, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 2, overflow = TextOverflow.Ellipsis)
        }
        TextButton(onClick = refresh) { Text("刷新") }
    }
}

@Composable
private fun AdminFilter(label: String, selected: String, options: List<Pair<String, String>>, change: (String) -> Unit) {
    var open by remember { mutableStateOf(false) }
    Box {
        ChoicePill("$label：${options.find { it.first == selected }?.second ?: selected}", selected.isNotBlank(), icon = "chevron") { open = true }
        DropdownMenu(expanded = open, onDismissRequest = { open = false }) {
            options.forEach { (id, title) -> DropdownMenuItem(text = { Text(title, color = if (id == selected) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurface) }, trailingIcon = if (id == selected) ({ ArcoIcon("check", size = 18.dp, tint = MaterialTheme.colorScheme.primary) }) else null, onClick = { open = false; change(id) }) }
        }
    }
}

@Composable
private fun AdminBadge(text: String, color: Color) {
    Surface(color = color.copy(alpha = 0.1f), shape = RoundedCornerShape(6.dp)) {
        Text(text, Modifier.padding(horizontal = 8.dp, vertical = 4.dp), color = color, fontSize = 11.sp, fontWeight = FontWeight.Medium)
    }
}

@Composable
private fun AdminSwitch(title: String, subtitle: String, checked: Boolean, enabled: Boolean = true, change: (Boolean) -> Unit) {
    Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
        Column(Modifier.weight(1f).padding(end = 12.dp)) {
            Text(title, fontWeight = FontWeight.Medium, fontSize = 14.sp)
            if (subtitle.isNotBlank()) Text(subtitle, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        Switch(checked = checked, onCheckedChange = change, enabled = enabled)
    }
}

@Composable
private fun AdminInput(label: String, value: String, change: (String) -> Unit, modifier: Modifier = Modifier, numeric: Boolean = false, secret: Boolean = false, helper: String = "") {
    OutlinedTextField(
        value = value, onValueChange = change, modifier = modifier.fillMaxWidth(), label = { Text(label) }, singleLine = true,
        shape = RoundedCornerShape(12.dp),
        keyboardOptions = KeyboardOptions(keyboardType = if (numeric) KeyboardType.Number else if (secret) KeyboardType.Password else KeyboardType.Text),
        visualTransformation = if (secret) PasswordVisualTransformation() else androidx.compose.ui.text.input.VisualTransformation.None,
        supportingText = if (helper.isNotEmpty()) ({ Text(helper, fontSize = 11.sp) }) else null,
    )
}

@Composable
private fun AdminFormDialog(title: String, busy: Boolean, valid: Boolean, dismiss: () -> Unit, submit: () -> Unit, content: @Composable ColumnScope.() -> Unit) {
    AlertDialog(
        onDismissRequest = { if (!busy) dismiss() }, title = { Text(title) },
        text = { Column(Modifier.fillMaxWidth().heightIn(max = 480.dp).verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(12.dp), content = content) },
        confirmButton = { TextButton(enabled = valid && !busy, onClick = submit) { if (busy) CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp) else Text("确认") } },
        dismissButton = { TextButton(enabled = !busy, onClick = dismiss) { Text("取消") } },
    )
}

@Composable
private fun AdminPager(page: Int, total: Int, size: Int, enabled: Boolean, change: (Int) -> Unit) {
    val pages = max(1, (total + size - 1) / size)
    Row(Modifier.fillMaxWidth().padding(vertical = 8.dp), horizontalArrangement = Arrangement.SpaceBetween, verticalAlignment = Alignment.CenterVertically) {
        OutlinedButton(enabled = enabled && page > 0, onClick = { change(page - 1) }) { Text("上一页") }
        Text("${page + 1} / $pages", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        OutlinedButton(enabled = enabled && page + 1 < pages, onClick = { change(page + 1) }) { Text("下一页") }
    }
}

@Composable
private fun AdminEmpty(title: String, message: String) {
    AdminCard {
        Text(title, Modifier.padding(top = 16.dp), fontWeight = FontWeight.SemiBold)
        Text(message, Modifier.padding(bottom = 16.dp), color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 12.sp)
    }
}

private val adminStatusOptions = listOf("" to "全部状态", "processing" to "处理中", "queued" to "排队中", "completed" to "已完成", "failed" to "失败", "cancelled" to "已取消")
private val adminActionOptions = listOf("" to "全部类型", "info" to "仅解析", "download" to "下载", "transcribe" to "历史转写", "full" to "文案转写")

private fun adminStatusLabel(status: String): String = when (status) {
    "queued" -> "排队中"; "resolving" -> "解析中"; "downloading" -> "下载中"; "extracting" -> "提取音频"
    "transcribing" -> "转写中"; "postprocessing" -> "生成结果"; "retry_wait" -> "等待重试"; "completed" -> "已完成"
    "failed" -> "失败"; "cancelled" -> "已取消"; else -> status
}

private fun adminStatusColor(status: String): Color = when (status) {
    "completed" -> Color(0xFF00B42A); "failed" -> Color(0xFFF53F3F); "cancelled" -> Color(0xFF86909C)
    "queued", "retry_wait" -> Color(0xFFFF7D00); else -> Color(0xFF165DFF)
}

private fun adminMethodColor(method: String): Color = when (method) {
    "GET", "HEAD" -> Color(0xFF165DFF); "POST" -> Color(0xFF00B42A); "DELETE" -> Color(0xFFF53F3F); else -> Color(0xFFFF7D00)
}

private fun adminActionLabel(action: String): String = adminActionOptions.find { it.first == action }?.second ?: action
private fun adminProviderLabel(id: String): String = when (id) { "siliconflow" -> "硅基流动"; "aliyun" -> "阿里云"; "deepseek" -> "DeepSeek"; else -> id }
private fun adminJobAllows(status: String, action: String): Boolean = when (action) {
    "cancel" -> status !in listOf("completed", "failed", "cancelled")
    "retry" -> status == "failed" || status == "cancelled"
    "delete" -> status in listOf("completed", "failed", "cancelled")
    else -> false
}

private fun adminLimit(user: JSONObject, key: String): String = if (user.isNull(key)) "跟随默认" else if (user.optInt(key) == 0) "不限" else "${user.optInt(key)} 次"
private fun adminSegment(value: String): String = URLEncoder.encode(value, "UTF-8").replace("+", "%20")
private fun adminDate(value: String): String = if (value.isEmpty()) "暂无记录" else runCatching { OffsetDateTime.parse(value).atZoneSameInstant(java.time.ZoneId.systemDefault()).format(DateTimeFormatter.ofPattern("MM-dd HH:mm")) }.getOrDefault(value.take(16).replace('T', ' '))

private fun adminCopy(context: Context, text: String, sensitive: Boolean = false) {
    val clip = ClipData.newPlainText(if (sensitive) "CDK 密钥" else "Luxury Capture", text)
    if (sensitive) clip.description.extras = PersistableBundle().apply { putBoolean("android.content.extra.IS_SENSITIVE", true) }
    context.getSystemService(ClipboardManager::class.java).setPrimaryClip(clip)
}

private fun adminLifecycle(context: Context): Lifecycle? {
    var current = context
    while (current is ContextWrapper) {
        if (current is LifecycleOwner) return current.lifecycle
        val base = current.baseContext
        if (base === current) return null
        current = base
    }
    return (current as? LifecycleOwner)?.lifecycle
}

private fun adminCurl(operation: JSONObject, base: String): String {
    val type = operation.text("curlExample")
    if (type !in listOf("request", "create-job", "download-zip", "signed-zip")) return ""
    val path = operation.text("path")
    fun literal(value: String) = "'${value.replace("'", "''")}'"
    val url = when {
        type == "signed-zip" -> "${'$'}env:DOWNLOAD_URL"
        "{id}" in path -> "(${literal(base.trimEnd('/') + path.substringBefore("{id}"))} + ${'$'}env:JOB_ID + ${literal(path.substringAfter("{id}"))})"
        else -> literal(base.trimEnd('/') + path)
    }
    val lines = mutableListOf("curl.exe --request ${operation.text("method")} `", "  --url $url")
    if (operation.text("auth") in listOf("admin", "bearer")) {
        val variable = if (operation.text("auth") == "admin") "ADMIN_ACCESS_TOKEN" else "ACCESS_TOKEN"
        lines[lines.lastIndex] += " `"
        lines += "  --header \"Authorization: Bearer ${'$'}env:$variable\""
    }
    if (type == "create-job") {
        lines[lines.lastIndex] += " `"
        lines += "  --header 'Content-Type: application/json' `"
        lines += "  --header \"Idempotency-Key: ${'$'}([guid]::NewGuid())\" `"
        lines += "  --data-binary '@-'"
        return "${'$'}body = @{ shareText = ${'$'}env:DOUYIN_URL; action = 'download' } | ConvertTo-Json\n${'$'}body | " + lines.joinToString("\n")
    }
    if (type == "download-zip" || type == "signed-zip") {
        lines[lines.lastIndex] += " `"
        lines += "  --fail --output 'images.zip'"
    }
    return lines.joinToString("\n")
}
