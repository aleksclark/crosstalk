import { useCallback, useEffect, useRef, useState } from "react";
import { getApiClient } from "../lib/api";
import type { components } from "@crosstalk/api-client";
import { Button } from "@crosstalk/theme";

type AudioSettings = components["schemas"]["ABCAudioSettingsOut"];
type Capability = components["schemas"]["ABCAudioCapabilityOut"];
type OverallState = AudioSettings["overall_state"];
type ControlState =
  | "unknown"
  | "pending"
  | "applied"
  | "unsupported"
  | "error"
  | "device_mismatch";

const DEFAULT_VOLUME = 80;
const DEFAULT_GAIN = 50;
const FAST_POLL_MS = 2000;
const SLOW_POLL_MS = 10000;

export type ABCAudioControlsProps = {
  abcId: string;
  token: string;
  /** When false, parent already knows ABC is offline; still prefer settings.connected. */
  abcConnected?: boolean;
};

type Editable = {
  outputVolume: number;
  outputMuted: boolean;
  inputGain: number;
  outputDeviceUid: string;
  inputDeviceUid: string;
};

function clampPercent(n: number): number {
  if (!Number.isFinite(n)) return 0;
  return Math.max(0, Math.min(100, Math.round(n)));
}

function truncateUid(uid: string, max = 28): string {
  if (!uid) return "—";
  if (uid.length <= max) return uid;
  return `${uid.slice(0, max - 1)}…`;
}

function pickCapability(
  caps: Capability[] | null | undefined,
  direction: "output" | "input",
): Capability | undefined {
  if (!caps?.length) return undefined;
  const exact = caps.find(
    (c) => c.direction === direction || c.direction === "both",
  );
  if (exact) return exact;
  // Prefer any cap with a UID when direction is unspecified.
  return caps.find((c) => !!c.device_uid);
}

function initEditable(settings: AudioSettings): Editable {
  const desired = settings.desired;
  const reported = settings.reported;
  const caps = reported.capabilities ?? [];
  const outCap = pickCapability(caps, "output");
  const inCap = pickCapability(caps, "input");

  const hasDesired = (desired.revision ?? 0) > 0;

  const outputDeviceUid =
    desired.output_device_uid ||
    reported.output_device_uid ||
    outCap?.device_uid ||
    "";
  const inputDeviceUid =
    desired.input_device_uid ||
    reported.input_device_uid ||
    inCap?.device_uid ||
    "";

  let outputVolume = DEFAULT_VOLUME;
  let outputMuted = false;
  let inputGain = DEFAULT_GAIN;

  if (hasDesired) {
    if (desired.output_volume_percent != null) {
      outputVolume = clampPercent(desired.output_volume_percent);
    }
    if (desired.output_muted != null) {
      outputMuted = desired.output_muted;
    }
    if (desired.input_gain_percent != null) {
      inputGain = clampPercent(desired.input_gain_percent);
    }
  } else {
    if (reported.observed_output_volume_percent != null) {
      outputVolume = clampPercent(reported.observed_output_volume_percent);
    }
    if (reported.observed_output_muted != null) {
      outputMuted = reported.observed_output_muted;
    }
    if (reported.observed_input_gain_percent != null) {
      inputGain = clampPercent(reported.observed_input_gain_percent);
    }
  }

  return {
    outputVolume,
    outputMuted,
    inputGain,
    outputDeviceUid,
    inputDeviceUid,
  };
}

function supportsVolume(settings: AudioSettings | null, uid: string): boolean {
  if (!settings) return true;
  const caps = settings.reported.capabilities ?? [];
  if (!caps.length) {
    // Offline configured: allow previously bound controls.
    return (settings.desired.revision ?? 0) > 0 && !!uid;
  }
  const cap =
    caps.find((c) => c.device_uid === uid) || pickCapability(caps, "output");
  if (!cap) return false;
  // Missing flag → assume supported when UID is known (capability inventory incomplete).
  if (cap.supports_volume === false) return false;
  return true;
}

