import { CheckCircle2, CircleDashed, Eye, EyeOff, HelpCircle, Loader2, MinusCircle, Play, RefreshCw, XCircle } from "lucide-react";
import { Channel } from "mygo-runtime";
import { type ReactNode, useEffect, useRef, useState } from "react";
import { Logo } from "../components/Logo";
import { PageHeader } from "../components/Page";
import { delayClass, delayText, flag, relative } from "../lib/format";
import { useT } from "../lib/i18n";
import { toastError, useApp } from "../lib/store";
import { type IPInfo, type Site, type SiteResult, Tools, type Unlock as Result } from "../mygo";
import { Badge, Banner, Button } from "../ui";

// The connectivity page, after MyIP (github.com/jason5ng32/MyIP): the
// addresses that sites at home and abroad see, the reachability of popular
// sites over several rounds, and which streaming and AI services work.

const VIEWS = ["domestic", "global", "cloudflare"] as const;
type View = (typeof VIEWS)[number];

const ROUNDS = 5;

export default function Connectivity() {
  const t = useT();
  const running = useApp((s) => s.state?.core.status === "running");
  const preview = useApp((s) => s.preview);
  const ips = useExitIPs();
  const sites = useSites();
  const unlock = useUnlock();
  const started = useRef(false);
  useEffect(() => {
    // The addresses and the sites are quick, so they run on opening.
    if (running && !preview && !started.current) {
      started.current = true;
      void ips.load();
      void sites.test();
    }
  }, [running, preview]);
  const busy = ips.loading || sites.busy || unlock.busy;
  const runAll = async () => {
    await Promise.all([ips.load(), sites.test()]);
    await unlock.check([]);
  };
  return (
    <>
      <PageHeader title={t("nav.connectivity")}>
        <Button variant="primary" size="sm" icon={busy ? <Loader2 size={14} className="spin" /> : <Play size={14} />} disabled={busy || !running} onClick={runAll}>
          {t("unlock.checkAll")}
        </Button>
      </PageHeader>
      <div className="page-body">
        {!running && <Banner tone="warning">{t("common.coreNotRunning")}</Banner>}
        <IPSection {...ips} running={running} />
        <SiteSection {...sites} running={running} />
        <UnlockSection {...unlock} running={running} />
      </div>
    </>
  );
}

function Heading({ title, hint, children }: { title: ReactNode; hint: ReactNode; children?: ReactNode }) {
  return (
    <>
      <div className="section-title cty-title">
        {title}
        <div className="spacer" />
        {children}
      </div>
      <div className="muted cty-hint">{hint}</div>
    </>
  );
}

// ---------- Exit IPs ----------

type IPState = { info?: IPInfo; error?: string; loading?: boolean };

function useExitIPs() {
  const [ips, setIPs] = useState<Partial<Record<View, IPState>>>({});
  const [loading, setLoading] = useState(false);
  const load = async () => {
    setLoading(true);
    setIPs((old) => Object.fromEntries(VIEWS.map((v) => [v, { ...old[v], loading: true }])));
    await Promise.all(
      VIEWS.map(async (v) => {
        let next: IPState;
        try {
          next = { info: await Tools.exitIP(v) };
        } catch (e) {
          next = { error: String(e instanceof Error ? e.message : e) };
        }
        setIPs((old) => ({ ...old, [v]: next }));
      }),
    );
    setLoading(false);
  };
  return { ips, loading, load };
}

function IPSection({ ips, loading, load, running }: ReturnType<typeof useExitIPs> & { running: boolean }) {
  const t = useT();
  const [shown, setShown] = useState(false);
  const home = ips.domestic?.info?.ip;
  const abroad = ips.global?.info?.ip;
  return (
    <section className="cty-section">
      <Heading title={t("connectivity.ip")} hint={t("connectivity.ipHint")}>
        {home && abroad && <Badge tone={home !== abroad ? "success" : undefined}>{t(home !== abroad ? "connectivity.split" : "connectivity.notSplit")}</Badge>}
        <Button size="sm" variant="ghost" icon={shown ? <EyeOff size={14} /> : <Eye size={14} />} onClick={() => setShown(!shown)} tip={shown ? t("common.hide") : t("common.show")} />
        <Button size="sm" variant="ghost" icon={<RefreshCw size={14} />} loading={loading} disabled={!running} onClick={load} tip={t("common.refresh")} />
      </Heading>
      <div className="cty-grid cty-ips">
        {VIEWS.map((v) => {
          const s = ips[v];
          const info = s?.info;
          const place = info ? [...new Set([info.city, info.region].filter(Boolean))].join(" · ") : "";
          return (
            <div key={v} className="card card-pad col cty-ip">
              <div className="row cty-ip-head">
                <span className="grow">{t(`connectivity.view.${v}`)}</span>
                {info?.source && info.source !== t(`connectivity.view.${v}`) && <span className="faint">{info.source}</span>}
              </div>
              {s?.loading && !info ? (
                <>
                  <div className="skeleton" style={{ height: 22 }} />
                  <div className="skeleton" style={{ height: 58 }} />
                </>
              ) : s?.error ? (
                <div className="faint cty-error" title={s.error}>
                  {s.error}
                </div>
              ) : !info ? (
                <div className="faint">—</div>
              ) : (
                <>
                  <div className="cty-ip-place">
                    <span className="cty-flag">{flag(info.countryCode)}</span>
                    <span className="ellipsis" title={[info.country, place].filter(Boolean).join(" · ")}>
                      {info.country || "—"}
                      {place && <span className="muted"> · {place}</span>}
                    </span>
                  </div>
                  <dl className="kv">
                    <dt>IP</dt>
                    <dd className="mono selectable" style={{ filter: shown ? "none" : "blur(5px)", transition: "filter .2s" }}>
                      {info.ip}
                    </dd>
                    <dt>{t("home.isp")}</dt>
                    <dd title={info.isp}>{info.isp || "—"}</dd>
                    <dt>ASN</dt>
                    <dd>{info.asn ? `AS${info.asn}` : "—"}</dd>
                    {info.colo && (
                      <>
                        <dt>{t("connectivity.colo")}</dt>
                        <dd>{info.colo}</dd>
                      </>
                    )}
                  </dl>
                </>
              )}
            </div>
          );
        })}
      </div>
    </section>
  );
}

