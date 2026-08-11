package com.crosstalk.translator.rtc

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Ensures FakeRtcEngine never lands in the main source set (release APK).
 */
class FakeRtcBoundaryTest {
    @Test
    fun fakeRtcEngineOnlyInTestSources() {
        val mainJava = locateMainJava()
        assertTrue(mainJava.isDirectory)

        val offenders = mutableListOf<String>()
        mainJava.walkTopDown()
            .filter { it.isFile && it.extension == "kt" }
            .forEach { file ->
                val text = file.readText()
                if (text.contains("class FakeRtcEngine") || text.contains("FakeRtcEngine(")) {
                    // Allow mentions in comments that forbid it, but not a real class/import.
                    val hasClass = Regex("""\bclass\s+FakeRtcEngine\b""").containsMatchIn(text)
                    val hasImport =
                        Regex("""import\s+.*FakeRtcEngine\b""").containsMatchIn(text)
                    if (hasClass || hasImport) {
                        offenders += file.relativeTo(mainJava).invariantSeparatorsPath
                    }
                }
            }
        assertTrue(
            "FakeRtcEngine must not exist in main source set:\n${offenders.joinToString("\n")}",
            offenders.isEmpty(),
        )
    }

    @Test
    fun productionEngineIsFinalLibWebRtc() {
        val mainJava = locateMainJava()
        val engine =
            File(mainJava, "com/crosstalk/translator/rtc/LibWebRtcEngine.kt")
        assertTrue("LibWebRtcEngine.kt missing", engine.isFile)
        val text = engine.readText()
        assertTrue(text.contains("final class LibWebRtcEngine"))
        assertFalse(text.contains("class FakeRtcEngine"))
    }

    private fun locateMainJava(): File {
        val candidates =
            listOf(
                File("src/main/java"),
                File("app/src/main/java"),
                File("../app/src/main/java"),
            )
        candidates.firstOrNull { it.isDirectory }?.let { return it }
        var dir: File? = File(System.getProperty("user.dir") ?: ".")
        repeat(8) {
            val cur = dir ?: return@repeat
            val hit = File(cur, "app/src/main/java")
            if (hit.isDirectory) return hit
            dir = cur.parentFile
        }
        return File("src/main/java")
    }
}
