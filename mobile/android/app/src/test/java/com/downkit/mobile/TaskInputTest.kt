package com.downkit.mobile

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class TaskInputTest {
    @Test
    fun extractsUrlFromSharedText() {
        assertEquals(
            "https://cdn.example/video/master.m3u8?token=abc",
            extractHttpUrl("视频地址：https://cdn.example/video/master.m3u8?token=abc。"),
        )
    }

    @Test
    fun acceptsDirectHttpUrls() {
        assertEquals("http://127.0.0.1:8080/video.mp4", normalizeHttpUrl(" http://127.0.0.1:8080/video.mp4 "))
    }

    @Test
    fun rejectsNonHttpAndMalformedValues() {
        assertNull(normalizeHttpUrl("ftp://example.com/video.mp4"))
        assertNull(normalizeHttpUrl("not a url"))
        assertNull(extractHttpUrl("这里没有链接"))
    }
}
