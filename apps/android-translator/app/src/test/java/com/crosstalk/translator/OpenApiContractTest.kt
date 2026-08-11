package com.crosstalk.translator

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Guards required OpenAPI paths used by the translator without duplicating
 * server authorization logic. Spec path is repo-root api/openapi.json.
 */
class OpenApiContractTest {
    private val json = Json { ignoreUnknownKeys = true }

    @Test
    fun requiredTranslatorPathsExist() {
        val spec = locateOpenApiSpec()
        assertTrue("OpenAPI spec missing at ${spec.absolutePath}", spec.isFile)

        val root = json.parseToJsonElement(spec.readText()).jsonObject
        val paths = root["paths"]?.jsonObject
        assertNotNull("OpenAPI paths object missing", paths)
        val pathMap = requireNotNull(paths)

        val required = listOf(
            "/api/auth/login" to "post",
            "/api/auth/refresh" to "post",
            "/api/auth/logout" to "post",
            "/api/sessions" to "get",
            "/api/sessions/{id}" to "get",
            "/api/sessions/{id}/channels" to "get",
            "/api/webrtc/token" to "post",
        )

        val missing = mutableListOf<String>()
        for ((path, method) in required) {
            val item = pathMap[path]?.jsonObject
            if (item == null) {
                missing += "missing path $path"
                continue
            }
            if (item[method] == null) {
                missing += "missing $method on $path"
            }
        }

        assertTrue(
            "OpenAPI contract drift:\n" + missing.joinToString("\n"),
            missing.isEmpty(),
        )

        val tokenPost = pathMap["/api/webrtc/token"]?.jsonObject?.get("post")?.jsonObject
        assertNotNull(tokenPost)
        assertNotNull(tokenPost?.get("responses"))
    }

    private fun locateOpenApiSpec(): File {
        val env = System.getenv("CROSSTALK_OPENAPI_SPEC")
        if (!env.isNullOrBlank()) {
            val f = File(env)
            if (f.isFile) return f
        }

        val candidates = listOf(
            File("../../api/openapi.json"),
            File("../../../api/openapi.json"),
            File("api/openapi.json"),
            File("../api/openapi.json"),
        )
        candidates.firstOrNull { it.isFile }?.let { return it }

        var dir: File? = File(System.getProperty("user.dir") ?: ".")
        repeat(8) {
            val cur = dir ?: return@repeat
            val hit = File(cur, "api/openapi.json")
            if (hit.isFile) return hit
            dir = cur.parentFile
        }
        return candidates.first()
    }
}
