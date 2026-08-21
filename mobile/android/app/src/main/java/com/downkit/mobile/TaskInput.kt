package com.downkit.mobile

import java.net.URI

private val httpUrlPattern = Regex("""https?://[^\s<>"']+""", RegexOption.IGNORE_CASE)
private val trailingSharePunctuation = setOf('.', ',', '，', '。', ')', ']', '}', '>', '；', ';')

internal fun normalizeHttpUrl(raw: String): String? {
    val value = raw.trim()
    if (value.isEmpty()) return null
    val parsed = runCatching { URI(value) }.getOrNull() ?: return null
    if (parsed.scheme?.lowercase() !in setOf("http", "https") || parsed.host.isNullOrBlank()) return null
    return value
}

internal fun extractHttpUrl(text: String): String? {
    val candidate = httpUrlPattern.find(text)?.value
        ?.trimEnd { it in trailingSharePunctuation }
        ?: return null
    return normalizeHttpUrl(candidate)
}
