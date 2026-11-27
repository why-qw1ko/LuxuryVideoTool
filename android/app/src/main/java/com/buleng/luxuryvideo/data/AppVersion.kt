package com.buleng.luxuryvideo.data

import android.content.Context

data class AppVersion(val code: Long, val name: String) {
    companion object {
        @Suppress("DEPRECATION")
        fun current(context: Context): AppVersion {
            val info = context.packageManager.getPackageInfo(context.packageName, 0)
            return AppVersion(info.longVersionCode, info.versionName.orEmpty())
        }
    }
}
