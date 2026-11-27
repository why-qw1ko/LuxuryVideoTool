package com.buleng.luxuryvideo

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.buleng.luxuryvideo.data.ApiException
import com.buleng.luxuryvideo.data.CaptureApi
import com.buleng.luxuryvideo.data.CaptureJob
import com.buleng.luxuryvideo.data.JobFile
import com.buleng.luxuryvideo.data.SessionStore
import com.buleng.luxuryvideo.data.SubmissionIdentity
import com.buleng.luxuryvideo.data.objects
import com.buleng.luxuryvideo.data.text
import com.buleng.luxuryvideo.platform.CaptureNotifications
import com.buleng.luxuryvideo.platform.DeviceDownload
import com.buleng.luxuryvideo.platform.DownloadSelection
import com.buleng.luxuryvideo.platform.SystemDownloads
import java.net.URLEncoder
import java.nio.charset.StandardCharsets
import java.time.Instant
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import org.json.JSONArray
import org.json.JSONObject

data class PreviewItem(
    val file: JobFile,
    val url: String,
    val headers: Map<String, String> = emptyMap(),
)

sealed interface NativeEvent {
    data class Preview(
        val file: JobFile,
        val url: String,
        val headers: Map<String, String> = emptyMap(),
        val gallery: List<PreviewItem> = emptyList(),
        val galleryIndex: Int = 0,
        val sessionEpoch: Long = -1,
        val detailVersion: Long = -1,
        val requestId: Long = -1,
    ) : NativeEvent
    data object RequestNotifications : NativeEvent
}

class CaptureViewModel(application: Application) : AndroidViewModel(application) {
    private val store = SessionStore(application)
    val appUpdates = com.buleng.luxuryvideo.platform.AppUpdateController(application, viewModelScope, store.preferences.serverUrl)
    private val api = CaptureApi(store)
    private val notifications = CaptureNotifications(application)
    private val systemDownloads = SystemDownloads(application)
    private val mutableState = MutableStateFlow(CaptureUiState(preferences = store.preferences))
    val state = mutableState.asStateFlow()
    private val mutableDownloads = MutableStateFlow<List<DeviceDownload>>(emptyList())
    val downloads = mutableDownloads.asStateFlow()
    private val nativeEvents = Channel<NativeEvent>(Channel.BUFFERED)
    val events = nativeEvents.receiveAsFlow()
    private val submissionIdentity = SubmissionIdentity()
    private val adminMutationMutex = Mutex()
    private var epoch = 0L
    private var listGeneration = 0L
    private var adminGeneration = 0L
    private var detailGeneration = 0L
    private var previewGeneration = 0L
    private var detailPoll: Job? = null
    private var foregroundPoll: Job? = null
    private var listLoad: Job? = null
    private var listRequest: String? = null
    private var adminLoad: Job? = null
    private var adminRequestPath: String? = null
    private var adminOwnersLoad: Job? = null
    private var adminOwners: JSONObject? = null
    private var adminOwnersUpdatedAt = 0L
    private var detailLoad: Job? = null
    private var downloadPreparation: Job? = null
    private var downloadPoll: Job? = null
    private var downloadsRefresh: Job? = null
    private var usageLoading = false
    private var sessionUserId: String? = null
    private var adminExtra: JSONObject? = null
    private var sessionsUserId: String? = null
    private var foreground = false
    private var activeRequests = 0
    private val observedStatuses = mutableMapOf<String, String>()

    init {
        viewModelScope.launch {
            val currentEpoch = epoch
            val (saved, cached) = withContext(Dispatchers.IO) { store.loadSession() to store.loadJobCache() }
            if (currentEpoch != epoch) return@launch
            if (saved == null) {
                mutableState.update { it.copy(initializing = false) }
                return@launch
            }
            sessionUserId = saved.user.id
            mutableState.update { it.copy(session = saved.user, jobs = cached, totalJobs = cached.size, initializing = false) }
            try {
                val restored = api.restoreSession()
                if (currentEpoch != epoch) return@launch
                sessionUserId = restored?.user?.id
                mutableState.update {
                    it.copy(
                        session = restored?.user,
                        jobs = if (restored != null) cached else emptyList(),
                        totalJobs = if (restored != null) cached.size else 0,
                        initializing = false,
                    )
                }
                if (restored != null) {
                    refresh()
                    startForegroundPolling()
                }
            } catch (error: Exception) {
                if (error is CancellationException) throw error
                if (currentEpoch != epoch) return@launch
                val retained = withContext(Dispatchers.IO) { store.loadSession() }
                if (currentEpoch != epoch) return@launch
                sessionUserId = retained?.user?.id
                mutableState.update {
                    it.copy(
                        session = retained?.user,
                        jobs = if (retained != null) cached else emptyList(),
                        offline = retained != null,
                        initializing = false,
                        error = if (retained != null) "暂时无法连接服务器，已显示本机历史。" else friendlyError(error),
                    )
                }
            }
        }
    }

    fun setShareText(text: String) {
        mutableState.update { it.copy(shareText = text) }
    }

    fun acceptSharedText(text: String) {
        closeJob()
        mutableState.update { it.copy(shareText = text, externalShareId = System.nanoTime()) }
    }

    fun clearMessage() {
        mutableState.update { it.copy(error = null, notice = null) }
    }

    fun clearAdminResult() {
        mutableState.update { it.copy(adminResult = null) }
    }

    fun login(serverUrl: String, username: String, password: String, cdk: String = "") {
        if (state.value.authenticating) return
        if (serverUrl.isBlank() || (cdk.isBlank() && (username.isBlank() || password.isEmpty()))) {
            showError("请填写服务器地址和登录信息。")
            return
        }
        invalidateSessionWork()
        val currentEpoch = epoch
        mutableState.update { it.copy(authenticating = true, error = null, session = null) }
        viewModelScope.launch {
            try {
                val session = api.login(serverUrl, username.trim(), password, cdk.trim())
                if (epoch != currentEpoch) return@launch
                sessionUserId = session.user.id
                mutableState.update {
                    CaptureUiState(
                        preferences = store.preferences,
                        session = session.user,
                        initializing = false,
                        shareText = it.shareText,
                        notice = "欢迎回来，${session.user.displayName}",
                    )
                }
                refresh()
                startForegroundPolling()
            } catch (error: Exception) {
                if (epoch == currentEpoch) {
                    mutableState.update {
                        it.copy(preferences = store.preferences, authenticating = false, error = friendlyError(error))
                    }
                }
            }
        }
    }