// ---------- Sites ----------

/** The delays of a site's rounds so far: ms, or 0 when the round failed. */
type Rounds = number[];

function useSites() {
  const t = useT();
  const [sites, setSites] = useState<Site[]>([]);
  const [rounds, setRounds] = useState<Record<string, Rounds>>({});
  const [busy, setBusy] = useState(false);
  const loaded = useRef<Promise<Site[]> | null>(null);
  const list = () => (loaded.current ??= Tools.connectivitySites().then((s) => (setSites(s), s)));
  useEffect(() => {
    if (!useApp.getState().preview) void list().catch(() => {});
  }, []);
  const test = async () => {
    setBusy(true);
    setRounds({});
    try {
      const all = await list();
      for (let i = 0; i < ROUNDS; i++) {
        const ch = new Channel<SiteResult>((r) => setRounds((old) => ({ ...old, [r.id]: [...(old[r.id] ?? []), r.error ? 0 : r.delayMs] })));
        await Tools.testSites(all, ch);
      }
    } catch (e) {
      toastError(t("common.failed"), e);
    }
    setBusy(false);
  };
  return { sites, rounds, busy, test };
}

function best(r: Rounds | undefined): number | undefined {
  const ok = r?.filter((d) => d > 0) ?? [];
  if (ok.length) return Math.min(...ok);
  return r?.length ? 0 : undefined;
}

/** verdict reads the results like MyIP does: whether sites at home and abroad open. */
function verdict(sites: Site[], rounds: Record<string, Rounds>): { tone: "success" | "warning" | "danger"; key: "ok" | "noGlobal" | "noDomestic" | "none" } | null {
  const opens = (group: string) => {
    const of = sites.filter((s) => (s.group ?? "global") === group);
    if (of.some((s) => (rounds[s.id]?.length ?? 0) === 0)) return undefined;
    return of.filter((s) => (best(rounds[s.id]) ?? 0) > 0).length * 2 >= of.length;
  };
  const home = opens("domestic");
  const abroad = opens("global");
  if (home === undefined || abroad === undefined) return null;
  if (home && abroad) return { tone: "success", key: "ok" };
  if (home) return { tone: "danger", key: "noGlobal" };
  if (abroad) return { tone: "warning", key: "noDomestic" };
  return { tone: "danger", key: "none" };
}

