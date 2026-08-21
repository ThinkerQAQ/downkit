package com.downkit.mobile

import android.media.MediaCodec
import android.media.MediaDataSource
import android.media.MediaExtractor
import android.media.MediaFormat
import android.media.MediaMuxer
import android.os.SystemClock
import downkit.PlatformMuxer
import java.io.File
import java.io.RandomAccessFile
import java.nio.ByteBuffer

internal fun mediaCodecFlags(extractorFlags: Int): Int {
    if (extractorFlags and MediaExtractor.SAMPLE_FLAG_ENCRYPTED != 0) {
        throw IllegalArgumentException("系统封装器不支持加密媒体样本")
    }
    var codecFlags = 0
    if (extractorFlags and MediaExtractor.SAMPLE_FLAG_SYNC != 0) {
        codecFlags = codecFlags or MediaCodec.BUFFER_FLAG_KEY_FRAME
    }
    if (extractorFlags and MediaExtractor.SAMPLE_FLAG_PARTIAL_FRAME != 0) {
        codecFlags = codecFlags or MediaCodec.BUFFER_FLAG_PARTIAL_FRAME
    }
    return codecFlags
}

private val hlsMapUriPattern = Regex("""(?i)^#EXT-X-MAP:.*URI="([^"]+)"""")

internal fun localHlsPartFiles(playlist: File): List<File> {
    val root = (playlist.parentFile ?: error("本地 HLS 缺少父目录")).canonicalFile
    val rootPrefix = root.path + File.separator
    fun resolve(raw: String): File {
        if (raw.contains("://") || File(raw).isAbsolute) error("本地 HLS 包含非相对路径")
        val resolved = File(root, raw.replace('/', File.separatorChar)).canonicalFile
        if (!resolved.path.startsWith(rootPrefix) || !resolved.isFile) {
            error("本地 HLS 分片不存在或越出工作目录")
        }
        return resolved
    }

    val parts = mutableListOf<File>()
    playlist.forEachLine { rawLine ->
        val line = rawLine.trim()
        val mapUri = hlsMapUriPattern.find(line)?.groupValues?.get(1)
        when {
            mapUri != null -> parts += resolve(mapUri)
            line.isNotEmpty() && !line.startsWith('#') -> parts += resolve(line)
        }
    }
    if (parts.isEmpty()) error("本地 HLS 没有可读取的媒体分片")
    return parts
}

private class ConcatenatedMediaDataSource(files: List<File>) : MediaDataSource() {
    private data class Part(val file: File, val start: Long, val size: Long)

    private val parts: List<Part>
    private val totalSize: Long
    private var openIndex = -1
    private var openFile: RandomAccessFile? = null

    init {
        var offset = 0L
        parts = files.map { file ->
            val part = Part(file, offset, file.length())
            offset += part.size
            part
        }
        totalSize = offset
    }

    override fun getSize(): Long = totalSize

    @Synchronized
    override fun readAt(position: Long, buffer: ByteArray, offset: Int, size: Int): Int {
        if (size == 0) return 0
        if (position < 0 || position >= totalSize) return -1
        var sourcePosition = position
        var destinationOffset = offset
        var remaining = minOf(size.toLong(), totalSize - position).toInt()
        var written = 0
        while (remaining > 0) {
            val index = parts.indexOfLast { it.start <= sourcePosition }
            if (index < 0) break
            val part = parts[index]
            val partOffset = sourcePosition - part.start
            val chunk = minOf(remaining.toLong(), part.size - partOffset).toInt()
            if (chunk <= 0) break
            if (openIndex != index) {
                openFile?.close()
                openFile = RandomAccessFile(part.file, "r")
                openIndex = index
            }
            openFile!!.seek(partOffset)
            val count = openFile!!.read(buffer, destinationOffset, chunk)
            if (count <= 0) break
            sourcePosition += count
            destinationOffset += count
            remaining -= count
            written += count
        }
        return if (written > 0) written else -1
    }

    @Synchronized
    override fun close() {
        openFile?.close()
        openFile = null
        openIndex = -1
    }
}

class AndroidMediaMuxer : PlatformMuxer {
    override fun mux(requestId: String, workDir: String, videoInput: String, audioInput: String, output: String) {
        val operationStartedAt = SystemClock.elapsedRealtime()
        MobileLog.info(
            requestId,
            "media-muxer",
            "mux",
            "start",
            null,
            "separateAudio" to audioInput.isNotBlank(),
        )
        val finalFile = File(output)
        val partial = File(finalFile.parentFile, "${finalFile.nameWithoutExtension}.partial.mp4")
        if (partial.exists()) partial.delete()

        val inputs = mutableListOf<Input>()
        try {
            val separateAudio = audioInput.isNotBlank()
            inputs += open(requestId, File(workDir, videoInput), includeVideo = true, includeAudio = !separateAudio)
            if (separateAudio) {
                inputs += open(requestId, File(workDir, audioInput), includeVideo = false, includeAudio = true)
            }
            if (inputs.none { it.selectedTracks.isNotEmpty() }) error("没有可封装的音视频轨")

            val muxer = MediaMuxer(partial.absolutePath, MediaMuxer.OutputFormat.MUXER_OUTPUT_MPEG_4)
            try {
                for (input in inputs) {
                    for (track in input.selectedTracks) {
                        track.outputIndex = muxer.addTrack(input.extractor.getTrackFormat(track.inputIndex))
                    }
                }
                muxer.start()
                for (input in inputs) {
                    val copied = copySamples(input, muxer)
                    MobileLog.info(
                        requestId,
                        "media-muxer",
                        "copy-samples",
                        "succeeded",
                        null,
                        "samples" to copied.first,
                        "bytes" to copied.second,
                    )
                }
                muxer.stop()
            } finally {
                muxer.release()
            }
            if (finalFile.exists() && !finalFile.delete()) error("无法替换已有输出文件")
            if (!partial.renameTo(finalFile)) error("无法完成输出文件重命名")
            MobileLog.info(
                requestId,
                "media-muxer",
                "mux",
                "succeeded",
                SystemClock.elapsedRealtime() - operationStartedAt,
                "bytes" to finalFile.length(),
            )
        } catch (error: Throwable) {
            MobileLog.error(
                requestId,
                "media-muxer",
                "mux",
                "failed",
                error,
                SystemClock.elapsedRealtime() - operationStartedAt,
            )
            throw error
        } finally {
            inputs.forEach { it.close() }
            if (partial.exists() && !finalFile.exists()) partial.delete()
        }
    }

    private fun open(requestId: String, file: File, includeVideo: Boolean, includeAudio: Boolean): Input {
        val extractor = MediaExtractor()
        var dataSource: MediaDataSource? = null
        try {
            if (file.extension.equals("m3u8", ignoreCase = true)) {
                val parts = localHlsPartFiles(file)
                dataSource = ConcatenatedMediaDataSource(parts)
                extractor.setDataSource(dataSource)
                MobileLog.info(
                    requestId,
                    "media-muxer",
                    "open-input",
                    "ready",
                    null,
                    "mode" to "concatenated-local-hls",
                    "parts" to parts.size,
                    "bytes" to dataSource.size,
                )
            } else {
                extractor.setDataSource(file.absolutePath)
            }
            val tracks = mutableListOf<Track>()
            for (index in 0 until extractor.trackCount) {
                val mime = extractor.getTrackFormat(index).getString(MediaFormat.KEY_MIME).orEmpty()
                if ((includeVideo && mime.startsWith("video/")) || (includeAudio && mime.startsWith("audio/"))) {
                    extractor.selectTrack(index)
                    tracks += Track(index)
                }
            }
            return Input(extractor, dataSource, tracks)
        } catch (error: Throwable) {
            extractor.release()
            dataSource?.close()
            throw error
        }
    }

    private fun copySamples(input: Input, muxer: MediaMuxer): Pair<Long, Long> {
        val outputByInput = input.selectedTracks.associate { it.inputIndex to it.outputIndex }
        val buffer = ByteBuffer.allocateDirect(16 * 1024 * 1024)
        val info = MediaCodec.BufferInfo()
        var sampleCount = 0L
        var copiedBytes = 0L
        while (true) {
            buffer.clear()
            val size = input.extractor.readSampleData(buffer, 0)
            if (size < 0) break
            val outputTrack = outputByInput[input.extractor.sampleTrackIndex]
            if (outputTrack != null) {
                info.set(0, size, input.extractor.sampleTime, mediaCodecFlags(input.extractor.sampleFlags))
                muxer.writeSampleData(outputTrack, buffer, info)
                sampleCount++
                copiedBytes += size
            }
            input.extractor.advance()
        }
        return sampleCount to copiedBytes
    }

    private data class Input(
        val extractor: MediaExtractor,
        val dataSource: MediaDataSource?,
        val selectedTracks: MutableList<Track>,
    ) {
        fun close() {
            extractor.release()
            dataSource?.close()
        }
    }
    private data class Track(val inputIndex: Int, var outputIndex: Int = -1)
}