    fun checkConnection(serverUrl: String) {
        val candidate = serverUrl.trim().trimEnd('/')
        launchRequest {
            require(candidate.isNotBlank()) { "请填写服务器地址。" }
            val health = guardedRequest("GET", "$candidate/health/ready", authenticated = false)
            check(health.text("status") == "ready") { "服务器尚未就绪。" }
            val live = guardedRequest("GET", "$candidate/health/live", authenticated = false)
            mutableState.update {
                it.copy(
                    serverOnline = true,
                    serverVersion = live.optJSONObject("version")?.text("version").orEmpty(),
                    notice = "服务器连接正常",
                )
            }
        }
    }

    fun savePreferences(serverUrl: String, themeMode: String, motionEnabled: Boolean, notificationsEnabled: Boolean) {
        val old = state.value.preferences
        val normalized = serverUrl.trim().trimEnd('/')
        if (normalized.isNotEmpty()) {
            val uri = runCatching { java.net.URI(normalized) }.getOrNull()
            if (uri == null || uri.scheme !in setOf("http", "https") || uri.host.isNullOrBlank() || uri.userInfo != null || uri.query != null || uri.fragment != null) {
                showError("服务器地址须为有效的 HTTP 或 HTTPS 地址。")
                return
            }
        }
        val preferences = old.copy(
            serverUrl = normalized,
            themeMode = themeMode.takeIf { it in setOf("system", "light", "dark") } ?: "system",
            motionEnabled = motionEnabled,
            notificationsEnabled = notificationsEnabled,
        )
        if (old.serverUrl.trimEnd('/') != normalized) {
            invalidateSessionWork()
            store.clearSession()
            sessionUserId = null
            store.savePreferences(preferences)
            appUpdates.configureServer(preferences.serverUrl)
            mutableState.update {
                CaptureUiState(preferences = preferences, initializing = false, shareText = it.shareText, notice = "服务器已切换，请重新登录。")
            }
        } else {
            store.savePreferences(preferences)
            mutableState.update { it.copy(preferences = preferences, notice = "设置已保存") }
        }
    }

    fun logout() {
        invalidateSessionWork()
        val currentEpoch = epoch
        mutableState.update { it.copy(busy = true) }
        viewModelScope.launch {
            try {
                api.logout()
            } catch (error: Exception) {
                if (error is CancellationException) throw error
            }
            if (currentEpoch == epoch) {
                sessionUserId = null
                mutableState.update {
                    CaptureUiState(preferences = store.preferences, initializing = false, shareText = it.shareText, notice = "已退出登录")
                }
            }
        }
    }

    fun refresh() {
        if (state.value.session == null) return
        loadJobs()
        launchRequest(markBusy = false) { loadUsage() }
        state.value.selectedJob?.let { openJob(it.id, state.value.selectedJobAdmin) }
    }

    fun loadJobs(
        query: String = state.value.query,
        status: String = state.value.statusFilter,
        action: String = state.value.actionFilter,
        append: Boolean = false,
        admin: Boolean = false,
        refreshLoaded: Boolean = false,
    ) {
        if (admin) {
            loadAdmin("jobs", query, 0, JSONObject().put("status", status).put("action", action))
            return
        }
        if (state.value.session == null || (append && state.value.loading)) return
        val previous = state.value
        val sameFilters = query == previous.query && status == previous.statusFilter && action == previous.actionFilter
        val offset = if (append) previous.jobs.size else 0
        val requestedCount = if (refreshLoaded && sameFilters) previous.jobs.size.coerceAtLeast(10) else 10
        val requestKey = queryString(mapOf("q" to query, "status" to status, "action" to action, "offset" to offset.toString(), "count" to requestedCount.toString()))
        if (listLoad?.isActive == true && listRequest == requestKey) return
        listLoad?.cancel()
        listRequest = requestKey
        val generation = ++listGeneration
        val currentEpoch = epoch
        val requestUser = state.value.session?.id
        val requestServer = state.value.preferences.serverUrl
        mutableState.update {
            it.copy(loading = !refreshLoaded || it.jobs.isEmpty(), query = query, statusFilter = status, actionFilter = action)
        }
        listLoad = viewModelScope.launch {
            try {
                val received = mutableListOf<CaptureJob>()
                var total = 0
                val actions = if (action == "transcription") listOf("transcribe", "full") else listOf(action)
                val knownJobs = previous.jobs.associate { it.id to it.raw }
                for (requestedAction in actions) {
                    val actionOffset = if (actions.size == 1) offset else if (append) previous.jobs.count { it.action == requestedAction } else 0
                    val actionJobs = mutableListOf<CaptureJob>()
                    var actionTotal: Int
                    do {
                        val pageSize = (requestedCount - actionJobs.size).coerceIn(1, 100)
                        val params = mutableMapOf("q" to query, "status" to status, "action" to requestedAction, "limit" to pageSize.toString(), "offset" to (actionOffset + actionJobs.size).toString())
                        val response = fetchJobPage("/api/v1/jobs", params, knownJobs)
                        val page = withContext(Dispatchers.Default) { response.objects("jobs").map(CaptureJob::parse) }
                        actionTotal = response.optInt("total", actionOffset + actionJobs.size + page.size)
                        actionJobs += page
                        if (page.size < pageSize) break
                    } while (actionJobs.size < requestedCount && actionOffset + actionJobs.size < actionTotal)
                    total += actionTotal
                    received += actionJobs
                }
                if (generation != listGeneration || currentEpoch != epoch) return@launch
                val existingJobs = state.value.jobs
                val combined = withContext(Dispatchers.Default) {
                    val ordered = if (actions.size > 1) {
                        received.sortedWith(compareByDescending<CaptureJob> { runCatching { Instant.parse(it.createdAt) }.getOrDefault(Instant.EPOCH) }.thenByDescending { it.id }).take(requestedCount)
                    } else received
                    val known = previous.jobs.associateBy { it.id }
                    val summaries = ordered.map { job ->
                        val cachedJob = known[job.id]
                        val needsCover = job.coverUrl.isBlank() && !cachedJob?.coverUrl.isNullOrBlank()
                        val needsType = job.workType.isBlank() && !cachedJob?.workType.isNullOrBlank()
                        val resolved = if (!needsCover && !needsType) job else {
                            val cover = job.coverUrl.ifBlank { cachedJob?.coverUrl.orEmpty() }
                            val type = job.workType.ifBlank { cachedJob?.workType.orEmpty() }
                            val cachedRaw = JSONObject(job.raw.toString())
                            val work = cachedRaw.optJSONObject("work") ?: JSONObject().also { cachedRaw.put("work", it) }
                            if (needsCover) work.put("coverPreviewUrl", cover)
                            if (needsType) work.put("type", type)
                            job.copy(coverUrl = cover, workType = type, raw = cachedRaw)
                        }
                        if (cachedJob != null && cachedJob.updatedAt == resolved.updatedAt &&
                                cachedJob.status == resolved.status && cachedJob.progress == resolved.progress &&
                                cachedJob.statusMessage == resolved.statusMessage && cachedJob.action == resolved.action &&
                                cachedJob.title == resolved.title && cachedJob.authorName == resolved.authorName &&
                                cachedJob.createdAt == resolved.createdAt && cachedJob.coverUrl == resolved.coverUrl &&
                                cachedJob.workType == resolved.workType) cachedJob else resolved
                    }
                    if (append) (existingJobs + summaries).distinctBy { it.id } else summaries.distinctBy { it.id }
                }
                if (generation != listGeneration || currentEpoch != epoch) return@launch
                received.forEach(::observeJob)
                mutableState.update {
                    it.copy(jobs = combined, totalJobs = total, hasMore = combined.size < total, loading = false, offline = false)
                }
                if (query.isBlank() && status.isBlank() && action.isBlank()) {
                    try {
                        withContext(Dispatchers.IO) { store.saveJobCache(combined, requestUser, requestServer) }
                    } catch (error: Exception) {
                        if (error is CancellationException) throw error
                    }
                }
            } catch (error: Exception) {
                if (error is CancellationException) throw error
                if (generation != listGeneration || currentEpoch != epoch) return@launch
                val cached = if (sameFilters && previous.jobs.isNotEmpty()) previous.jobs else withContext(Dispatchers.IO) {
                    store.loadJobCache().filter { job ->
                        val matchesStatus = status.isBlank() || job.status == status || (status == "processing" && !job.terminal)
                        val matchesAction = action.isBlank() || job.action == action || action == "transcription" && job.action in setOf("transcribe", "full")
                        val matchesQuery = query.isBlank() || job.title.contains(query, true) || job.authorName.contains(query, true)
                        matchesStatus && matchesAction && matchesQuery
                    }
                }
                if (generation != listGeneration || currentEpoch != epoch) return@launch
                mutableState.update { it.copy(jobs = cached, totalJobs = cached.size, hasMore = false, loading = false, offline = true) }
                handleError(error, currentEpoch)
            }
        }
    }

