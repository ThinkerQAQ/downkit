package com.downkit.mobile

import android.media.MediaCodec
import android.media.MediaExtractor
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test
import java.io.File
import java.nio.file.Files

class AndroidMediaMuxerTest {
    @Test
    fun mapsExtractorFlagsToCodecFlags() {
        assertEquals(0, mediaCodecFlags(0))
        assertEquals(
            MediaCodec.BUFFER_FLAG_KEY_FRAME,
            mediaCodecFlags(MediaExtractor.SAMPLE_FLAG_SYNC),
        )
        assertEquals(
            MediaCodec.BUFFER_FLAG_PARTIAL_FRAME,
            mediaCodecFlags(MediaExtractor.SAMPLE_FLAG_PARTIAL_FRAME),
        )
        assertEquals(
            MediaCodec.BUFFER_FLAG_KEY_FRAME or MediaCodec.BUFFER_FLAG_PARTIAL_FRAME,
            mediaCodecFlags(MediaExtractor.SAMPLE_FLAG_SYNC or MediaExtractor.SAMPLE_FLAG_PARTIAL_FRAME),
        )
    }

    @Test
    fun rejectsEncryptedSamples() {
        assertThrows(IllegalArgumentException::class.java) {
            mediaCodecFlags(MediaExtractor.SAMPLE_FLAG_ENCRYPTED)
        }
    }

    @Test
    fun resolvesLocalHlsPartsInPlaybackOrder() {
        val root = Files.createTempDirectory("downkit-hls-test").toFile()
        try {
            val segments = File(root, "segments").apply { mkdirs() }
            val init = File(segments, "init000.mp4").apply { writeBytes(byteArrayOf(1)) }
            val first = File(segments, "seg000000.m4s").apply { writeBytes(byteArrayOf(2)) }
            val second = File(segments, "seg000001.m4s").apply { writeBytes(byteArrayOf(3)) }
            val playlist = File(root, "local.m3u8").apply {
                writeText(
                    """#EXTM3U
                    |#EXT-X-MAP:URI="segments/init000.mp4"
                    |#EXTINF:1,
                    |segments/seg000000.m4s
                    |#EXTINF:1,
                    |segments/seg000001.m4s
                    |""".trimMargin(),
                )
            }
            assertEquals(listOf(init, first, second).map(File::getCanonicalFile), localHlsPartFiles(playlist))
        } finally {
            root.deleteRecursively()
        }
    }

    @Test
    fun rejectsLocalHlsPathTraversal() {
        val root = Files.createTempDirectory("downkit-hls-test").toFile()
        try {
            val outside = File(root.parentFile, "outside-${root.name}.ts").apply { writeBytes(byteArrayOf(1)) }
            try {
                val playlist = File(root, "local.m3u8").apply { writeText("#EXTM3U\n../${outside.name}\n") }
                assertThrows(IllegalStateException::class.java) { localHlsPartFiles(playlist) }
            } finally {
                outside.delete()
            }
        } finally {
            root.deleteRecursively()
        }
    }
}
