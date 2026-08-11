package com.crosstalk.translator.rtc

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test

class SignalingCodecTest {
    @Test
    fun encodeDecodeOfferWithSeq() {
        val msg =
            SignalingMessage(
                type = "offer",
                sdp = "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\n",
                seq = 42,
            )
        val raw = SignalingCodec.encode(msg)
        val decoded = SignalingCodec.decode(raw)
        assertEquals("offer", decoded.type)
        assertEquals(msg.sdp, decoded.sdp)
        assertEquals(42L, decoded.seq)
        assertNull(decoded.candidate)
    }

    @Test
    fun encodeDecodeAnswer() {
        val msg = SignalingMessage(type = "answer", sdp = "v=0\r\nanswer")
        val decoded = SignalingCodec.decode(SignalingCodec.encode(msg))
        assertEquals("answer", decoded.type)
        assertEquals("v=0\r\nanswer", decoded.sdp)
    }

    @Test
    fun encodeDecodeCandidate() {
        val cand =
            IceCandidateInit(
                candidate = "candidate:1 1 udp 2130706431 192.168.1.1 12345 typ host",
                sdpMid = "0",
                sdpMLineIndex = 0,
                usernameFragment = "frag",
            )
        val msg = SignalingMessage(type = "candidate", candidate = cand, seq = 7)
        val decoded = SignalingCodec.decode(SignalingCodec.encode(msg))
        assertEquals("candidate", decoded.type)
        assertNotNull(decoded.candidate)
        assertEquals(cand.candidate, decoded.candidate!!.candidate)
        assertEquals("0", decoded.candidate!!.sdpMid)
        assertEquals(0, decoded.candidate!!.sdpMLineIndex)
        assertEquals("frag", decoded.candidate!!.usernameFragment)
        assertEquals(7L, decoded.seq)
    }

    @Test
    fun acceptsIceAliasAsCandidate() {
        val raw =
            """
            {"type":"ice","candidate":{"candidate":"candidate:1 1 udp 1 10.0.0.1 9 typ host","sdpMid":"0","sdpMLineIndex":0}}
            """.trimIndent()
        val decoded = SignalingCodec.decode(raw)
        assertEquals("candidate", decoded.type)
        assertEquals("0", decoded.candidate!!.sdpMid)
        assertTrue(decoded.candidate!!.candidate!!.contains("typ host"))
    }

    @Test
    fun rejectsMissingType() {
        try {
            SignalingCodec.decode("""{"sdp":"v=0"}""")
            fail("expected exception")
        } catch (e: SignalingCodecException) {
            assertTrue(e.message!!.contains("type"))
        }
    }

    @Test
    fun rejectsUnknownType() {
        try {
            SignalingCodec.decode("""{"type":"bye"}""")
            fail("expected exception")
        } catch (e: SignalingCodecException) {
            assertTrue(e.message!!.contains("unknown"))
        }
    }

    @Test
    fun rejectsOfferWithoutSdp() {
        try {
            SignalingCodec.decode("""{"type":"offer"}""")
            fail("expected exception")
        } catch (e: SignalingCodecException) {
            assertTrue(e.message!!.contains("sdp"))
        }
    }

    @Test
    fun rejectsMalformedJson() {
        try {
            SignalingCodec.decode("not-json{")
            fail("expected exception")
        } catch (e: SignalingCodecException) {
            assertTrue(e.message!!.contains("JSON") || e.message!!.contains("invalid"))
        }
    }

    @Test
    fun rejectsOversizedMessage() {
        val huge = "x".repeat(SignalingCodec.MAX_MESSAGE_BYTES + 1)
        val raw = """{"type":"offer","sdp":"$huge"}"""
        try {
            SignalingCodec.decode(raw)
            fail("expected exception")
        } catch (e: SignalingCodecException) {
            assertTrue(e.message!!.contains("exceeds"))
        }
    }

    @Test
    fun diagnosticSummaryNeverContainsSdpOrCandidateBody() {
        val sdp = "v=0\r\no=- SECRET_SDP_BODY 0 IN IP4 203.0.113.9\r\n"
        val candLine = "candidate:1 1 udp 2130706431 198.51.100.44 54321 typ srflx"
        val offer = SignalingMessage(type = "offer", sdp = sdp, seq = 3)
        val cand =
            SignalingMessage(
                type = "candidate",
                candidate = IceCandidateInit(candidate = candLine, sdpMid = "0", sdpMLineIndex = 0),
            )
        val offerDiag = SignalingCodec.diagnosticSummary(offer)
        val candDiag = SignalingCodec.diagnosticSummary(cand)

        assertFalse(offerDiag.contains("SECRET_SDP_BODY"))
        assertFalse(offerDiag.contains(sdp))
        assertTrue(offerDiag.contains("type=offer"))
        assertTrue(offerDiag.contains("sdpBytes="))

        assertFalse(candDiag.contains("198.51.100.44"))
        assertFalse(candDiag.contains(candLine))
        assertTrue(candDiag.contains("type=candidate"))
        assertTrue(candDiag.contains("candType=srflx") || candDiag.contains("candidatePresent=true"))
    }

    @Test
    fun buildUrlUsesEncodedTicketAndRedacts() {
        val ticket = "ticket+with/special=chars&more"
        val url =
            SignalingClient.buildUrl(
                wsBaseUrl = "https://crosstalk.example",
                sessionId = "01SESSIONID",
                mediaTicket = ticket,
            )
        assertTrue(url.startsWith("wss://crosstalk.example/api/sessions/01SESSIONID/ws?token="))
        assertTrue(url.contains("token="))
        assertFalse("raw + must be encoded", url.contains("token=ticket+with"))
        val redacted = SignalingClient.redactUrl(url)
        assertFalse(redacted.contains(ticket))
        assertTrue(redacted.contains("token=***") || redacted.contains("***"))
    }

    @Test
    fun buildUrlAcceptsHttpAndWsSchemes() {
        assertTrue(
            SignalingClient.buildUrl("http://10.0.2.2:8080", "s1", "t1")
                .startsWith("ws://10.0.2.2:8080/"),
        )
        assertTrue(
            SignalingClient.buildUrl("wss://host/base", "s1", "t1")
                .startsWith("wss://host/base/"),
        )
    }
}