    fun createJob(
        action: String,
        keepVideo: Boolean,
        force: Boolean = false,
        asrProvider: String = "",
        languageHints: String = "",
        hotwords: String = "",
    ) {
        if (state.value.busy) return
        val text = state.value.shareText.trim()
        if (text.isEmpty() || text.toByteArray(StandardCharsets.UTF_8).size > 4096) {
            showError("请输入不超过 4096 字节的抖音分享文本。")
            return
        }
        if (action !in setOf("info", "download", "transcribe", "full")) return
        val languages = splitOptions(languageHints.ifBlank { "zh,en" })
        val words = splitOptions(hotwords)
        if (languages.size > 8 || languages.any { it.toByteArray().size > 16 } || words.size > 100 || words.any { it.toByteArray().size > 128 }) {
            showError("语言提示最多 8 项（每项 16 字节），热词最多 100 项（每项 128 字节）。")
            return
        }
        val options = JSONObject().put("force", force).put("keepVideo", keepVideo)
            .put("languageHints", JSONArray(languages)).put("hotwords", JSONArray(words))
        if (asrProvider.isNotBlank() && action in setOf("transcribe", "full")) options.put("asrProvider", asrProvider)
        val body = JSONObject().put("shareText", text).put("action", action).put("options", options)
        val key = submissionIdentity.keyFor(body.toString())
        launchRequest {
            val result = guardedRequest("POST", "/api/v1/jobs", body, idempotencyKey = key)
            val job = CaptureJob.parse(requireNotNull(result.optJSONObject("job")))
            submissionIdentity.clear()
            mutableState.update {
                it.copy(selectedJob = job, selectedJobAdmin = false, notice = if (result.optBoolean("reused")) "已找到可复用的任务" else "任务已创建")
            }
            observeJob(job)
            listLoad?.cancel()
            loadJobs()
            loadUsage()
            startDetailPolling(job.id, false)
        }
    }

    fun openJob(id: String, admin: Boolean = false) {
        val generation = ++detailGeneration
        val currentEpoch = epoch
        detailPoll?.cancel()
        detailLoad?.cancel()
        val cached = if (admin) state.value.adminData.objects("jobs").find { it.text("id") == id }?.let(CaptureJob::parse)
            else state.value.jobs.find { it.id == id }
        mutableState.update { it.copy(selectedJob = cached, selectedJobAdmin = admin) }
        detailLoad = viewModelScope.launch {
            try {
                val job = fetchJob(id, admin)
                if (currentEpoch != epoch || generation != detailGeneration) return@launch
                observeJob(job)
                mutableState.update { it.copy(selectedJob = job, selectedJobAdmin = admin) }
                startDetailPolling(id, admin)
            } catch (error: Exception) {
                if (error is CancellationException) throw error
                if (currentEpoch != epoch || generation != detailGeneration) return@launch
                handleError(error, currentEpoch)
            }
        }
    }

    fun closeJob() {
        detailGeneration++
        detailPoll?.cancel()
        detailLoad?.cancel()
        mutableState.update { it.copy(selectedJob = null, selectedJobAdmin = false) }
    }

