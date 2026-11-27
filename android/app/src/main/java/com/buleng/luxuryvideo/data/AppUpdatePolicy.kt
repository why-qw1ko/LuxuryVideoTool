package com.buleng.luxuryvideo.data

import java.net.URI

const val MAX_APP_UPDATE_BYTES = 256L * 1024 * 1024

data class AndroidRelease(
    val id: String,
    val versionCode: Long,
    val versionName: String,
    val changelog: String,
    val forceUpdate: Boolean,
    val packageName: String,
    val size: Long,
    val sha256: String,
    val downloadUrl: String,
    val publishedAt: String,
) {
    fun validate(expectedPackage: String) {
        require(id.matches(Regex("[A-Za-z0-9_-]{1,128}"))) { "更新包标识不正确。" }
        require(versionCode in 1..2_100_000_000L && versionName.isNotBlank() && versionName.length <= 128) { "新版版本信息不正确。" }
        require(changelog.codePointCount(0, changelog.length) <= 10_000) { "更新说明过长。" }
        require(packageName == expectedPackage) { "更新包不属于当前应用。" }
        require(size in 1..MAX_APP_UPDATE_BYTES) { "更新包大小不正确。" }
        require(sha256.matches(Regex("[a-f0-9]{64}"))) { "更新包校验信息不正确。" }
        val path = URI(downloadUrl)
        require(downloadUrl.startsWith("/") && !downloadUrl.startsWith("//") && !path.isAbsolute &&
            path.rawAuthority == null && path.rawQuery == null && path.rawFragment == null &&
            path.normalize().rawPath == path.rawPath
        ) { "更新包下载地址不正确。" }
    }
}

fun appUpdateDownloadUri(server: String, path: String): URI {
    val base = URI(server.trim().trimEnd('/'))
    require(base.scheme in setOf("http", "https") && !base.host.isNullOrBlank() &&
        base.rawUserInfo == null && base.rawQuery == null && base.rawFragment == null
    ) { "请先设置有效的服务器地址。" }
    val target = base.resolve(path)
    require(target.scheme == base.scheme && target.host.equals(base.host, true) && target.port == base.port &&
        target.rawUserInfo == null && target.rawFragment == null
    ) { "更新包必须由当前服务器提供。" }
    return target
}
