package com.buleng.luxuryvideo

import com.buleng.luxuryvideo.data.SubmissionIdentity
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Test

class SubmissionIdentityTest {
    @Test
    fun uncertainNetworkRetryKeepsOriginalKey() {
        val identity = SubmissionIdentity()
        val first = identity.keyFor("same request")
        assertEquals(first, identity.keyFor("same request"))
    }

    @Test
    fun changingPayloadCreatesAnotherKey() {
        val identity = SubmissionIdentity()
        val first = identity.keyFor("action=info")
        assertNotEquals(first, identity.keyFor("action=full"))
    }

    @Test
    fun completionOrAccountChangeEndsOriginalSubmission() {
        val identity = SubmissionIdentity()
        val first = identity.keyFor("same request")
        identity.clear()
        assertNotEquals(first, identity.keyFor("same request"))
    }
}