    fun jobAction(jobId: String, action: String, admin: Boolean = false) {
        if (action !in setOf("cancel", "retry", "delete")) return
        launchRequest {
            val base = if (admin) "/api/v1/admin/jobs" else "/api/v1/jobs"
            val path = "$base/${encode(jobId)}" + if (action == "delete") "" else "/$action"
            val response = guardedRequest(if (action == "delete") "DELETE" else "POST", path)
            if (action == "delete") {
                if (state.value.selectedJob?.id == jobId) closeJob()
                mutableState.update { it.copy(jobs = it.jobs.filterNot { job -> job.id == jobId }, notice = "任务及关联文件已删除") }
            } else {
                response.optJSONObject("job")?.let { raw ->
                    val job = CaptureJob.parse(raw)
                    if (state.value.selectedJob?.id == jobId) mutableState.update { it.copy(selectedJob = job) }
                    observeJob(job)
                    startDetailPolling(jobId, admin)
                }
                mutableState.update { it.copy(notice = if (action == "cancel") "任务已取消" else "已安排重试") }
            }
            if (admin) reloadAdmin() else {
                listLoad?.cancel()
                loadJobs()
            }
        }
    }

    fun loadSessions(userId: String? = null) {
        sessionsUserId = userId
        mutableState.update { it.copy(sessions = emptyList()) }
        launchRequest {
            val path = if (userId == null) "/api/v1/auth/sessions" else "/api/v1/admin/users/${encode(userId)}/sessions"
            val response = guardedRequest("GET", path)
            if (sessionsUserId == userId) mutableState.update { it.copy(sessions = response.objects("sessions")) }
        }
    }

    fun revokeSession(id: String, admin: Boolean = false) {
        val current = state.value.sessions.find { it.text("id") == id }?.optBoolean("current") == true
        launchRequest {
            guardedRequest("DELETE", "/api/v1/${if (admin) "admin" else "auth"}/sessions/${encode(id)}")
            if (current) {
                expireLocalSession("当前设备会话已退出。")
            } else {
                mutableState.update { it.copy(notice = "设备会话已下线") }
                loadSessions(if (admin) sessionsUserId else null)
            }
        }
    }

    fun changePassword(currentPassword: String, newPassword: String) {
        if (currentPassword.isEmpty() || newPassword.length !in 6..12) {
            showError("请填写当前密码，新密码须为 6 至 12 位。")
            return
        }
        launchRequest {
            guardedRequest("POST", "/api/v1/auth/password", JSONObject().put("currentPassword", currentPassword).put("newPassword", newPassword))
            mutableState.update { it.copy(notice = "密码已修改，其他设备会话已失效。") }
        }
    }

    fun loadAdmin(section: String, query: String = "", page: Int = 0, extra: JSONObject? = null, refreshLoaded: Boolean = false) {
        if (state.value.session?.role != "admin") return
        val previousJobs = state.value.adminData.objects("jobs").associateBy { it.text("id") }
        val defaultPageSize = if (section == "jobs") 10 else 20
        val pageSize = (extra?.optInt("limit", defaultPageSize) ?: defaultPageSize).coerceIn(1, 100)
        val params = mutableMapOf("q" to query, "limit" to pageSize.toString(), "offset" to (page.coerceAtLeast(0) * pageSize).toString())
        extra?.keys()?.forEach { key -> params[key] = extra.text(key) }
        if (section == "jobs") params["view"] = "summary"
        val path = when (section) {
            "overview" -> "/api/v1/admin/stats"
            "users" -> "/api/v1/admin/users?includeDeletedJobOwners=1"
            "jobs" -> "/api/v1/admin/jobs?${queryString(params)}"
            "cdks" -> "/api/v1/admin/cdks?${queryString(params)}"
            "providers" -> "/api/v1/admin/settings/providers"
            "usage" -> "/api/v1/admin/settings/usage"
            "api" -> "/api/v1/admin/api-catalog"
            else -> return
        }
        if (adminLoad?.isActive == true && adminRequestPath == path) return
        adminLoad?.cancel()
        adminRequestPath = path
        val generation = ++adminGeneration
        val currentEpoch = epoch
        val switching = state.value.adminSection != section
        adminExtra = extra
        mutableState.update {
            it.copy(adminSection = section, adminQuery = query, adminPage = page.coerceAtLeast(0), adminLoading = true, adminData = if (switching) JSONObject() else it.adminData)
        }
        if (section == "jobs") loadAdminOwners(currentEpoch)
        adminLoad = viewModelScope.launch {
            try {
                val response = if (section == "jobs") fetchJobPage("/api/v1/admin/jobs", params, previousJobs)
                    else guardedRequest("GET", path)
                if (currentEpoch != epoch || generation != adminGeneration) return@launch
                response.put("_filters", extra ?: JSONObject())
                if (section == "jobs") {
                    response.objects("jobs").forEach { job ->
                        val oldWork = previousJobs[job.text("id")]?.optJSONObject("work")
                        val work = job.optJSONObject("work") ?: JSONObject().also { job.put("work", it) }
                        if (work.text("coverPreviewUrl").isBlank() && work.text("coverUrl").isBlank()) {
                            work.put("coverPreviewUrl", oldWork?.text("coverPreviewUrl").orEmpty().ifBlank { oldWork?.text("coverUrl").orEmpty() })
                        }
                        if (work.text("type").isBlank()) work.put("type", oldWork?.text("type").orEmpty())
                    }
                    response.put("_users", adminOwners?.optJSONArray("users") ?: JSONArray())
                    response.put("_deletedJobOwners", adminOwners?.optJSONArray("deletedJobOwners") ?: JSONArray())
                } else if (section == "users") {
                    adminOwners = response
                    adminOwnersUpdatedAt = System.currentTimeMillis()
                }
                if (currentEpoch == epoch && generation == adminGeneration) {
                    mutableState.update { it.copy(adminData = response, adminLoading = false) }
                }
            } catch (error: Exception) {
                if (error is CancellationException) throw error
                if (currentEpoch == epoch && generation == adminGeneration) {
                    mutableState.update { it.copy(adminLoading = false) }
                    handleError(error, currentEpoch)
                }
            }
        }
    }

