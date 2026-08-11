package com.crosstalk.translator

import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Fails if generated OpenAPI packages are imported outside contract/.
 * GeneratedApiAdapter (Phase 2) is the only handwritten consumer allowed.
 */
class GeneratedImportBoundaryTest {
    @Test
    fun generatedPackagesOnlyImportedFromContract() {
        val mainJava = locateMainJava()
        assertTrue("main source tree missing: ${mainJava.absolutePath}", mainJava.isDirectory)

        val forbidden = Regex(
            """import\s+com\.crosstalk\.translator\.generated(\.[\w.*]+)?""",
        )
        val offenders = mutableListOf<String>()

        mainJava.walkTopDown()
            .filter { it.isFile && it.extension == "kt" }
            .forEach { file ->
                val rel = file.relativeTo(mainJava).invariantSeparatorsPath
                val inContract = rel.startsWith("com/crosstalk/translator/contract/")
                if (inContract) return@forEach
                file.readLines().forEachIndexed { index, line ->
                    if (forbidden.containsMatchIn(line)) {
                        offenders += "$rel:${index + 1}: ${line.trim()}"
                    }
                }
            }

        assertTrue(
            "generated OpenAPI imports must stay inside contract/:\n" +
                offenders.joinToString("\n"),
            offenders.isEmpty(),
        )
    }

    private fun locateMainJava(): File {
        val candidates = listOf(
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
            val hit2 = File(cur, "src/main/java")
            if (hit2.isDirectory && File(hit2, "com/crosstalk/translator").isDirectory) return hit2
            dir = cur.parentFile
        }
        return File("src/main/java")
    }
}
