package com.buleng.luxuryvideo

import com.buleng.luxuryvideo.data.AppPreferences
import com.buleng.luxuryvideo.data.CaptureJob
import com.buleng.luxuryvideo.data.UserProfile
import com.buleng.luxuryvideo.platform.DownloadSelection
import org.json.JSONObject

data class CaptureUiState(
    val preferences: AppPreferences = AppPreferences(),
    val session: UserProfile? = null,
    val initializing: Boolean = true,
    val authenticating: Boolean = false,
    val loading: Boolean = false,
    val busy: Boolean = false,
    val serverOnline: Boolean? = null,
    val serverVersion: String = "",
    val shareText: String = "",
    val externalShareId: Long = 0,
    val jobs: List<CaptureJob> = emptyList(),
    val totalJobs: Int = 0,
    val query: String = "",
    val statusFilter: String = "",
    val actionFilter: String = "",
    val hasMore: Boolean = false,
    val offline: Boolean = false,
    val selectedJob: CaptureJob? = null,
    val selectedJobAdmin: Boolean = false,
    val usage: JSONObject? = null,
    val sessions: List<JSONObject> = emptyList(),
    val adminSection: String = "overview",
    val adminData: JSONObject = JSONObject(),
    val adminResult: JSONObject? = null,
    val adminLoading: Boolean = false,
    val adminQuery: String = "",
    val adminPage: Int = 0,
    val error: String? = null,
    val notice: String? = null,
    val pendingDownload: DownloadSelection? = null,
    val downloadPreparing: Boolean = false,
    val downloadProgress: Float? = null,
    val downloadName: String = "",
)

data class AdminOperation(
    val method: String,
    val path: String,
    val body: JSONObject? = null,
)