    private fun loadAdminOwners(expectedEpoch: Long) {
        if (adminOwnersLoad?.isActive == true || (adminOwners != null && System.currentTimeMillis() - adminOwnersUpdatedAt < 60_000)) return
        adminOwnersLoad = viewModelScope.launch {
            try {
                val users = guardedRequest("GET", "/api/v1/admin/users?includeDeletedJobOwners=1")
                if (expectedEpoch != epoch) return@launch
                adminOwners = users
                adminOwnersUpdatedAt = System.currentTimeMillis()
                mutableState.update { current ->
                    if (current.adminSection != "jobs") current else {
                        val updated = JSONObject().apply { current.adminData.keys().forEach { key -> put(key, current.adminData.opt(key)) } }
                        updated.put("_users", users.optJSONArray("users") ?: JSONArray())
                        updated.put("_deletedJobOwners", users.optJSONArray("deletedJobOwners") ?: JSONArray())
                        current.copy(adminData = updated)
                    }
                }
            } catch (error: Exception) {
                if (error is CancellationException) throw error
                if (error is ApiException && error.status == 401) handleError(error, expectedEpoch)
            }
        }
    }

    fun adminRequest(method: String, path: String, body: JSONObject? = null, success: String = "已保存") {
        launchRequest {
            adminMutationMutex.withLock {
                require(state.value.session?.role == "admin") { "需要管理员权限。" }
                require(path.startsWith("/api/v1/admin/")) { "无效的管理接口。" }
                val result = guardedRequest(method, path, body)
                if (path.startsWith("/api/v1/admin/users")) invalidateAdminOwners()
                mutableState.update { it.copy(adminResult = result, notice = success) }
                if (!(method == "POST" && path.startsWith("/api/v1/admin/settings/providers/") && path.endsWith("/test"))) {
                    reloadAdmin()
                    loadUsage()
                }
            }
        }
    }

    fun batchAdminRequests(requests: List<AdminOperation>, success: String = "批量操作完成") {
        if (requests.isEmpty()) return
        launchRequest {
            adminMutationMutex.withLock {
                require(state.value.session?.role == "admin") { "需要管理员权限。" }
                val requestEpoch = epoch
                val results = JSONArray()
                val codes = JSONArray()
                var succeeded = 0
                for (operation in requests) {
                    if (requestEpoch != epoch) return@withLock
                    require(operation.path.startsWith("/api/v1/admin/")) { "无效的管理接口。" }
                    try {
                        val result = guardedRequest(operation.method, operation.path, operation.body)
                        if (requestEpoch != epoch) return@withLock
                        results.put(JSONObject().put("success", true).put("response", result))
                        result.objects("items").forEach(codes::put)
                        result.optJSONObject("item")?.let { if (it.text("code").isNotEmpty()) codes.put(it) }
                        succeeded++
                    } catch (error: Exception) {
                        if (error is CancellationException) throw error
                        if (error is ApiException && error.status == 401) throw error
                        results.put(JSONObject().put("success", false).put("error", friendlyError(error)))
                    }
                }
                mutableState.update {
                    it.copy(
                        adminResult = JSONObject().put("results", results).put("items", codes).put("succeeded", succeeded).put("failed", requests.size - succeeded),
                        notice = "$success：成功 $succeeded 项，失败 ${requests.size - succeeded} 项",
                    )
                }
                if (requests.any { it.path.startsWith("/api/v1/admin/users") }) invalidateAdminOwners()
                reloadAdmin()
            }
        }
    }

    fun download(file: JobFile, admin: Boolean = false) {
        if (state.value.downloadPreparing) return
        if (file.isExpired) {
            showError("服务器文件已超过保留期，请重新创建下载任务。")
            return
        }
        mutableState.update {
            it.copy(pendingDownload = DownloadSelection(
                name = file.name, mimeType = file.mimeType, sizeBytes = file.sizeBytes,
                fileId = if (file.kind == "music") "" else file.id, jobId = it.selectedJob?.id.orEmpty(), admin = admin,
                sourceKind = if (file.kind == "music") "music" else "",
            ))
        }
    }

    fun downloadSource(url: String, name: String, mimeType: String = "audio/mpeg") {
        if (state.value.downloadPreparing) return
        val job = state.value.selectedJob ?: return
        val expected = job.music.takeIf { it.source == "original" }?.file?.previewUrl.orEmpty()
        if (expected.isBlank() || runCatching { api.absoluteUrl(url) != api.absoluteUrl(expected) }.getOrDefault(true)) {
            showError("原声地址已变化，请刷新任务后重试。")
            return
        }
        mutableState.update {
            it.copy(pendingDownload = DownloadSelection(
                name = name, mimeType = mimeType, jobId = job.id, admin = it.selectedJobAdmin, sourceKind = "music",
            ))
        }
    }

    fun downloadArchive(job: CaptureJob, admin: Boolean = false) {
        if (state.value.downloadPreparing) return
        mutableState.update {
            it.copy(pendingDownload = DownloadSelection(
                name = "${job.title.take(60)}.zip", mimeType = "application/zip", jobId = job.id, admin = admin, archive = true,
            ))
        }
    }

    fun dismissDownload() {
        if (!state.value.downloadPreparing) mutableState.update { it.copy(pendingDownload = null) }
    }

