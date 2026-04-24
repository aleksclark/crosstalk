# Instructions

- Following Playwright test failed.
- Explain why, be concise, respect Playwright best practices.
- Provide a snippet of code with the fix, if possible.

# Test info

- Name: golden-audio.spec.ts >> Golden Audio Tests >> K2B→Browser: capture received WebRTC audio
- Location: test/playwright/specs/golden-audio.spec.ts:83:7

# Error details

```
Error: expect(received).toBeGreaterThan(expected)

Expected: > 1000
Received:   110
```

# Page snapshot

```yaml
- generic [ref=e3]:
  - banner [ref=e4]:
    - generic [ref=e5]:
      - link "CrossTalk" [ref=e6] [cursor=pointer]:
        - /url: /dashboard
      - navigation [ref=e7]:
        - link "Dashboard" [ref=e8] [cursor=pointer]:
          - /url: /dashboard
        - link "Templates" [ref=e9] [cursor=pointer]:
          - /url: /templates
        - link "Sessions" [ref=e10] [cursor=pointer]:
          - /url: /sessions
        - link "Clients" [ref=e11] [cursor=pointer]:
          - /url: /clients
        - link "Settings" [ref=e12] [cursor=pointer]:
          - /url: /settings
      - generic [ref=e13]:
        - generic [ref=e14]: admin
        - button "Logout" [ref=e15]
  - main [ref=e16]:
    - generic [ref=e17]:
      - generic [ref=e18]:
        - generic [ref=e19]:
          - generic [ref=e20]: "Session: e2e-session"
          - generic [ref=e21]: "Role: translator"
          - generic [ref=e22]: "ICE: connected"
        - button "End Session" [ref=e23]
      - generic [ref=e24]:
        - generic [ref=e25]:
          - heading "Audio Channels" [level=3] [ref=e27]
          - generic [ref=e28]:
            - generic [ref=e29]:
              - generic [ref=e30]: Incoming Channels
              - generic [ref=e34]:
                - generic [ref=e35]: output
                - slider [ref=e36]: "1"
            - generic [ref=e38]:
              - generic [ref=e39]: Microphone (Outgoing)
              - generic [ref=e40]:
                - combobox [ref=e41]:
                  - option "Fake Default Audio Input" [selected]
                  - option "Fake Audio Input 1"
                  - option "Fake Audio Input 2"
                - button "Mute" [ref=e42]
              - generic [ref=e46]: "-40.4 dBFS"
            - generic [ref=e47]:
              - generic [ref=e48]: Volume Controls
              - generic [ref=e49]:
                - generic [ref=e50]: output
                - slider [ref=e51]: "1"
        - generic [ref=e52]:
          - heading "WebRTC Debug" [level=3] [ref=e54]
          - generic [ref=e56]:
            - generic [ref=e57]:
              - generic [ref=e58]: ICE State
              - generic [ref=e59]: connected
            - generic [ref=e60]:
              - generic [ref=e61]: ICE Candidates
              - generic [ref=e62]: 8 local, 35 remote
            - generic [ref=e63]:
              - generic [ref=e64]: Bytes Sent
              - generic [ref=e65]: 54.7 KB
            - generic [ref=e66]:
              - generic [ref=e67]: Bytes Received
              - generic [ref=e68]: 22.6 KB
            - generic [ref=e69]:
              - generic [ref=e70]: Packet Loss
              - generic [ref=e71]: 0.0%
            - generic [ref=e72]:
              - generic [ref=e73]: Jitter
              - generic [ref=e74]: 21ms
            - generic [ref=e75]:
              - generic [ref=e76]: RTT
              - generic [ref=e77]: 0ms
      - generic [ref=e78]:
        - generic [ref=e80]:
          - heading "Session Participants" [level=3] [ref=e81]
          - generic [ref=e82]: 2 connected
        - generic [ref=e84]:
          - generic [ref=e85]:
            - generic [ref=e86]:
              - generic [ref=e87]:
                - generic [ref=e88]: Browser
                - generic [ref=e89]: studio
              - generic [ref=e90]: 01KPZK24H9EX...
            - generic [ref=e91]:
              - generic [ref=e92]: Devices
              - generic [ref=e93]: "Sources: alsa_input.platform-snd_aloop.0.analog-stereo"
              - generic [ref=e94]: "Sinks: alsa_output.platform-snd_aloop.0.analog-stereo"
            - generic [ref=e96]: opus/48000/2
          - generic [ref=e98]:
            - generic [ref=e99]:
              - generic [ref=e100]: Browser
              - generic [ref=e101]: translator
            - generic [ref=e102]: 01KPZK2WV7PF...
      - generic [ref=e103]:
        - generic [ref=e105]:
          - heading "Session Logs" [level=3] [ref=e106]
          - combobox [ref=e108]:
            - option "All" [selected]
            - option "Debug"
            - option "Info"
            - option "Warn"
            - option "Error"
        - generic [ref=e110]:
          - generic [ref=e111]:
            - text: 6:13:02 AM
            - generic [ref=e112]: "[system]"
            - text: Initiating WebRTC connection...
          - generic [ref=e113]:
            - text: 6:13:02 AM
            - generic [ref=e114]: "[signaling]"
            - text: "Connecting WS: ws://127.0.0.1:34887/ws/signaling?token=ct_57a274b4cacb6f8a7..."
          - generic [ref=e115]:
            - text: 6:13:02 AM
            - generic [ref=e116]: "[system]"
            - text: WebSocket connected, creating offer...
          - generic [ref=e117]:
            - text: 6:13:02 AM
            - generic [ref=e118]: "[webrtc]"
            - text: "Signaling state: have-local-offer"
          - generic [ref=e119]:
            - text: 6:13:02 AM
            - generic [ref=e120]: "[signaling]"
            - text: SDP offer sent
          - generic [ref=e121]:
            - text: 6:13:02 AM
            - generic [ref=e122]: "[signaling]"
            - text: "WS recv: {\"type\":\"answer\",\"sdp\":\"v=0\\r\\no=- 2927172693564050093 1777029182 IN IP4 0.0.0.0"
          - generic [ref=e123]:
            - text: 6:13:02 AM
            - generic [ref=e124]: "[signaling]"
            - text: "WS recv: {\"type\":\"ice\",\"candidate\":{\"candidate\":\"candidate:2887923442 1 udp 2130706431 19"
          - generic [ref=e125]:
            - text: 6:13:02 AM
            - generic [ref=e126]: "[signaling]"
            - text: "WS recv: {\"type\":\"ice\",\"candidate\":{\"candidate\":\"candidate:2929902828 1 udp 2130706431 fd"
          - generic [ref=e127]:
            - text: 6:13:02 AM
            - generic [ref=e128]: "[webrtc]"
            - text: "ICE state: checking"
          - generic [ref=e129]:
            - text: 6:13:02 AM
            - generic [ref=e130]: "[webrtc]"
            - text: "Signaling state: stable"
          - generic [ref=e131]:
            - text: 6:13:02 AM
            - generic [ref=e132]: "[webrtc]"
            - text: "Received remote track: kind=audio id=b285a3fd-cec1-4454-8aa7-599a69e1c5e4 streams=1 readyState=live"
          - generic [ref=e133]:
            - text: 6:13:02 AM
            - generic [ref=e134]: "[audio]"
            - text: "AudioContext created (state: running)"
          - generic [ref=e135]:
            - text: 6:13:02 AM
            - generic [ref=e136]: "[webrtc]"
            - text: Audio track wired to speakers (ctx.state=running, sampleRate=48000)
          - generic [ref=e137]:
            - text: 6:13:02 AM
            - generic [ref=e138]: "[webrtc]"
            - text: Set remote description (answer)
          - generic [ref=e139]:
            - text: 6:13:02 AM
            - generic [ref=e140]: "[signaling]"
            - text: "WS recv: {\"type\":\"ice\",\"candidate\":{\"candidate\":\"candidate:2030765712 1 udp 2130706431 19"
          - generic [ref=e141]:
            - text: 6:13:02 AM
            - generic [ref=e142]: "[signaling]"
            - text: "WS recv: {\"type\":\"ice\",\"candidate\":{\"candidate\":\"candidate:3749448996 1 udp 2130706431 19"
          - generic [ref=e143]:
            - text: 6:13:02 AM
            - generic [ref=e144]: "[signaling]"
            - text: "WS recv: {\"type\":\"ice\",\"candidate\":{\"candidate\":\"candidate:3585672253 1 udp 2130706431 19"
          - generic [ref=e145]:
            - text: 6:13:02 AM
            - generic [ref=e146]: "[signaling]"
            - text: "WS recv: {\"type\":\"ice\",\"candidate\":{\"candidate\":\"candidate:2830600246 1 udp 2130706431 fc"
          - generic [ref=e147]:
            - text: 6:13:02 AM
            - generic [ref=e148]: "[signaling]"
            - text: "WS recv: {\"type\":\"ice\",\"candidate\":{\"candidate\":\"candidate:1836732591 1 udp 2130706431 17"
          - generic [ref=e149]:
            - text: 6:13:02 AM
            - generic [ref=e150]: "[webrtc]"
            - text: "ICE state: connected"
          - generic [ref=e151]:
            - text: 6:13:02 AM
            - generic [ref=e152]: "[webrtc]"
            - text: "Connection state: connecting"
          - generic [ref=e153]:
            - text: 6:13:02 AM
            - generic [ref=e154]: "[signaling]"
            - text: "WS recv: {\"type\":\"ice\",\"candidate\":{\"candidate\":\"candidate:540623837 1 udp 2130706431 192"
          - generic [ref=e155]:
            - text: 6:13:02 AM
            - generic [ref=e156]: "[signaling]"
            - text: "WS recv: {\"type\":\"ice\",\"candidate\":{\"candidate\":\"candidate:2253277289 1 udp 2130706431 19"
          - generic [ref=e157]:
            - text: 6:13:02 AM
            - generic [ref=e158]: "[signaling]"
            - text: "WS recv: {\"type\":\"ice\",\"candidate\":{\"candidate\":\"candidate:1222843405 1 udp 2130706431 19"
          - generic [ref=e159]:
            - text: 6:13:02 AM
            - generic [ref=e160]: "[signaling]"
            - text: "WS recv: {\"type\":\"ice\",\"candidate\":{\"candidate\":\"candidate:2692523754 1 udp 2130706431 10"
          - generic [ref=e161]:
            - text: 6:13:02 AM
            - generic [ref=e162]: "[signaling]"
            - text: "WS recv: {\"type\":\"ice\",\"candidate\":{\"candidate\":\"candidate:3029389644 1 udp 2130706431 fd"
          - generic [ref=e163]:
            - text: 6:13:02 AM
            - generic [ref=e164]: "[signaling]"
            - text: "WS recv: {\"type\":\"ice\",\"candidate\":{\"candidate\":\"candidate:2370052739 1 udp 2130706431 19"
          - generic [ref=e165]:
            - text: 6:13:02 AM
            - generic [ref=e166]: "[signaling]"
            - text: "WS recv: {\"type\":\"ice\",\"candidate\":{\"candidate\":\"candidate:4002682809 1 udp 2130706431 19"
          - generic [ref=e167]:
            - text: 6:13:02 AM
            - generic [ref=e168]: "[signaling]"
            - text: "WS recv: {\"type\":\"ice\",\"candidate\":{\"candidate\":\"candidate:724773175 1 udp 2130706431 192"
          - generic [ref=e169]:
            - text: 6:13:02 AM
            - generic [ref=e170]: "[webrtc]"
            - text: "Connection state: connected"
          - generic [ref=e171]:
            - text: 6:13:02 AM
            - generic [ref=e172]: "[webrtc]"
            - text: "Data channel received: control (readyState=open)"
          - generic [ref=e173]:
            - text: 6:13:02 AM
            - generic [ref=e174]: "[system]"
            - text: Sent Hello
          - generic [ref=e175]:
            - text: 6:13:02 AM
            - generic [ref=e176]: "[server]"
            - text: "Welcome: client=01KPZK2WV7PFBE5N0BM1H1SF9R server=0.1.0"
          - generic [ref=e177]:
            - text: 6:13:02 AM
            - generic [ref=e178]: "[session]"
            - text: "undefined: joined as translator"
          - generic [ref=e179]:
            - text: 6:13:02 AM
            - generic [ref=e180]: "[system]"
            - text: "Bind channel: output (SINK) trackId=01KPZK2WWEZ91S9K9KZMZ5MCKJ"
          - generic [ref=e181]:
            - text: 6:13:02 AM
            - generic [ref=e182]: "[system]"
            - text: "Bind channel: mic (undefined) trackId=01KPZK2WWEZ91S9K9KZQM3PN01"
          - generic [ref=e183]:
            - text: 6:13:02 AM
            - generic [ref=e184]: "[signaling]"
            - text: "WS recv: {\"type\":\"ice\",\"candidate\":{\"candidate\":\"candidate:1490732268 1 udp 1694498815 10"
          - generic [ref=e185]:
            - text: 6:13:02 AM
            - generic [ref=e186]: "[audio]"
            - text: "Mic track added: Fake Default Audio Input"
          - generic [ref=e187]:
            - text: 6:13:02 AM
            - generic [ref=e188]: "[webrtc]"
            - text: "Signaling state: have-local-offer"
          - generic [ref=e189]:
            - text: 6:13:02 AM
            - generic [ref=e190]: "[audio]"
            - text: Renegotiation offer sent after mic add
          - generic [ref=e191]:
            - text: 6:13:02 AM
            - generic [ref=e192]: "[signaling]"
            - text: "WS recv: {\"type\":\"answer\",\"sdp\":\"v=0\\r\\no=- 2927172693564050093 1777029183 IN IP4 0.0.0.0"
          - generic [ref=e193]:
            - text: 6:13:02 AM
            - generic [ref=e194]: "[webrtc]"
            - text: "Signaling state: stable"
          - generic [ref=e195]:
            - text: 6:13:02 AM
            - generic [ref=e196]: "[webrtc]"
            - text: "Received remote track: kind=audio id=b285a3fd-cec1-4454-8aa7-599a69e1c5e4 streams=1 readyState=live"
          - generic [ref=e197]:
            - text: 6:13:02 AM
            - generic [ref=e198]: "[webrtc]"
            - text: Audio track wired to speakers (ctx.state=running, sampleRate=48000)
          - generic [ref=e199]:
            - text: 6:13:02 AM
            - generic [ref=e200]: "[webrtc]"
            - text: Set remote description (answer)
          - generic [ref=e201]:
            - text: 6:13:02 AM
            - generic [ref=e202]: "[signaling]"
            - text: "WS recv: {\"type\":\"offer\",\"sdp\":\"v=0\\r\\no=- 2927172693564050093 1777029184 IN IP4 0.0.0.0\\"
          - generic [ref=e203]:
            - text: 6:13:02 AM
            - generic [ref=e204]: "[signaling]"
            - text: Received renegotiation offer (state=stable)
          - generic [ref=e205]:
            - text: 6:13:02 AM
            - generic [ref=e206]: "[webrtc]"
            - text: "Signaling state: have-remote-offer"
          - generic [ref=e207]:
            - text: 6:13:02 AM
            - generic [ref=e208]: "[webrtc]"
            - text: "Signaling state: stable"
          - generic [ref=e209]:
            - text: 6:13:02 AM
            - generic [ref=e210]: "[webrtc]"
            - text: Handled renegotiation offer — answer sent
          - generic [ref=e211]:
            - text: 6:13:03 AM
            - generic [ref=e212]: "[webrtc]"
            - text: "Track unmuted: b285a3fd-cec1-4454-8aa7-599a69e1c5e4"
```

