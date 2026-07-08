import { useEffect, useState } from "react";
import { VUMeter } from "@crosstalk/theme";
import { parseBroadcastParams } from "./params";
import { fetchBroadcastInfo, BroadcastApiError, type BroadcastInfo } from "./api";
import { useBroadcastListener } from "./use-broadcast-listener";

type AppState =
  | { kind: "loading" }
  | { kind: "error"; message: string }
  | { kind: "ready"; info: BroadcastInfo }
  | { kind: "listening"; info: BroadcastInfo };

export function App() {
  const { sessionId, token, debug } = parseBroadcastParams();
  const [appState, setAppState] = useState<AppState>({ kind: "loading" });

  useEffect(() => {
    if (!sessionId || !token) {
      setAppState({ kind: "error", message: "Invalid broadcast link. Please check the URL." });
      return;
    }

    fetchBroadcastInfo(sessionId, token)
      .then((info) => setAppState({ kind: "ready", info }))
      .catch((err) => {
        if (err instanceof BroadcastApiError) {
          setAppState({ kind: "error", message: err.message });
        } else {
          setAppState({ kind: "error", message: "Failed to connect. Please try again." });
        }
      });
  }, [sessionId, token]);

  if (appState.kind === "loading") {
    return <LoadingScreen />;
  }

  if (appState.kind === "error") {
    return <ErrorScreen message={appState.message} />;
  }

  return (
    <ListenerView
      sessionId={sessionId!}
      token={token!}
      info={appState.info}
      listening={appState.kind === "listening"}
      onListen={() => setAppState({ kind: "listening", info: appState.info })}
      debug={debug}
    />
  );
}

function LoadingScreen() {
  return (
    <div className="min-h-screen flex items-center justify-center">
      <div className="animate-spin rounded-full h-12 w-12 border-4 border-slate-600 border-t-blue-500" />
    </div>
  );
}

function ErrorScreen({ message }: { message: string }) {
  return (
    <div className="min-h-screen flex items-center justify-center p-6">
      <div className="text-center max-w-sm">
        <div className="text-red-400 text-5xl mb-4">⚠</div>
        <h1 className="text-xl font-semibold text-white mb-2">Unable to Connect</h1>
        <p className="text-slate-400">{message}</p>
      </div>
    </div>
  );
}

interface ListenerViewProps {
  sessionId: string;
  token: string;
  info: BroadcastInfo;
  listening: boolean;
  onListen: () => void;
  debug: boolean;
}

function ListenerView({ sessionId, token, info, listening, onListen, debug }: ListenerViewProps) {
  const broadcast = useBroadcastListener({
    sessionId,
    token,
    info,
    enabled: listening,
  });

  return (
    <div className="min-h-screen flex flex-col items-center justify-center p-6">
      <div className="w-full max-w-sm space-y-8">
        {/* Session header */}
        <div className="text-center">
          <h1 className="text-2xl font-bold text-white mb-2">{info.session_name}</h1>
          {listening && broadcast.state === "live" && <LiveBadge />}
          {listening && broadcast.state === "connecting" && (
            <span className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-yellow-900/50 text-yellow-300 text-sm">
              <span className="animate-spin h-3 w-3 border-2 border-yellow-300 border-t-transparent rounded-full" />
              Connecting...
            </span>
          )}
          {listening && broadcast.state === "error" && (
            <span className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-red-900/50 text-red-300 text-sm">
              ● Disconnected
            </span>
          )}
        </div>

        {/* Listen button or controls */}
        {!listening ? (
          <button
            onClick={onListen}
            className="w-full py-6 rounded-2xl bg-blue-600 hover:bg-blue-500 active:bg-blue-700 text-white text-2xl font-bold transition-colors shadow-lg shadow-blue-600/30"
          >
            🎧 Listen
          </button>
        ) : (
          <div className="space-y-6">
            {/* Pause/Resume */}
            <button
              onClick={broadcast.togglePause}
              className="w-full py-4 rounded-xl bg-slate-700 hover:bg-slate-600 active:bg-slate-800 text-white text-lg font-medium transition-colors"
            >
              {broadcast.paused ? "▶ Resume" : "⏸ Pause"}
            </button>

            {/* Signal level (measured before the volume control) */}
            <div className="space-y-2">
              <span className="text-sm text-slate-400">Signal</span>
              <VUMeter level={broadcast.level} showValue={false} />
            </div>

            {/* Volume */}
            <div className="space-y-2">
              <label className="text-sm text-slate-400 flex justify-between">
                <span>Volume</span>
                <span>{Math.round(broadcast.volume * 100)}%</span>
              </label>
              <input
                type="range"
                min="0"
                max="1"
                step="0.01"
                value={broadcast.volume}
                onChange={(e) => broadcast.setVolume(parseFloat(e.target.value))}
                className="w-full h-2 bg-slate-700 rounded-full appearance-none cursor-pointer accent-blue-500"
              />
            </div>

            {/* Listener count */}
            <div className="text-center text-slate-400 text-sm">
              👥 {broadcast.listenerCount} listener{broadcast.listenerCount !== 1 ? "s" : ""}
            </div>
          </div>
        )}

        {/* Error message */}
        {broadcast.error && (
          <p className="text-center text-red-400 text-sm">{broadcast.error}</p>
        )}

        {/* Debug panel */}
        {debug && listening && <DebugPanel debugInfo={broadcast.debugInfo} />}
      </div>
    </div>
  );
}

function LiveBadge() {
  return (
    <span className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-red-900/50 text-red-300 text-sm">
      <span className="relative flex h-2.5 w-2.5">
        <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-red-400 opacity-75" />
        <span className="relative inline-flex rounded-full h-2.5 w-2.5 bg-red-500" />
      </span>
      LIVE
    </span>
  );
}

function DebugPanel({ debugInfo }: { debugInfo: { iceState: string; signalingState: string; candidates: string[]; localSdp: string | null; remoteSdp: string | null } }) {
  return (
    <div className="mt-6 p-4 rounded-xl bg-slate-800 border border-slate-700 text-xs font-mono space-y-2 overflow-x-auto">
      <h3 className="text-slate-300 font-semibold text-sm">Debug Info</h3>
      <div>
        <span className="text-slate-500">ICE:</span>{" "}
        <span className="text-green-400">{debugInfo.iceState}</span>
      </div>
      <div>
        <span className="text-slate-500">Signaling:</span>{" "}
        <span className="text-blue-400">{debugInfo.signalingState}</span>
      </div>
      <div>
        <span className="text-slate-500">Candidates ({debugInfo.candidates.length}):</span>
        {debugInfo.candidates.length > 0 && (
          <pre className="text-slate-400 mt-1 whitespace-pre-wrap break-all max-h-32 overflow-y-auto">
            {debugInfo.candidates.join("\n")}
          </pre>
        )}
      </div>
      {debugInfo.remoteSdp && (
        <details>
          <summary className="text-slate-500 cursor-pointer">Remote SDP</summary>
          <pre className="text-slate-400 mt-1 whitespace-pre-wrap break-all max-h-40 overflow-y-auto">
            {debugInfo.remoteSdp}
          </pre>
        </details>
      )}
      {debugInfo.localSdp && (
        <details>
          <summary className="text-slate-500 cursor-pointer">Local SDP</summary>
          <pre className="text-slate-400 mt-1 whitespace-pre-wrap break-all max-h-40 overflow-y-auto">
            {debugInfo.localSdp}
          </pre>
        </details>
      )}
    </div>
  );
}