    fun confirmDownload(wifiOnly: Boolean = false) {
        val selection = state.value.pendingDownload ?: return
        val ownerId = state.value.session?.id ?: return
        if (downloadPreparation?.isActive == true || state.value.downloadPreparing) return
        val currentEpoch = epoch
        val server = state.value.preferences.serverUrl
        mutableState.update { it.copy(downloadPreparing = true) }
        downloadPreparation = viewModelScope.launch {
            try {
                var resolvedSelection = selection
                val (url, headers) = if (selection.archive) {
                    val response = guardedRequest("POST", "/api/v1/${if (selection.admin) "admin/" else ""}jobs/${encode(selection.jobId)}/images/archive-download")
                    val signed = response.text("downloadUrl")
                    check(signed.isNotBlank()) { "未能获取打包下载地址。" }
                    api.prepareSystemDownload(signed, authenticated = false)
                } else if (selection.sourceKind == "music") {
                    val freshJob = fetchJob(selection.jobId, selection.admin)
                    val music = freshJob.music
                    val savedMusic = music.file?.takeIf { music.source == "server" }
                    if (savedMusic != null) {
                        resolvedSelection = selection.copy(name = savedMusic.name, mimeType = savedMusic.mimeType, sizeBytes = savedMusic.sizeBytes, fileId = savedMusic.id)
                        val path = savedMusic.previewUrl.ifBlank { "/api/v1/${if (selection.admin) "admin/" else ""}files/${encode(savedMusic.id)}" }
                        api.prepareSystemDownload(path, authenticated = savedMusic.previewUrl.isBlank())
                    } else {
                        val source = music.file?.previewUrl.orEmpty()
                        check(source.isNotBlank()) { "原声源地址暂不可用，请刷新任务后重试。" }
                        resolvedSelection = selection.copy(fileId = "")
                        val resolved = api.absoluteUrl(source)
                        val extension = java.net.URI(resolved).path.orEmpty().substringAfterLast('.').lowercase()
                        val mime = when (extension) {
                            "m4a", "mp4" -> "audio/mp4"
                            "aac" -> "audio/aac"
                            "ogg", "opus" -> "audio/ogg"
                            "wav" -> "audio/wav"
                            "flac" -> "audio/flac"
                            else -> selection.mimeType
                        }
                        if (extension in setOf("m4a", "mp4", "aac", "ogg", "opus", "wav", "flac", "mp3")) {
                            resolvedSelection = resolvedSelection.copy(name = selection.name.substringBeforeLast('.', selection.name) + ".$extension", mimeType = mime)
                        }
                        resolved to mapOf("Referer" to "https://www.douyin.com/")
                    }
                } else {
                    val freshJob = if (selection.jobId.isNotBlank()) fetchJob(selection.jobId, selection.admin) else null
                    val file = freshJob?.files?.find { it.id == selection.fileId }
                    if (freshJob != null) check(file != null) { "文件已不存在，请刷新任务。" }
                    check(file?.isExpired != true) { "服务器文件已过期，请重新创建下载任务。" }
                    if (file != null) resolvedSelection = selection.copy(name = file.name, mimeType = file.mimeType, sizeBytes = file.sizeBytes)
                    val signed = file?.previewUrl.orEmpty()
                    val path = signed.ifBlank { "/api/v1/${if (selection.admin) "admin/" else ""}files/${encode(selection.fileId)}" }
                    api.prepareSystemDownload(path, authenticated = signed.isBlank())
                }
                if (currentEpoch != epoch) return@launch
                systemDownloads.enqueue(url, headers, resolvedSelection, ownerId, server, wifiOnly)
                if (currentEpoch != epoch) return@launch
                selection.retryId?.let { systemDownloads.removeRecord(it) }
                if (currentEpoch != epoch) return@launch
                mutableState.update { it.copy(pendingDownload = null, notice = "已加入下载") }
                refreshDownloads()
            } catch (error: Exception) {
                if (error is CancellationException) throw error
                handleError(error, currentEpoch)
            } finally {
                if (currentEpoch == epoch) mutableState.update { it.copy(downloadPreparing = false) }
            }
        }
    }

    fun refreshDownloads() = loadDownloads(reportError = true)

    private fun loadDownloads(reportError: Boolean = false) {
        val ownerId = state.value.session?.id ?: return
        if (downloadsRefresh?.isActive == true) return
        val currentEpoch = epoch
        val server = state.value.preferences.serverUrl
        downloadsRefresh = viewModelScope.launch {
            try {
                val items = systemDownloads.list(ownerId, server)
                if (currentEpoch == epoch) mutableDownloads.value = items
            } catch (error: Exception) {
                if (error is CancellationException) throw error
                if (reportError) handleError(error, currentEpoch)
            }
        }
    }

    fun cancelDownload(id: Long) {
        if (mutableDownloads.value.none { it.id == id && it.active }) return
        launchRequest(markBusy = false) {
            systemDownloads.cancel(id)
            refreshDownloads()
        }
    }

    fun removeDownload(id: Long, deleteLocalFile: Boolean = false) {
        if (mutableDownloads.value.none { it.id == id && !it.active }) return
        launchRequest(markBusy = false) {
            systemDownloads.removeRecord(id, deleteLocalFile)
            refreshDownloads()
        }
    }

    fun retryDownload(id: Long) {
        if (state.value.downloadPreparing) return
        val download = mutableDownloads.value.find { it.id == id && !it.active } ?: return
        mutableState.update { it.copy(pendingDownload = download.source.copy(retryId = id)) }
    }

    private fun startDownloadPolling() {
        downloadPoll?.cancel()
        if (!foreground || state.value.session == null) return
        downloadPoll = viewModelScope.launch {
            while (isActive) {
                loadDownloads()
                delay(1_000)
            }
        }
    }

    fun dismissPreview() { previewGeneration++ }

    fun isPreviewSessionCurrent(event: NativeEvent.Preview): Boolean = state.value.session != null &&
        event.sessionEpoch == epoch && event.detailVersion == detailGeneration

    fun isCurrentPreview(event: NativeEvent.Preview): Boolean =
        isPreviewSessionCurrent(event) && event.requestId == previewGeneration

    fun preview(file: JobFile, refreshMusic: Boolean = false) {
        val selected = state.value.selectedJob
        val admin = state.value.selectedJobAdmin
        val generation = detailGeneration
        val currentEpoch = epoch
        val requestId = ++previewGeneration
        launchRequest(markBusy = false) {
            val fresh = if (selected != null && (file.id.isNotBlank() || file.kind == "music")) {
                if (file.kind == "music" && refreshMusic) refreshMusicJob(selected.id, admin) else fetchJob(selected.id, admin)
            } else null
            val refreshedFile = if (fresh != null) {
                if (file.kind == "music") fresh.music.file ?: error("原声暂不可用，请刷新任务后重试。")
                else fresh.files.find { it.id == file.id } ?: error("媒体已移除，请刷新任务后重试。")
            } else file
            val item = resolvePreview(refreshedFile, admin)
            if (currentEpoch != epoch || generation != detailGeneration || requestId != previewGeneration || state.value.selectedJob?.id != selected?.id) return@launchRequest
            if (fresh != null) mutableState.update { it.copy(selectedJob = fresh) }
            nativeEvents.send(NativeEvent.Preview(item.file, item.url, item.headers, sessionEpoch = currentEpoch, detailVersion = generation, requestId = requestId))
        }
    }