# Test source

```ts
  105 |     );
  106 | 
  107 |     // Wait for the remote stream to appear. Poll with logging.
  108 |     let hasStream = false;
  109 |     for (let i = 0; i < 30; i++) {
  110 |       hasStream = await page.evaluate(() => !!(window as any).__remoteStream);
  111 |       if (hasStream) break;
  112 |       if (i % 5 === 0) {
  113 |         const logs = await page.evaluate(() => {
  114 |           const logEl = document.querySelector('[data-testid="session-logs"]');
  115 |           const text = logEl ? logEl.textContent : "no-log-el";
  116 |           return text ? text.substring(text.length - 800) : "empty";
  117 |         });
  118 |         console.log(`[DEBUG] Poll ${i}: __remoteStream=${hasStream}, TAIL logs=${logs?.substring(0, 800)}`);
  119 |       }
  120 |       await page.waitForTimeout(1000);
  121 |     }
  122 |     if (!hasStream) {
  123 |       throw new Error("__remoteStream never set after 30s");
  124 |     }
  125 | 
  126 |     // Capture received audio using Web Audio API + MediaRecorder.
  127 |     // Inject a script that hooks into the received audio track and records it.
  128 |     const audioBase64 = await page.evaluate(
  129 |       async (durationMs: number) => {
  130 |         return new Promise<string>((resolve, reject) => {
  131 |           try {
  132 |             // Find the audio element or remote stream
  133 |             const audioElements = document.querySelectorAll("audio, video");
  134 |             let stream: MediaStream | null = null;
  135 | 
  136 |             // Try to get the stream from an audio/video element
  137 |             for (const el of audioElements) {
  138 |               const mediaEl = el as HTMLMediaElement;
  139 |               if (mediaEl.srcObject instanceof MediaStream) {
  140 |                 stream = mediaEl.srcObject;
  141 |                 break;
  142 |               }
  143 |             }
  144 | 
  145 |             // Fallback: look for exposed stream on window
  146 |             if (!stream && (window as any).__remoteStream) {
  147 |               stream = (window as any).__remoteStream;
  148 |             }
  149 | 
  150 |             if (!stream) {
  151 |               reject(new Error("No remote audio stream found"));
  152 |               return;
  153 |             }
  154 | 
  155 |             // Use MediaRecorder to capture the audio
  156 |             const audioTracks = stream.getAudioTracks();
  157 |             if (audioTracks.length === 0) {
  158 |               reject(new Error("No audio tracks in remote stream"));
  159 |               return;
  160 |             }
  161 | 
  162 |             const audioStream = new MediaStream(audioTracks);
  163 |             const recorder = new MediaRecorder(audioStream, {
  164 |               mimeType: "audio/webm;codecs=opus",
  165 |             });
  166 | 
  167 |             const chunks: Blob[] = [];
  168 |             recorder.ondataavailable = (e) => {
  169 |               if (e.data.size > 0) chunks.push(e.data);
  170 |             };
  171 | 
  172 |             recorder.onstop = async () => {
  173 |               const blob = new Blob(chunks, { type: "audio/webm" });
  174 |               const buffer = await blob.arrayBuffer();
  175 |               const bytes = new Uint8Array(buffer);
  176 |               let binary = "";
  177 |               for (let i = 0; i < bytes.length; i++) {
  178 |                 binary += String.fromCharCode(bytes[i]);
  179 |               }
  180 |               resolve(btoa(binary));
  181 |             };
  182 | 
  183 |             recorder.start();
  184 | 
  185 |             // Record for the specified duration
  186 |             setTimeout(() => {
  187 |               recorder.stop();
  188 |             }, durationMs);
  189 |           } catch (err) {
  190 |             reject(err);
  191 |           }
  192 |         });
  193 |       },
  194 |       AUDIO_DURATION,
  195 |     );
  196 | 
  197 |     // Write captured audio to file
  198 |     const audioBuffer = Buffer.from(audioBase64, "base64");
  199 |     const captureDir = path.dirname(CAPTURE_PATH);
  200 |     if (!fs.existsSync(captureDir)) {
  201 |       fs.mkdirSync(captureDir, { recursive: true });
  202 |     }
  203 |     fs.writeFileSync(CAPTURE_PATH, audioBuffer);
  204 | 
> 205 |     expect(audioBuffer.length).toBeGreaterThan(1000);
      |                                ^ Error: expect(received).toBeGreaterThan(expected)
  206 | 
  207 |     await context.close();
  208 |   });
  209 | });
  210 | 
```