function SiteSection({ sites, rounds, busy, test, running }: ReturnType<typeof useSites> & { running: boolean }) {
  const t = useT();
  const v = verdict(sites, rounds);
  // One scale for every bar, so that the bars compare across sites.
  const slowest = Math.max(1, ...Object.values(rounds).flat());
  return (
    <section className="cty-section">
      <Heading title={t("connectivity.sites")} hint={t("connectivity.sitesHint", { rounds: ROUNDS })}>
        {v && <Badge tone={v.tone}>{t(`connectivity.verdict.${v.key}`)}</Badge>}
        <Button size="sm" variant="ghost" icon={<RefreshCw size={14} />} loading={busy} disabled={!running} onClick={test} tip={t("common.retest")} />
      </Heading>
      {(["global", "domestic"] as const).map((g) => {
        const of = sites.filter((s) => (s.group ?? "global") === g);
        if (!of.length) return null;
        return (
          <div key={g} className="cty-group">
            <div className="cty-group-name faint">{t(`connectivity.group.${g}`)}</div>
            <div className="cty-grid cty-sites">
              {of.map((s) => {
                const r = rounds[s.id];
                const b = best(r);
                return (
                  <div key={s.id} className="card cty-site" title={s.url}>
                    <div className="row">
                      <Logo id={s.id} name={s.name} size={22} />
                      <span className="grow ellipsis cty-name">{s.name}</span>
                      {busy && !r?.length ? <Loader2 size={13} className="spin muted" /> : <span className={`delay ${delayClass(b)}`}>{delayText(b, t("connectivity.unreachable"))}</span>}
                    </div>
                    <div className="cty-bars">
                      {Array.from({ length: ROUNDS }, (_, i) => {
                        const d = r?.[i];
                        const tip = d === undefined ? "" : t("connectivity.round", { n: i + 1, result: delayText(d, t("connectivity.unreachable")) });
                        return (
                          <span
                            key={i}
                            className={`cty-bar ${d === undefined ? (busy && i === (r?.length ?? 0) ? "next" : "none") : d ? delayClass(d) : "fail"}`}
                            style={d ? { height: `${30 + 70 * Math.sqrt(d / slowest)}%` } : undefined}
                            title={tip}
                          />
                        );
                      })}
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        );
      })}
    </section>
  );
}

// ---------- Streaming and AI ----------

const look: Record<string, { icon: typeof CheckCircle2; tone: "success" | "warning" | "danger" | undefined; color: string }> = {
  yes: { icon: CheckCircle2, tone: "success", color: "var(--success)" },
  partial: { icon: MinusCircle, tone: "warning", color: "var(--warning)" },
  no: { icon: XCircle, tone: "danger", color: "var(--danger)" },
  failed: { icon: HelpCircle, tone: undefined, color: "var(--text-faint)" },
  pending: { icon: CircleDashed, tone: undefined, color: "var(--text-faint)" },
  checking: { icon: Loader2, tone: undefined, color: "var(--text-muted)" },
};

function loadResults(): Record<string, Result> {
  try {
    return JSON.parse(localStorage.getItem("unlock.results") ?? "{}");
  } catch {
    return {};
  }
}

function useUnlock() {
  const t = useT();
  const [items, setItems] = useState<Result[]>([]);
  const [results, setResults] = useState<Record<string, Result>>(loadResults);
  const [checking, setChecking] = useState<Set<string>>(new Set());
  useEffect(() => {
    if (!useApp.getState().preview) Tools.unlockList().then(setItems).catch(() => {});
  }, []);
  const check = async (ids: string[]) => {
    setChecking((c) => new Set([...c, ...(ids.length ? ids : items.map((i) => i.id))]));
    const ch = new Channel<Result>((r) => {
      setResults((old) => {
        const next = { ...old, [r.id]: r };
        try {
          localStorage.setItem("unlock.results", JSON.stringify(next));
        } catch {
          // the results only last the session then
        }
        return next;
      });
      setChecking((c) => {
        const n = new Set(c);
        n.delete(r.id);
        return n;
      });
    });
    try {
      await Tools.checkUnlock(ids, ch);
    } catch (e) {
      toastError(t("common.failed"), e);
    }
    setChecking(new Set());
  };
  return { items, results, checking, check, busy: checking.size > 0 };
}

function UnlockSection({ items, results, checking, check, busy, running }: ReturnType<typeof useUnlock> & { running: boolean }) {
  const t = useT();
  const lang = useApp((s) => s.lang);
  return (
    <section className="cty-section">
      <Heading title={t("connectivity.unlock")} hint={t("unlock.hint")}>
        <Button size="sm" variant="ghost" icon={<RefreshCw size={14} />} loading={busy} disabled={!running} onClick={() => check([])} tip={t("unlock.checkAll")} />
      </Heading>
      <div className="cty-grid cty-unlock">
        {items.map((it) => {
          const r = results[it.id];
          const state = checking.has(it.id) ? "checking" : (r?.status ?? "pending");
          const l = look[state] ?? look.pending!;
          const Icon = l.icon;
          return (
            <div key={it.id} className="card card-pad col unlock-card" style={{ gap: 8 }}>
              <div className="row">
                <Logo id={it.id} name={it.name} size={22} />
                <span className="grow ellipsis cty-name">{it.name}</span>
                <Button size="sm" variant="ghost" icon={<RefreshCw size={13} />} disabled={state === "checking" || !running} onClick={() => check([it.id])} tip={t("common.retest")} />
              </div>
              <div className="row">
                <Badge tone={l.tone}>
                  <Icon size={12} className={state === "checking" ? "spin" : ""} />
                  {t(`unlock.${state}` as never)}
                </Badge>
                {r?.region && (
                  <span style={{ fontSize: 13 }}>
                    {r.region.length === 2 ? flag(r.region) : ""} {r.region}
                  </span>
                )}
              </div>
              <div className="faint" style={{ fontSize: 11.5, minHeight: 16 }}>
                {r?.detail}
                {r?.detail && r?.at && " · "}
                {r?.at && relative(r.at, lang)}
              </div>
            </div>
          );
        })}
      </div>
    </section>
  );
}