    fun previewGallery(files: List<JobFile>, index: Int) {
        if (index !in files.indices) return
        val selected = state.value.selectedJob
        val admin = state.value.selectedJobAdmin
        val generation = detailGeneration
        val currentEpoch = epoch
        val requestId = ++previewGeneration
        launchRequest(markBusy = false) {
            val fresh = selected?.let { fetchJob(it.id, admin) }
            val images = fresh?.raw?.optJSONObject("work")?.objects("images").orEmpty()
            val gallery = files.mapIndexed { position, file ->
                val saved = fresh?.files?.find { file.id.isNotBlank() && it.id == file.id }?.takeUnless { it.isExpired }
                val refreshedFile = saved ?: if (fresh != null) {
                    val source = images.getOrNull(position)?.text(if (file.kind == "animated") "animatedUrl" else "url").orEmpty()
                    check(source.isNotBlank()) { "图片已失效，请刷新任务后重试。" }
                    file.copy(id = "", sizeBytes = 0, expiresAt = "", previewUrl = source, thumbnailUrl = "")
                } else file
                resolvePreview(refreshedFile, admin)
            }
            if (currentEpoch != epoch || generation != detailGeneration || requestId != previewGeneration || state.value.selectedJob?.id != selected?.id) return@launchRequest
            if (fresh != null) mutableState.update { it.copy(selectedJob = fresh) }
            val active = gallery[index]
            nativeEvents.send(NativeEvent.Preview(active.file, active.url, active.headers, gallery, index, sessionEpoch = currentEpoch, detailVersion = generation, requestId = requestId))
        }
    }

    private suspend fun resolvePreview(file: JobFile, admin: Boolean): PreviewItem {
        check(!file.isExpired) { "服务器媒体已过期，请重新创建下载任务。" }
        if (file.previewUrl.isNotBlank()) return PreviewItem(
            file, api.absoluteUrl(file.previewUrl),
            if (file.kind == "music" && file.id.isBlank()) mapOf("Referer" to "https://www.douyin.com/") else emptyMap(),
        )
        check(file.id.isNotBlank()) { "媒体暂时无法预览，请刷新任务后重试。" }
        val (url, headers) = api.prepareSystemDownload("/api/v1/${if (admin) "admin/" else ""}files/${encode(file.id)}")
        return PreviewItem(file, url, headers)
    }

    fun previewUrl(url: String, mimeType: String = "image/jpeg", name: String = "媒体预览") {
        val currentEpoch = epoch
        val generation = detailGeneration
        val requestId = ++previewGeneration
        launchRequest(markBusy = false) {
            val resolved = api.absoluteUrl(url)
            val uri = java.net.URI(resolved)
            require(uri.scheme in setOf("https", "http") && uri.userInfo == null) { "无效的媒体地址。" }
            if (currentEpoch == epoch && generation == detailGeneration && requestId == previewGeneration) {
                nativeEvents.send(NativeEvent.Preview(JobFile("", "image", name, mimeType, 0, previewUrl = resolved), resolved, sessionEpoch = currentEpoch, detailVersion = generation, requestId = requestId))
            }
        }
    }

    fun setForeground(value: Boolean) {
        foreground = value
        if (value) {
            startForegroundPolling()
            state.value.selectedJob?.takeUnless { it.terminal }?.let { startDetailPolling(it.id, state.value.selectedJobAdmin) }
        } else {
            foregroundPoll?.cancel()
            detailPoll?.cancel()
            downloadPoll?.cancel()
        }
    }

    private fun startForegroundPolling() {
        foregroundPoll?.cancel()
        startDownloadPolling()
        if (!foreground || state.value.session == null) return
        foregroundPoll = viewModelScope.launch {
            while (isActive) {
                delay(8_000)
                if (state.value.jobs.any { !it.terminal } && listLoad?.isActive != true) loadJobs(refreshLoaded = true)
            }
        }
    }

    private fun startDetailPolling(id: String, admin: Boolean) {
        detailPoll?.cancel()
        if (!foreground || state.value.selectedJob?.terminal == true) return
        val currentEpoch = epoch
        val generation = detailGeneration
        detailPoll = viewModelScope.launch {
            while (isActive && currentEpoch == epoch && generation == detailGeneration && state.value.selectedJob?.id == id) {
                delay(if (state.value.selectedJob?.status in setOf("queued", "resolving")) 2_000 else 3_000)
                try {
                    val job = fetchJob(id, admin)
                    if (currentEpoch != epoch || generation != detailGeneration) return@launch
                    observeJob(job)
                    mutableState.update { it.copy(selectedJob = job, jobs = it.jobs.map { previous -> if (previous.id == id) job else previous }) }
                    if (job.terminal) {
                        loadUsage()
                        return@launch
                    }
                } catch (error: Exception) {
                    if (error is CancellationException) throw error
                    if (error is ApiException && error.status in setOf(401, 404)) {
                        handleError(error, currentEpoch)
                        return@launch
                    }
                    delay(7_000)
                }
            }
        }
    }

    private suspend fun fetchJobPage(path: String, params: Map<String, String>, known: Map<String, JSONObject>): JSONObject {
        val pageParams = params - "view"
        val response = guardedRequest("GET", "$path?${queryString(pageParams + ("view" to "summary"))}")
        if (response.optInt("summaryVersion") >= 2) return response
        val needsFullPage = response.objects("jobs").any { job ->
            val work = job.optJSONObject("work")
            val cached = known[job.text("id")]?.optJSONObject("work")
            val type = work?.text("type").orEmpty().ifBlank { cached?.text("type").orEmpty() }
            val cover = work?.text("coverPreviewUrl").orEmpty().ifBlank { work?.text("coverUrl").orEmpty() }
                .ifBlank { cached?.text("coverPreviewUrl").orEmpty().ifBlank { cached?.text("coverUrl").orEmpty() } }
            val coverExpired = runCatching {
                java.net.URI(cover).rawQuery.orEmpty().split('&').firstOrNull { it.startsWith("expires=") }
                    ?.substringAfter('=')?.toLongOrNull()?.let { it <= Instant.now().epochSecond + 10 } ?: false
            }.getOrDefault(true)
            work != null && (type.isBlank() || ((cover.isBlank() || coverExpired) && job.text("status") == "completed"))
        }
        // 旧服务缺少列表展示字段时，当前页最多补取一次完整列表。
        return if (needsFullPage) guardedRequest("GET", "$path?${queryString(pageParams)}") else response
    }

