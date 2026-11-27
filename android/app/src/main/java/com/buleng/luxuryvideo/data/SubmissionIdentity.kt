package com.buleng.luxuryvideo.data

import java.util.UUID

/** 在结果未知时保留同一次提交的幂等键，避免网络重试重复消耗额度。 */
class SubmissionIdentity {
    private var previousBody: String? = null
    private var previousKey: String? = null

    fun keyFor(body: String): String {
        if (body != previousBody || previousKey == null) {
            previousBody = body
            previousKey = UUID.randomUUID().toString()
        }
        return requireNotNull(previousKey)
    }

    fun clear() {
        previousBody = null
        previousKey = null
    }
}
