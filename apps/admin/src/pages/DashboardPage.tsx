import { Link } from "react-router-dom";
import { DataState, PageHeader, Status } from "@crosstalk/theme";
import { useAuth } from "../hooks/useAuth";
import { getApiClient } from "../lib/api";
import { useEffect, useState } from "react";

interface DashboardStats {
  sessionsTotal: number | null;
  sessionsPageCount: number;
  abcsTotal: number | null;
  abcsOnlineOnPage: number;
  abcsPageCount: number;
  translatorsTotal: number | null;
  translatorsPageCount: number;
  fetchedAt: Date | null;
}

type LoadState = "loading" | "ready" | "error";

export function DashboardPage() {
  const { token } = useAuth();
  const [stats, setStats] = useState<DashboardStats>({
    sessionsTotal: null,
    sessionsPageCount: 0,
    abcsTotal: null,
    abcsOnlineOnPage: 0,
    abcsPageCount: 0,
    translatorsTotal: null,
    translatorsPageCount: 0,
    fetchedAt: null,
  });
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [error, setError] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    let cancelled = false;
    const controller = new AbortController();

    async function fetchStats() {
      if (!token) return;
      setLoadState("loading");
      setError(null);
      const client = getApiClient(token);

      try {
        const [sessionsRes, abcsRes, translatorsRes] = await Promise.all([
          client.GET("/api/sessions", {
            params: { query: { limit: 100 } },
            signal: controller.signal,
          }),
          client.GET("/api/abcs", {
            params: { query: { limit: 100 } },
            signal: controller.signal,
          }),
          client.GET("/api/translators", {
            params: { query: { limit: 100 } },
            signal: controller.signal,
          }),
        ]);

        if (cancelled) return;

        const sessionErr = sessionsRes.error;
        const abcErr = abcsRes.error;
        const translatorErr = translatorsRes.error;
        if (sessionErr || abcErr || translatorErr) {
          setError(
            sessionErr?.detail ||
              abcErr?.detail ||
              translatorErr?.detail ||
              "Failed to load dashboard measurements",
          );
          setLoadState("error");
          return;
        }

        const sessions = sessionsRes.data?.data ?? [];
        const abcs = abcsRes.data?.data ?? [];
        const translators = translatorsRes.data?.data ?? [];

        setStats({
          sessionsTotal: sessionsRes.data?.total ?? null,
          sessionsPageCount: sessions.length,
          abcsTotal: abcsRes.data?.total ?? null,
          abcsOnlineOnPage: abcs.filter((a) => a.connected).length,
          abcsPageCount: abcs.length,
          translatorsTotal: translatorsRes.data?.total ?? null,
          translatorsPageCount: translators.length,
          fetchedAt: new Date(),
        });
        setLoadState("ready");
      } catch (err) {
        if (cancelled || controller.signal.aborted) return;
        setError(err instanceof Error ? err.message : "Failed to load dashboard");
        setLoadState("error");
      }
    }

    void fetchStats();
    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [token, reloadKey]);

  if (loadState === "loading") {
    return (
      <DataState
        kind="loading"
        title="Loading dashboard"
        description="Reading session, ABC, and translator collections from the server."
      />
    );
  }

  if (loadState === "error") {
    return (
      <DataState
        kind="error"
        title="Dashboard measurements unavailable"
        description={error ?? "The collection endpoints did not return data."}
        action={
          <button
            type="button"
            onClick={() => setReloadKey((k) => k + 1)}
            className="inline-flex min-h-[var(--house-control-height)] items-center border border-[var(--house-rule-strong)] px-4 text-sm font-medium text-[var(--house-text-secondary)]"
          >
            Retry
          </button>
        }
      />
    );
  }

  const sessionCount = stats.sessionsTotal ?? stats.sessionsPageCount;
  const abcCount = stats.abcsTotal ?? stats.abcsPageCount;
  const translatorCount = stats.translatorsTotal ?? stats.translatorsPageCount;
  const onlineLabel =
    stats.abcsTotal != null && stats.abcsPageCount < stats.abcsTotal
      ? `${stats.abcsOnlineOnPage} online in this page of ${stats.abcsPageCount} (total ${stats.abcsTotal})`
      : `${stats.abcsOnlineOnPage} of ${abcCount} ABCs report connected`;

  const statusSentence = `Collection snapshot: ${sessionCount} session${sessionCount === 1 ? "" : "s"}, ${onlineLabel}, ${translatorCount} translator account${translatorCount === 1 ? "" : "s"}. No separate health, uptime, or signaling probe is exposed by the API.`;

  const actionLinkClass =
    "inline-flex min-h-[var(--house-control-height)] items-center border border-[var(--house-rule-strong)] px-4 text-sm font-medium text-[var(--house-text-secondary)] hover:bg-[var(--house-bg-raised)]";

  return (
    <div className="space-y-6">
      <PageHeader
        eyebrow="Monitor"
        title="Dashboard"
        lede="Live counts from the admin collection endpoints. Claims are limited to what those responses contain."
        meta={
          stats.fetchedAt ? (
            <span>Last refreshed {stats.fetchedAt.toLocaleString()}</span>
          ) : null
        }
      />

      <section
        aria-label="Operational status"
        className="border-y border-border py-4"
      >
        <p className="house-type-lede text-foreground">{statusSentence}</p>
        <div className="mt-3">
          <Status tone="info">Collection snapshot only</Status>
        </div>
      </section>

      <section aria-label="Measurements">
        <h2 className="house-type-section mb-3">Measurements</h2>
        <dl className="divide-y divide-border border-y border-border">
          <div className="grid grid-cols-1 gap-1 py-3 sm:grid-cols-[minmax(10rem,14rem)_1fr] sm:items-baseline sm:gap-6">
            <dt className="house-type-label text-muted-foreground">Sessions</dt>
            <dd className="house-type-data text-foreground">
              {sessionCount}
              <span className="ml-2 house-type-meta text-muted-foreground">
                total from list endpoint
              </span>
            </dd>
          </div>
          <div className="grid grid-cols-1 gap-1 py-3 sm:grid-cols-[minmax(10rem,14rem)_1fr] sm:items-baseline sm:gap-6">
            <dt className="house-type-label text-muted-foreground">ABCs connected</dt>
            <dd className="house-type-data text-foreground">
              {stats.abcsOnlineOnPage}
              <span className="ml-2 house-type-meta text-muted-foreground">
                of {abcCount} listed
              </span>
            </dd>
          </div>
          <div className="grid grid-cols-1 gap-1 py-3 sm:grid-cols-[minmax(10rem,14rem)_1fr] sm:items-baseline sm:gap-6">
            <dt className="house-type-label text-muted-foreground">Translators</dt>
            <dd className="house-type-data text-foreground">
              {translatorCount}
              <span className="ml-2 house-type-meta text-muted-foreground">
                accounts in scope
              </span>
            </dd>
          </div>
        </dl>
      </section>

      <section aria-label="Operate">
        <h2 className="house-type-section mb-3">Operate</h2>
        <div className="flex flex-wrap gap-2 border-t border-border pt-4">
          <Link to="/sessions" className={actionLinkClass}>
            Open sessions
          </Link>
          <Link to="/abcs" className={actionLinkClass}>
            Open ABCs
          </Link>
          <Link to="/translators" className={actionLinkClass}>
            Open translators
          </Link>
          <Link to="/debug" className={actionLinkClass}>
            Open debug peers
          </Link>
        </div>
      </section>
    </div>
  );
}