    private suspend fun refreshMusicJob(id: String, admin: Boolean): CaptureJob {
        val currentEpoch = epoch
        val response = try {
            guardedRequest("POST", "/api/v1/${if (admin) "admin/" else ""}jobs/${encode(id)}/music/refresh")
        } catch (error: ApiException) {
            if (currentEpoch != epoch) throw CancellationException("登录状态已改变")
            if (error.status !in setOf(404, 429, 502, 503, 504)) throw error
            return fetchJob(id, admin)
        }
        val job = withContext(Dispatchers.Default) { CaptureJob.parse(requireNotNull(response.optJSONObject("job"))) }
        if (currentEpoch != epoch) throw CancellationException("登录状态已改变")
        return job
    }

    private suspend fun fetchJob(id: String, admin: Boolean): CaptureJob {
        val currentEpoch = epoch
        val response = guardedRequest("GET", "/api/v1/${if (admin) "admin/" else ""}jobs/${encode(id)}")
        val job = withContext(Dispatchers.Default) { CaptureJob.parse(requireNotNull(response.optJSONObject("job"))) }
        if (currentEpoch != epoch) throw CancellationException("登录状态已改变")
        return job
    }

    private suspend fun loadUsage() {
        if (usageLoading) return
        usageLoading = true
        val currentEpoch = epoch
        try {
            val response = guardedRequest("GET", "/api/v1/jobs/usage")
            if (currentEpoch == epoch) mutableState.update { it.copy(usage = response.optJSONObject("usage")) }
        } finally {
            if (currentEpoch == epoch) usageLoading = false
        }
    }

    private fun reloadAdmin() {
        adminLoad?.cancel()
        val value = state.value
        loadAdmin(value.adminSection, value.adminQuery, value.adminPage, adminExtra)
    }

    private fun invalidateAdminOwners() {
        adminOwnersLoad?.cancel()
        adminOwners = null
        adminOwnersUpdatedAt = 0L
    }

    private fun observeJob(job: CaptureJob) {
        val previous = observedStatuses.put(job.id, job.status)
        if (previous != null && previous != job.status && job.terminal && state.value.preferences.notificationsEnabled) {
            notifications.jobCompleted(job.id, job.title, job.status == "completed")
        }
    }

    private fun launchRequest(markBusy: Boolean = true, block: suspend () -> Unit) {
        val currentEpoch = epoch
        if (markBusy) {
            activeRequests++
            mutableState.update { it.copy(busy = true, error = null) }
        }
        viewModelScope.launch {
            try {
                if (currentEpoch != epoch) return@launch
                block()
            } catch (error: Exception) {
                if (error is CancellationException) throw error
                handleError(error, currentEpoch)
            } finally {
                if (markBusy && currentEpoch == epoch) {
                    activeRequests = (activeRequests - 1).coerceAtLeast(0)
                    mutableState.update { it.copy(busy = activeRequests > 0) }
                }
            }
        }
    }

    private suspend fun guardedRequest(
        method: String,
        path: String,
        body: JSONObject? = null,
        authenticated: Boolean = true,
        idempotencyKey: String? = null,
    ): JSONObject {
        val requestEpoch = epoch
        val result = api.request(method, path, body, authenticated, idempotencyKey)
        if (requestEpoch != epoch) throw CancellationException("登录状态已改变")
        return result
    }

    private fun handleError(error: Exception, expectedEpoch: Long) {
        if (expectedEpoch != epoch || error is CancellationException) return
        if (error is ApiException && error.status == 401 && store.loadSession() == null && state.value.session != null) {
            expireLocalSession("登录状态已失效，请重新登录。")
        } else {
            showError(friendlyError(error))
        }
    }

    private fun expireLocalSession(message: String) {
        invalidateSessionWork()
        store.clearSession()
        sessionUserId = null
        mutableState.update {
            CaptureUiState(preferences = store.preferences, initializing = false, shareText = it.shareText, error = message)
        }
    }

    private fun invalidateSessionWork() {
        epoch++
        previewGeneration++
        activeRequests = 0
        listGeneration++
        adminGeneration++
        detailGeneration++
        detailPoll?.cancel()
        foregroundPoll?.cancel()
        listLoad?.cancel()
        adminLoad?.cancel()
        adminOwnersLoad?.cancel()
        detailLoad?.cancel()
        downloadPreparation?.cancel()
        downloadPoll?.cancel()
        downloadsRefresh?.cancel()
        mutableDownloads.value = emptyList()
        mutableState.update { it.copy(pendingDownload = null, downloadPreparing = false) }
        listRequest = null
        adminRequestPath = null
        adminOwners = null
        adminOwnersUpdatedAt = 0L
        usageLoading = false
        submissionIdentity.clear()
        observedStatuses.clear()
        adminExtra = null
        sessionsUserId = null
    }

    private fun showError(message: String) {
        mutableState.update { it.copy(error = message) }
    }

    private fun friendlyError(error: Exception): String = when (error) {
        is ApiException -> error.message + if (error.requestId.isNotBlank()) "\n请求编号：${error.requestId}" else ""
        is java.net.SocketTimeoutException -> "连接超时。任务可能已提交，请先查看历史；再次提交相同内容会复用本次请求。"
        is java.net.UnknownHostException -> "找不到服务器，请检查地址和网络。"
        is java.net.ConnectException -> "无法连接服务器，请检查地址、端口和服务状态。"
        is javax.net.ssl.SSLException -> "HTTPS 连接失败，请检查服务器证书。"
        is java.io.IOException -> "网络连接中断，请检查网络后重试。"
        else -> error.message?.takeIf { it.isNotBlank() } ?: "操作未完成，请稍后重试。"
    }

    private fun splitOptions(value: String): List<String> = value.split(Regex("[,，\\n]")).map(String::trim).filter(String::isNotEmpty).distinct()
    private fun encode(value: String): String = URLEncoder.encode(value, StandardCharsets.UTF_8.name())
    private fun queryString(values: Map<String, String>): String = values.entries.joinToString("&") { "${encode(it.key)}=${encode(it.value)}" }
}