function supportsMute(settings: AudioSettings | null, uid: string): boolean {
  if (!settings) return true;
  const caps = settings.reported.capabilities ?? [];
  if (!caps.length) {
    return (settings.desired.revision ?? 0) > 0 && !!uid;
  }
  const cap =
    caps.find((c) => c.device_uid === uid) || pickCapability(caps, "output");
  if (!cap) return false;
  if (cap.supports_mute === false) return false;
  return true;
}

function supportsGain(settings: AudioSettings | null, uid: string): boolean {
  if (!settings) return true;
  const caps = settings.reported.capabilities ?? [];
  if (!caps.length) {
    return (settings.desired.revision ?? 0) > 0 && !!uid;
  }
  const cap =
    caps.find((c) => c.device_uid === uid) || pickCapability(caps, "input");
  if (!cap) return false;
  if (cap.supports_gain === false) return false;
  return true;
}

function overallMessage(
  settings: AudioSettings | null,
  ui: {
    conflict: string | null;
    saveError: string | null;
    fetchError: string | null;
    offlineQueued: boolean;
    lastSaveAccepted: boolean;
  },
): string {
  if (ui.fetchError) return ui.fetchError;
  if (ui.conflict) return ui.conflict;
  if (ui.saveError) return ui.saveError;
  if (!settings) return "";

  const state = settings.overall_state;
  if (ui.offlineQueued || (state === "offline" && ui.lastSaveAccepted)) {
    return "saved; will apply on reconnect";
  }
  switch (state) {
    case "pending":
      return "pending — waiting for device to apply settings";
    case "stale":
      return "stale — device report is outdated";
    case "unsupported":
      return "unsupported — device cannot apply one or more controls";
    case "device_mismatch":
      return "device mismatch — bound device is not currently present";
    case "error":
      return (
        settings.reported.error_detail ||
        settings.reported.error_code ||
        "error applying audio settings"
      );
    case "partial":
      return "partial — some controls applied";
    case "applied":
      return "applied";
    case "offline":
      return "offline — edits queue until reconnect";
    case "unconfigured":
      return "unconfigured — set levels and save when device UIDs are known";
    default:
      return state;
  }
}

function badgeClass(state: OverallState | "conflict" | "loading"): string {
  switch (state) {
    case "applied":
      return "bg-green-500/20 text-green-400";
    case "pending":
    case "partial":
      return "bg-amber-500/20 text-amber-300";
    case "offline":
    case "unconfigured":
      return "bg-gray-500/20 text-gray-400";
    case "stale":
      return "bg-yellow-500/20 text-yellow-300";
    case "unsupported":
    case "device_mismatch":
    case "error":
    case "conflict":
      return "bg-red-500/20 text-red-400";
    default:
      return "bg-gray-500/20 text-gray-400";
  }
}

function controlStateLabel(state: ControlState | undefined): string {
  return state || "unknown";
}

function extractErrorDetail(error: unknown, fallback: string): string {
  if (!error) return fallback;
  if (typeof error === "object" && error !== null && "detail" in error) {
    const d = (error as { detail?: unknown }).detail;
    if (typeof d === "string" && d.trim()) return d;
  }
  if (error instanceof Error && error.message) return error.message;
  return fallback;
}

