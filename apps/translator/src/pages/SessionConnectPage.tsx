import { useState, useEffect, useRef, useCallback } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { useAuth } from "../hooks/useAuth";
import { useWebRTC } from "../hooks/useWebRTC";
import { WebRTCDebugPanel } from "../components/WebRTCDebugPanel";
import { VUMeter } from "../components/VUMeter";

export function SessionConnectPage() {
  const { id: sessionId } = useParams<{ id: string }>();
  const { getToken } = useAuth();
  const navigate = useNavigate();

  const [audioDevices, setAudioDevices] = useState<MediaDeviceInfo[]>([]);
  const [selectedDevice, setSelectedDevice] = useState<string>("");
  const [isConnected, setIsConnected] = useState(false);

  const token = getToken();

  // Optional SFU routing overrides via ?produce=&listen= (used for deep-linked
  // producer/listener roles and by the e2e suite). Absent → server routes by role.
  const search = new URLSearchParams(window.location.search);
  const produce = search.get("produce") ?? undefined;
  const listen = search.get("listen") ?? undefined;

  const webrtc = useWebRTC({
    sessionId: sessionId ?? "",
    token: token ?? "",
    audioDeviceId: selectedDevice || undefined,
    produce,
    listen,
  });

  // Enumerate audio devices
  useEffect(() => {
    const enumerate = async () => {
      try {
        // Need permission first to get labels
        const tempStream = await navigator.mediaDevices.getUserMedia({ audio: true });
        tempStream.getTracks().forEach((t) => t.stop());

        const devices = await navigator.mediaDevices.enumerateDevices();
        const audioInputs = devices.filter((d) => d.kind === "audioinput");
        setAudioDevices(audioInputs);
        if (audioInputs.length > 0 && !selectedDevice) {
          setSelectedDevice(audioInputs[0]!.deviceId);
        }
      } catch {
        // Can't enumerate without permission
      }
    };
    enumerate();
  }, [selectedDevice]);

  // Track connection state
  useEffect(() => {
    setIsConnected(webrtc.connectionState === "connected");
  }, [webrtc.connectionState]);

  const handleConnect = async () => {
    await webrtc.connect();
  };

  const handleDisconnect = () => {
    webrtc.disconnect();
  };

  // Audio level meters
  const inputLevel = useAudioLevel(webrtc.localStream);
  const outputLevel = useAudioLevel(webrtc.remoteStream);

  return (
    <div className="min-h-screen p-4 max-w-2xl mx-auto">
      <div className="flex items-center justify-between mb-6">
        <button
          onClick={() => navigate("/")}
          className="text-sm text-gray-400 hover:text-white transition-colors"
        >
          ← Back to Sessions
        </button>
        <div className="text-sm text-gray-400">
          Session: <span className="text-white">{sessionId?.slice(0, 8)}...</span>
        </div>
      </div>

      {/* Connection Controls */}
      <div className="bg-gray-800 border border-gray-700 rounded-lg p-4 space-y-4 mb-4">
        <h2 className="text-lg font-semibold text-white">Audio Connection</h2>

        {/* Mic selector */}
        <div>
          <label className="block text-sm text-gray-400 mb-1">Microphone</label>
          <select
            value={selectedDevice}
            onChange={(e) => setSelectedDevice(e.target.value)}
            disabled={isConnected}
            className="w-full px-3 py-2 bg-gray-900 border border-gray-600 rounded text-white disabled:opacity-50"
          >
            {audioDevices.map((device) => (
              <option key={device.deviceId} value={device.deviceId}>
                {device.label || `Microphone ${device.deviceId.slice(0, 8)}`}
              </option>
            ))}
            {audioDevices.length === 0 && (
              <option value="">No microphones found</option>
            )}
          </select>
        </div>

        {/* Connect / Disconnect */}
        <div className="flex gap-3">
          {!isConnected ? (
            <button
              onClick={handleConnect}
              disabled={webrtc.connectionState === "connecting"}
              className="flex-1 py-2 px-4 bg-green-600 hover:bg-green-700 disabled:bg-green-800 disabled:opacity-50 text-white font-medium rounded transition-colors"
            >
              {webrtc.connectionState === "connecting" ? "Connecting..." : "Connect"}
            </button>
          ) : (
            <button
              onClick={handleDisconnect}
              className="flex-1 py-2 px-4 bg-red-600 hover:bg-red-700 text-white font-medium rounded transition-colors"
            >
              Disconnect
            </button>
          )}
          <button
            onClick={webrtc.toggleMute}
            disabled={!isConnected}
            className={`px-4 py-2 rounded font-medium transition-colors ${
              webrtc.isMuted
                ? "bg-yellow-600 hover:bg-yellow-700 text-white"
                : "bg-gray-700 hover:bg-gray-600 text-gray-200"
            } disabled:opacity-50`}
          >
            {webrtc.isMuted ? "🔇 Muted" : "🎙️ Live"}
          </button>
        </div>

        {/* Connection status */}
        <div className="flex items-center gap-2 text-sm">
          <div
            className={`w-2 h-2 rounded-full ${
              isConnected
                ? "bg-green-500"
                : webrtc.connectionState === "connecting"
                  ? "bg-yellow-500 animate-pulse"
                  : webrtc.connectionState === "failed"
                    ? "bg-red-500"
                    : "bg-gray-500"
            }`}
          />
          <span className="text-gray-400">
            Status: <span className="text-white capitalize">{webrtc.connectionState}</span>
          </span>
        </div>

        {/* VU Meters */}
        <div className="space-y-2">
          <VUMeter label="Input (Mic)" level={inputLevel} />
          <VUMeter label="Output (Speaker)" level={outputLevel} />
        </div>
      </div>

      {/* Debug Panel */}
      <WebRTCDebugPanel
        connectionState={webrtc.connectionState}
        iceState={webrtc.iceState}
        signalingState={webrtc.signalingState}
        localSdp={webrtc.localSdp}
        remoteSdp={webrtc.remoteSdp}
        candidates={webrtc.candidates}
        events={webrtc.events}
        stats={webrtc.stats}
      />
    </div>
  );
}

// Hook for computing audio level from a MediaStream
function useAudioLevel(stream: MediaStream | null): number {
  const [level, setLevel] = useState(0);
  const animFrameRef = useRef<number>(0);
  const analyserRef = useRef<AnalyserNode | null>(null);
  const ctxRef = useRef<AudioContext | null>(null);

  const update = useCallback(() => {
    const analyser = analyserRef.current;
    if (!analyser) {
      setLevel(0);
      return;
    }
    const data = new Uint8Array(analyser.fftSize);
    analyser.getByteTimeDomainData(data);
    let sum = 0;
    for (let i = 0; i < data.length; i++) {
      const v = (data[i]! - 128) / 128;
      sum += v * v;
    }
    const rms = Math.sqrt(sum / data.length);
    setLevel(Math.min(1, rms * 3)); // amplify for visibility
    animFrameRef.current = requestAnimationFrame(update);
  }, []);

  useEffect(() => {
    if (!stream || stream.getAudioTracks().length === 0) {
      setLevel(0);
      return;
    }

    const ctx = new AudioContext();
    ctxRef.current = ctx;
    const source = ctx.createMediaStreamSource(stream);
    const analyser = ctx.createAnalyser();
    analyser.fftSize = 256;
    source.connect(analyser);
    analyserRef.current = analyser;

    animFrameRef.current = requestAnimationFrame(update);

    return () => {
      cancelAnimationFrame(animFrameRef.current);
      analyserRef.current = null;
      ctx.close();
      ctxRef.current = null;
    };
  }, [stream, update]);

  return level;
}
