import { useState, useEffect, useRef, useMemo } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { createApiClient } from "@crosstalk/api-client";
import { SessionAudioManager } from "@crosstalk/session-audio";
import { VUMeter, useAudioLevel } from "@crosstalk/theme";
import { useAuth } from "../hooks/useAuth";
import { useWebRTC } from "../hooks/useWebRTC";
import { WebRTCDebugPanel } from "../components/WebRTCDebugPanel";
import { BroadcastShare } from "../components/BroadcastShare";

export function SessionConnectPage() {
  const { id: sessionId } = useParams<{ id: string }>();
  const { getToken } = useAuth();
  const navigate = useNavigate();

  const [audioDevices, setAudioDevices] = useState<MediaDeviceInfo[]>([]);
  const [selectedDevice, setSelectedDevice] = useState<string>("");
  const [isConnected, setIsConnected] = useState(false);
  const [broadcastToken, setBroadcastToken] = useState<string | null>(null);

  const token = getToken();

  const audioClient = useMemo(
    () =>
      token
        ? createApiClient({ baseUrl: window.location.origin, token })
        : null,
    [token],
  );

  // Load the session's broadcast token so translators can share the public
  // listener link (clickable + QR).
  useEffect(() => {
    if (!sessionId || !audioClient) return;
    audioClient
      .GET("/api/sessions/{id}/broadcast-url", { params: { path: { id: sessionId } } })
      .then(({ data }) => {
        setBroadcastToken(data?.broadcast_token ?? null);
      })
      .catch(() => setBroadcastToken(null));
  }, [sessionId, audioClient]);

  // Optional SFU routing overrides via ?produce=&listen= (deep links / e2e).
  // The mic connection is produce-only by default: monitoring of every channel
  // is handled independently by SessionAudioManager (receive-only per-channel
  // monitors), so the mic connection does not also listen (which would double
  // the audio). An explicit ?listen= still overrides.
  const search = new URLSearchParams(window.location.search);
  const produce = search.get("produce") ?? undefined;
  const listen = search.get("listen") ?? "";

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
    try {
      await webrtc.connect();
    } catch (err) {
      // Surface mint/WS/ICE failures in the event log instead of a silent
      // unhandled rejection that leaves the Connect button stuck forever.
      console.error("connect failed", err);
    }
  };

  const handleDisconnect = () => {
    webrtc.disconnect();
  };

  // Mic input level (pre-transmit).
  const inputLevel = useAudioLevel(webrtc.localStream);

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
        <h2 className="text-lg font-semibold text-white">Microphone</h2>

        {/* Mic selector */}
        <div>
          <label className="block text-sm text-gray-400 mb-1">Input device</label>
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

        {/* Mic input level */}
        <VUMeter label="Input (Mic)" level={inputLevel} />
      </div>

      {/* Session audio: monitor every channel + source→channel mixing */}
      <div className="bg-gray-800 border border-gray-700 rounded-lg p-4 mb-4">
        <h2 className="text-lg font-semibold text-white mb-3">Session Audio</h2>
        {audioClient && sessionId && token ? (
          <SessionAudioManager
            client={audioClient}
            token={token}
            sessionId={sessionId}
          />
        ) : (
          <p className="text-sm text-gray-500">Sign in to manage audio.</p>
        )}
      </div>

      {/* Broadcast link + QR */}
      <div className="bg-gray-800 border border-gray-700 rounded-lg p-4 mb-4">
        <h2 className="text-lg font-semibold text-white mb-3">Broadcast Link</h2>
        <BroadcastShare sessionId={sessionId ?? ""} token={broadcastToken} />
      </div>

      {/* Debug Panel */}
      <WebRTCDebugPanel
        connectionState={webrtc.connectionState}
        iceState={webrtc.iceState}
        signalingState={webrtc.signalingState}
        dataChannelState={webrtc.dataChannelState}
        localSdp={webrtc.localSdp}
        remoteSdp={webrtc.remoteSdp}
        candidates={webrtc.candidates}
        events={webrtc.events}
        stats={webrtc.stats}
      />
    </div>
  );
}