export function ABCAudioControls({
  abcId,
  token,
}: ABCAudioControlsProps) {
  const [settings, setSettings] = useState<AudioSettings | null>(null);
  const [editable, setEditable] = useState<Editable | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [fetchError, setFetchError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [conflict, setConflict] = useState<string | null>(null);
  const [offlineQueued, setOfflineQueued] = useState(false);
  const [lastSaveAccepted, setLastSaveAccepted] = useState(false);
  const [statusNote, setStatusNote] = useState<string | null>(null);

  // Retain request_id for retry until definitive response (not 409-refetch alone).
  const pendingRequestId = useRef<string | null>(null);
  const expectedRevisionRef = useRef<number>(0);
  const inFlight = useRef(false);
  const mounted = useRef(true);
  const dirty = useRef(false);
  const pollTimer = useRef<number | null>(null);

  const applySettings = useCallback(
    (next: AudioSettings, opts?: { preserveEdits?: boolean }) => {
      setSettings(next);
      expectedRevisionRef.current = next.desired.revision ?? 0;
      if (!opts?.preserveEdits || !dirty.current) {
        setEditable(initEditable(next));
        dirty.current = false;
      }
      if (next.overall_state === "applied") {
        setOfflineQueued(false);
        setLastSaveAccepted(false);
        pendingRequestId.current = null;
      } else if (next.connected && next.overall_state === "pending") {
        setOfflineQueued(false);
      }
    },
    [],
  );

  const fetchSettings = useCallback(
    async (opts?: { silent?: boolean }) => {
      if (!token || !abcId) return;
      if (inFlight.current) return;
      inFlight.current = true;
      if (!opts?.silent) setLoading(true);
      try {
        const client = getApiClient(token);
        const { data, error, response } = await client.GET(
          "/api/abcs/{id}/audio-settings",
          { params: { path: { id: abcId } } },
        );
        if (!mounted.current) return;
        if (error || !data) {
          const status = response?.status;
          const detail = extractErrorDetail(
            error,
            status
              ? `Failed to load audio settings (${status})`
              : "Failed to load audio settings",
          );
          setFetchError(detail);
          if (!opts?.silent) {
            setSettings(null);
          }
          return;
        }
        setFetchError(null);
        applySettings(data, { preserveEdits: opts?.silent === true });
      } catch (e) {
        if (!mounted.current) return;
        setFetchError(
          e instanceof Error ? e.message : "Failed to load audio settings",
        );
      } finally {
        inFlight.current = false;
        if (mounted.current && !opts?.silent) setLoading(false);
      }
    },
    [token, abcId, applySettings],
  );

  // Initial load + restart on id/token change.
  useEffect(() => {
    mounted.current = true;
    dirty.current = false;
    pendingRequestId.current = null;
    setOfflineQueued(false);
    setLastSaveAccepted(false);
    setConflict(null);
    setSaveError(null);
    setStatusNote(null);
    setEditable(null);
    setSettings(null);
    void fetchSettings();
    return () => {
      mounted.current = false;
      if (pollTimer.current != null) {
        window.clearTimeout(pollTimer.current);
        pollTimer.current = null;
      }
    };
  }, [fetchSettings]);

  // Polling: 2s while pending or offline-after-save; 10s otherwise. No overlap.
  useEffect(() => {
    if (!token || !abcId || !settings) return;

    const needsFast =
      settings.overall_state === "pending" ||
      offlineQueued ||
      (lastSaveAccepted && !settings.connected);

    const delay = needsFast ? FAST_POLL_MS : SLOW_POLL_MS;

    if (pollTimer.current != null) {
      window.clearTimeout(pollTimer.current);
    }
    pollTimer.current = window.setTimeout(() => {
      void fetchSettings({ silent: true });
    }, delay);

    return () => {
      if (pollTimer.current != null) {
        window.clearTimeout(pollTimer.current);
        pollTimer.current = null;
      }
    };
  }, [
    token,
    abcId,
    settings,
    offlineQueued,
    lastSaveAccepted,
    fetchSettings,
  ]);

  const onEdit = <K extends keyof Editable>(key: K, value: Editable[K]) => {
    dirty.current = true;
    setEditable((prev) => (prev ? { ...prev, [key]: value } : prev));
    setConflict(null);
    setSaveError(null);
    setStatusNote(null);
  };

  const handleSave = async () => {
    if (!token || !abcId || !editable || saving) return;

    const outUid = editable.outputDeviceUid.trim();
    const inUid = editable.inputDeviceUid.trim();
    if (!outUid || !inUid) {
      setSaveError("Device UIDs required before saving audio settings");
      return;
    }

    setSaving(true);
    setSaveError(null);
    setConflict(null);
    setStatusNote(null);

    // One request_id per Save; retain for retry until definitive response.
    if (!pendingRequestId.current) {
      pendingRequestId.current = crypto.randomUUID();
    }
    const requestId = pendingRequestId.current;
    const expectedRevision =
      settings?.desired.revision ?? expectedRevisionRef.current ?? 0;

    try {
      const client = getApiClient(token);
      const { data, error, response } = await client.PUT(
        "/api/abcs/{id}/audio-settings",
        {
          params: { path: { id: abcId } },
          body: {
            request_id: requestId,
            expected_revision: expectedRevision,
            output: {
              device_uid: outUid,
              volume_percent: clampPercent(editable.outputVolume),
              muted: editable.outputMuted,
            },
            input: {
              device_uid: inUid,
              gain_percent: clampPercent(editable.inputGain),
            },
          },
        },
      );

      if (!mounted.current) return;

      const status = response?.status ?? 0;

      if (status === 409) {
        setConflict(
          extractErrorDetail(
            error,
            "Conflict: settings changed elsewhere — reloading",
          ),
        );
        pendingRequestId.current = null; // new attempt needs fresh id after conflict
        dirty.current = false;
        await fetchSettings({ silent: true });
        return;
      }

      if (error && status !== 200 && status !== 202) {
        setSaveError(
          extractErrorDetail(error, `Save failed (${status || "network"})`),
        );
        return;
      }

      // 200 = duplicate/no-op; 202 = queued. Never claim "applied" from PUT alone.
      setLastSaveAccepted(true);
      dirty.current = false;

      if (data) {
        applySettings(data, { preserveEdits: false });
        if (!data.connected || data.overall_state === "offline") {
          setOfflineQueued(true);
          setStatusNote("saved; will apply on reconnect");
        } else if (status === 202 || data.overall_state === "pending") {
          setStatusNote("queued — waiting for device");
        } else if (status === 200) {
          setStatusNote("accepted (no new revision)");
        }
        // Definitive applied only when GET/body reports applied.
        if (data.overall_state === "applied") {
          pendingRequestId.current = null;
          setOfflineQueued(false);
        }
      } else {
        // 200 with empty body — refetch.
        await fetchSettings({ silent: true });
        setStatusNote("accepted");
      }
    } catch (e) {
      if (!mounted.current) return;
      setSaveError(e instanceof Error ? e.message : "Save failed");
    } finally {
      if (mounted.current) setSaving(false);
    }
  };

  if (loading && !settings) {
    return (
      <section
        className="space-y-3 border border-border bg-[var(--house-surface-raised)] p-4"
        aria-labelledby="abc-audio-controls-heading"
        data-testid="abc-audio-controls"
      >
        <h2 id="abc-audio-controls-heading" className="house-type-title text-lg">
          Audio controls
        </h2>
        <p className="house-type-body text-sm text-muted-foreground">Loading audio settings…</p>
      </section>
    );
  }

  if (fetchError && !settings) {
    return (
      <section
        className="space-y-3 border border-border bg-[var(--house-surface-raised)] p-4"
        aria-labelledby="abc-audio-controls-heading"
        data-testid="abc-audio-controls"
      >
        <h2 id="abc-audio-controls-heading" className="house-type-title text-lg">
          Audio controls
        </h2>
        <p
          className="house-type-body text-sm text-[var(--house-status-danger)]"
          role="alert"
          data-testid="abc-audio-fetch-error"
        >
          {fetchError}
        </p>
        <button
          type="button"
          className="house-type-body text-sm text-primary hover:underline"
          onClick={() => void fetchSettings()}
        >
          Retry
        </button>
      </section>
    );
  }

  const ed = editable ?? {
    outputVolume: DEFAULT_VOLUME,
    outputMuted: false,
    inputGain: DEFAULT_GAIN,
    outputDeviceUid: "",
    inputDeviceUid: "",
  };

  const volSupported = supportsVolume(settings, ed.outputDeviceUid);
  const muteSupported = supportsMute(settings, ed.outputDeviceUid);
  const gainSupported = supportsGain(settings, ed.inputDeviceUid);
  const awaitingCapabilities =
    (settings?.reported.capabilities?.length ?? 0) === 0 &&
    (settings?.desired.revision ?? 0) === 0;
  const hasUids = !!ed.outputDeviceUid && !!ed.inputDeviceUid;
  const canSave = hasUids && !saving && (volSupported || muteSupported || gainSupported);

  const overall = conflict
    ? "conflict"
    : settings?.overall_state ?? "unconfigured";
  const message = overallMessage(settings, {
    conflict,
    saveError,
    fetchError,
    offlineQueued,
    lastSaveAccepted,
  });

  const desiredRev = settings?.desired.revision ?? 0;
  const reportedRev = settings?.reported.revision ?? 0;
  const reportedAt = settings?.reported.reported_at
    ? new Date(settings.reported.reported_at).toLocaleString()
    : "—";

  return (
    <section
      className="space-y-4 border border-border bg-[var(--house-surface-raised)] p-4"
      aria-labelledby="abc-audio-controls-heading"
      data-testid="abc-audio-controls"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 id="abc-audio-controls-heading" className="house-type-title text-lg">
          Audio controls
        </h2>
        <span
          className={`inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-sm font-medium ${badgeClass(overall)}`}
          data-testid="abc-audio-overall-badge"
          data-state={overall}
        >
          {overall}
        </span>
      </div>

      {(message || statusNote) && (
        <p
          className={`text-sm ${
            conflict || saveError || fetchError || overall === "error"
              ? "text-red-400"
              : "text-muted-foreground"
          }`}
          role={conflict || saveError ? "alert" : "status"}
          data-testid="abc-audio-status-message"
        >
          {conflict || saveError || statusNote || message}
        </p>
      )}

      {awaitingCapabilities && (
        <p className="text-sm text-muted-foreground" role="status">
          Waiting for device capability report. Reconnect or update the K2B client if
          this persists.
        </p>
      )}

      <div className="grid grid-cols-1 md:grid-cols-2 gap-3 text-sm">
        <div>
          <p className="text-muted-foreground">Desired revision</p>
          <p data-testid="abc-audio-desired-revision">{desiredRev}</p>
        </div>
        <div>
          <p className="text-muted-foreground">Reported revision</p>
          <p data-testid="abc-audio-reported-revision">{reportedRev}</p>
        </div>
        <div>
          <p className="text-muted-foreground">Receipt time</p>
          <p data-testid="abc-audio-reported-at">{reportedAt}</p>
        </div>
        <div>
          <p className="text-muted-foreground">Connection</p>
          <p data-testid="abc-audio-connected">
            {settings?.connected ? "online" : "offline"}
          </p>
        </div>
      </div>

      {/* Devices */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-3 text-sm">
        <div>
          <p className="text-muted-foreground">Output device</p>
          <p
            className="font-mono text-xs break-all"
            title={ed.outputDeviceUid || undefined}
            data-testid="abc-audio-output-device"
          >
            {truncateUid(ed.outputDeviceUid)}
          </p>
        </div>
        <div>
          <p className="text-muted-foreground">Input device</p>
          <p
            className="font-mono text-xs break-all"
            title={ed.inputDeviceUid || undefined}
            data-testid="abc-audio-input-device"
          >
            {truncateUid(ed.inputDeviceUid)}
          </p>
        </div>
      </div>

      {/* Controls */}
      <div className="space-y-4">
        <div className="space-y-2">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <label htmlFor="abc-output-volume" className="text-sm font-medium">
              Output volume
            </label>
            <span
              className="text-xs text-muted-foreground"
              data-testid="abc-audio-output-volume-state"
            >
              {controlStateLabel(settings?.reported.output_volume_state)}
              {settings?.reported.observed_output_volume_percent != null
                ? ` · observed ${settings.reported.observed_output_volume_percent}`
                : ""}
            </span>
          </div>
          <div className="flex items-center gap-3">
            <input
              id="abc-output-volume"
              type="range"
              min={0}
              max={100}
              step={1}
              value={ed.outputVolume}
              disabled={!volSupported || !hasUids}
              onChange={(e) =>
                onEdit("outputVolume", clampPercent(Number(e.target.value)))
              }
              className="flex-1"
              aria-valuemin={0}
              aria-valuemax={100}
              aria-valuenow={ed.outputVolume}
              data-testid="abc-audio-output-volume"
            />
            <input
              type="number"
              min={0}
              max={100}
              step={1}
              value={ed.outputVolume}
              disabled={!volSupported || !hasUids}
              onChange={(e) =>
                onEdit("outputVolume", clampPercent(Number(e.target.value)))
              }
              className="w-16 bg-background border border-border rounded-md px-2 py-1 text-sm"
              aria-label="Output volume percent"
              data-testid="abc-audio-output-volume-number"
            />
          </div>
          {!volSupported && !awaitingCapabilities && (
            <p className="text-xs text-muted-foreground">
              Output volume unsupported on this device
            </p>
          )}
        </div>

        <div className="space-y-2">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <label
              htmlFor="abc-output-mute"
              className="text-sm font-medium inline-flex items-center gap-2"
            >
              <input
                id="abc-output-mute"
                type="checkbox"
                checked={ed.outputMuted}
                disabled={!muteSupported || !hasUids}
                onChange={(e) => onEdit("outputMuted", e.target.checked)}
                data-testid="abc-audio-output-mute"
              />
              Output mute
            </label>
            <span
              className="text-xs text-muted-foreground"
              data-testid="abc-audio-output-mute-state"
            >
              {controlStateLabel(settings?.reported.output_mute_state)}
              {settings?.reported.observed_output_muted != null
                ? ` · observed ${settings.reported.observed_output_muted ? "muted" : "unmuted"}`
                : ""}
            </span>
          </div>
          {!muteSupported && !awaitingCapabilities && (
            <p className="text-xs text-muted-foreground">
              Output mute unsupported on this device
            </p>
          )}
        </div>

        <div className="space-y-2">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <label htmlFor="abc-input-gain" className="text-sm font-medium">
              Input gain
            </label>
            <span
              className="text-xs text-muted-foreground"
              data-testid="abc-audio-input-gain-state"
            >
              {controlStateLabel(settings?.reported.input_gain_state)}
              {settings?.reported.observed_input_gain_percent != null
                ? ` · observed ${settings.reported.observed_input_gain_percent}`
                : ""}
            </span>
          </div>
          <div className="flex items-center gap-3">
            <input
              id="abc-input-gain"
              type="range"
              min={0}
              max={100}
              step={1}
              value={ed.inputGain}
              disabled={!gainSupported || !hasUids}
              onChange={(e) =>
                onEdit("inputGain", clampPercent(Number(e.target.value)))
              }
              className="flex-1"
              aria-valuemin={0}
              aria-valuemax={100}
              aria-valuenow={ed.inputGain}
              data-testid="abc-audio-input-gain"
            />
            <input
              type="number"
              min={0}
              max={100}
              step={1}
              value={ed.inputGain}
              disabled={!gainSupported || !hasUids}
              onChange={(e) =>
                onEdit("inputGain", clampPercent(Number(e.target.value)))
              }
              className="w-16 bg-background border border-border rounded-md px-2 py-1 text-sm"
              aria-label="Input gain percent"
              data-testid="abc-audio-input-gain-number"
            />
          </div>
          {!gainSupported && !awaitingCapabilities && (
            <p className="text-xs text-muted-foreground">
              Input gain unsupported on this device
            </p>
          )}
        </div>
      </div>

      {/* Desired snapshot */}
      {settings && (settings.desired.revision ?? 0) > 0 && (
        <div
          className="text-xs text-muted-foreground border-t border-border pt-3 space-y-1"
          data-testid="abc-audio-desired-snapshot"
        >
          <p>
            Desired: vol {settings.desired.output_volume_percent ?? "—"}
            {settings.desired.output_muted != null
              ? settings.desired.output_muted
                ? " · muted"
                : " · unmuted"
              : ""}
            {" · gain "}
            {settings.desired.input_gain_percent ?? "—"}
          </p>
        </div>
      )}

      <div className="flex flex-wrap items-center gap-3">
        <Button
          type="button"
          onClick={() => void handleSave()}
          disabled={!canSave}
          loading={saving}
          data-testid="abc-audio-save"
        >
          {saving ? "Saving…" : "Save audio settings"}
        </Button>
        {!hasUids && (
          <p className="text-xs text-muted-foreground" data-testid="abc-audio-uid-hint">
            Save disabled until device capability supplies UIDs
          </p>
        )}
      </div>
    </section>
  );
}
