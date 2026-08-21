package com.downkit.mobile

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class MobileLogTest {
    @Test
    fun redactsUrlsAndSecretsFromErrors() {
        val message = MobileLog.safeErrorMessage(
            IllegalStateException("GET https://example.com/video.mp4?token=secret failed; auth=private"),
        )

        assertFalse(message.contains("example.com"))
        assertFalse(message.contains("secret"))
        assertFalse(message.contains("private"))
        assertTrue(message.contains("<url>"))
        assertTrue(message.contains("<redacted>"))
    }
}
