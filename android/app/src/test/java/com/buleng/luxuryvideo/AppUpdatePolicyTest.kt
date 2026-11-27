package com.buleng.luxuryvideo

import com.buleng.luxuryvideo.data.AndroidRelease
import com.buleng.luxuryvideo.data.MAX_APP_UPDATE_BYTES
import com.buleng.luxuryvideo.data.appUpdateDownloadUri
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class AppUpdatePolicyTest {
    private val release = AndroidRelease(
        id = "release-2",
        versionCode = 2,
        versionName = "1.1",
        changelog = "更新说明",
        forceUpdate = true,
        packageName = "com.buleng.luxuryvideo",
        size = 1024,
        sha256 = "a".repeat(64),
        downloadUrl = "/api/v1/app/android/apks/release-2",
        publishedAt = "2026-11-27T00:00:00Z",
    )

    @Test
    fun validPublishedReleaseKeepsVersionAndForcedPolicy() {
        release.validate("com.buleng.luxuryvideo")
        assertEquals(2L, release.versionCode)
        assertTrue(release.forceUpdate)
    }

    @Test
    fun installationRequiresExpectedPackageAndBoundedRealVersion() {
        assertRejected { release.copy(packageName = "other.package").validate(release.packageName) }
        assertRejected { release.copy(versionCode = 0).validate(release.packageName) }
        assertRejected { release.copy(versionCode = 2_100_000_001L).validate(release.packageName) }
    }

    @Test
    fun updatePayloadMustHaveBoundedSizeAndCompleteHash() {
        assertRejected { release.copy(size = 0).validate(release.packageName) }
        assertRejected { release.copy(size = MAX_APP_UPDATE_BYTES + 1).validate(release.packageName) }
        assertRejected { release.copy(sha256 = "short").validate(release.packageName) }
    }

    @Test
    fun releaseCannotPointAtAnotherServerOrTraversalPath() {
        listOf("https://other.example/update.apk", "//other.example/update.apk", "/updates/../private.apk", "/updates/a.apk?token=secret").forEach { path ->
            assertRejected { release.copy(downloadUrl = path).validate(release.packageName) }
        }
    }

    @Test
    fun updateRequestResolvesOnlyOnConfiguredOrigin() {
        assertEquals("https://capture.example:8443/api/v1/app/android/version",
            appUpdateDownloadUri("https://capture.example:8443", "/api/v1/app/android/version").toASCIIString())
        assertRejected { appUpdateDownloadUri("https://capture.example", "https://other.example/update.apk") }
        assertRejected { appUpdateDownloadUri("https://capture.example", "http://capture.example/update.apk") }
        assertRejected { appUpdateDownloadUri("https://capture.example:8443", "https://capture.example/update.apk") }
    }

    @Test
    fun serverAddressCannotContainCredentialsOrSessionQueries() {
        assertRejected { appUpdateDownloadUri("https://user:password@capture.example", release.downloadUrl) }
        assertRejected { appUpdateDownloadUri("https://capture.example?token=secret", release.downloadUrl) }
        assertRejected { appUpdateDownloadUri("file:///tmp/update.apk", release.downloadUrl) }
    }

    private fun assertRejected(action: () -> Unit) {
        assertTrue(runCatching(action).isFailure)
    }
}
