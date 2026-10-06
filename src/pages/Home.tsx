import {
  Activity,
  AlertTriangle,
  ArrowDown,
  ArrowUp,
  ChevronRight,
  Cpu,
  Eye,
  EyeOff,
  FileStack,
  Globe2,
  Layers,
  LayoutGrid,
  Loader2,
  MapPin,
  Monitor,
  MonitorSmartphone,
  Power,
  RefreshCw,
  ShieldCheck,
  Timer,
  Waypoints,
  Zap,
} from "lucide-react";
import { Channel } from "mygo-runtime";
import { type ReactNode, useEffect, useLayoutEffect, useMemo, useState } from "react";
import { PageHeader } from "../components/Page";
import { TrafficGraph } from "../components/TrafficGraph";
import { bytes, dateOnly, duration, flag, percent, rate, relative } from "../lib/format";
import { density, pack } from "../lib/homeLayout";
import { useAsync, useNow, useWidth } from "../lib/hooks";
import { useT } from "../lib/i18n";
import { type Page, patchSettings, run, toastError, useApp, useTraffic } from "../lib/store";
import {
  App,
  Connections,
  Core,
  Profiles,
  Proxies,
  System,
  Tools,
  type HomeCard,
  type IPInfo,
  type ProxiesView,
  type ProxyGroup,
  type SiteResult,
} from "../mygo";
import { Badge, Button, Card, Delay, Dialog, Empty, Progress, Segmented, Select, Spinner, Switch } from "../ui";

// ---------- Pieces ----------

/** GoTo is a card's way to its page. */
function GoTo({ page, tab, label }: { page: Page; tab?: string; label: string }) {
  const navigate = useApp((s) => s.navigate);
  return (
    <button type="button" className="card-link" onClick={() => navigate(page, tab)}>
      {label}
      <ChevronRight size={13} />
    </button>
  );
}

/** Lead is the first line of a card: what the card is about, in a word. */
function Lead({ title, sub, extra }: { title: ReactNode; sub?: ReactNode; extra?: ReactNode }) {
  return (
    <div className="lead">
      <div className="grow">
        <div className="lead-title ellipsis">{title}</div>
        {sub && <div className="lead-sub ellipsis">{sub}</div>}
      </div>
      {extra}
    </div>
  );
}

// ---------- Cards ----------

function ControlCard() {
  const t = useT();
  const s = useApp((st) => st.state);
  const settings = useApp((st) => st.settings);
  const [busy, setBusy] = useState("");
  if (!s || !settings) return null;
  const mode = settings.clash.mode;
  const toggle = async (key: "sys" | "tun", on: boolean) => {
    setBusy(key);
    await run(() => patchSettings(key === "sys" ? { systemProxy: { enabled: on } } : { tun: { enabled: on } }), t("common.failed"));
    setBusy("");
  };
  const install = async () => {
    setBusy("svc");
    await run(() => System.installService(), t("service.installFailed"), t("service.installed"));
    setBusy("");
  };

  let tone: "on" | "off" | "warn" | "error";
  let title: string;
  let icon: ReactNode;
  let sub = t("home.modeLine", { mode: t(`mode.${mode}` as never), desc: t(`mode.${mode}Desc` as never) });
  if (s.core.status === "error") {
    tone = "error";
    title = t("home.state.error");
    icon = <AlertTriangle size={20} />;
    sub = s.core.error || sub;
  } else if (s.core.status !== "running") {
    tone = "warn";
    title = s.core.status === "starting" ? t("home.state.starting") : t("home.state.stopped");
    icon = s.core.status === "starting" ? <Loader2 size={20} className="spin" /> : <Power size={20} />;
  } else if (s.tun || s.systemProxy) {
    tone = "on";
    title = s.tun ? t("home.state.tun") : t("home.state.sysproxy");
    icon = <Zap size={20} />;
  } else {
    tone = "off";
    title = t("home.state.off");
    icon = <Power size={20} />;
  }

  const sysDesc = s.systemProxyError ? (
    <span style={{ color: "var(--danger)" }}>{s.systemProxyError}</span>
  ) : s.systemProxy ? (
    `${settings.systemProxy.host}:${s.mixedPort}${settings.systemProxy.pac ? " · PAC" : ""}`
  ) : (
    t("home.sysproxyOff")
  );
  const tunDesc = s.tun ? `${t("home.tunOn")} · ${settings.tun.stack}` : s.tunAvailable ? t("home.tunOff") : t("home.tunNeedsService");
  const needsService = !s.tunAvailable && s.service.supported;
  return (
    <section className={`card control-card ${tone}`}>
      <div className="control-top">
        <div className={`status-orb ${tone}`}>{icon}</div>
        <div className="grow">
          <div className="control-title">{title}</div>
          <div className="control-sub">{sub}</div>
        </div>
        <Segmented
          value={mode}
          onChange={(m) => run(() => Core.setMode(m), t("common.failed"))}
          options={[
            { value: "rule", label: t("mode.rule") },
            { value: "global", label: t("mode.global") },
            { value: "direct", label: t("mode.direct") },
          ]}
        />
      </div>
      <div className="control-tiles">
        <ToggleTile
          icon={<MonitorSmartphone size={17} />}
          title={t("settings.systemProxy")}
          desc={sysDesc}
          on={s.systemProxy}
          error={!!s.systemProxyError}
          onToggle={busy ? undefined : () => toggle("sys", !settings.systemProxy.enabled)}
        >
          <Switch checked={settings.systemProxy.enabled} busy={busy === "sys"} onChange={(v) => toggle("sys", v)} label={t("settings.systemProxy")} />
        </ToggleTile>
        <ToggleTile
          icon={<Layers size={17} />}
          title={t("settings.tun")}
          desc={tunDesc}
          on={s.tun}
          onToggle={busy || needsService || !s.tunAvailable ? undefined : () => toggle("tun", !settings.tun.enabled)}
        >
          {needsService ? (
            <Button size="sm" variant="primary" icon={<ShieldCheck size={14} />} loading={busy === "svc"} onClick={install}>
              {t("service.install")}
            </Button>
          ) : (
            <Switch checked={settings.tun.enabled} busy={busy === "tun"} disabled={!s.tunAvailable} onChange={(v) => toggle("tun", v)} label={t("settings.tun")} />
          )}
        </ToggleTile>
      </div>
    </section>
  );
}

function ToggleTile({
  icon,
  title,
  desc,
  on,
  error,
  onToggle,
  children,
}: {
  icon: ReactNode;
  title: string;
  desc: ReactNode;
  on: boolean;
  error?: boolean;
  onToggle?: () => void;
  children: ReactNode;
}) {
  return (
    <div className={`toggle-tile${on ? " on" : ""}${error ? " error" : ""}${onToggle ? " clickable" : ""}`} onClick={onToggle}>
      <div className="tile-icon">{icon}</div>
      <div className="grow">
        <div className="tile-title">{title}</div>
        <div className="tile-desc">{desc}</div>
      </div>
      <div className="row" onClick={(e) => e.stopPropagation()}>
        {children}
      </div>
    </div>
  );
}

function TrafficCard() {
  const t = useT();
  const tr = useTraffic();
  const showMem = useApp((s) => s.settings?.ui.memoryUsage ?? true);
  const [conns, setConns] = useState(0);
  const [ref, width] = useWidth<HTMLDivElement>();
  useEffect(() => {
    if (useApp.getState().preview) return;
    const ch = new Channel<{ connections: unknown[] }>((c) => setConns(c.connections?.length ?? 0));
    Connections.stream(ch as never).catch(() => {});
    return () => ch.close();
  }, []);
  const peak = Math.max(...tr.up, ...tr.down);
  return (
    <Card title={t("home.traffic")} icon={<Activity size={15} />} actions={<GoTo page="connections" label={t("nav.connections")} />}>
      <div ref={ref} className={`tstats${width > 0 && width < 460 ? " two" : ""}`}>
        <div className="tstat">
          <span className="tstat-label">
            <ArrowUp size={12} color="var(--graph-up, var(--accent-2))" /> {t("common.upload")}
          </span>
          <span className="tstat-value">{rate(tr.now.up)}</span>
          <span className="tstat-sub">{t("home.totalOf", { n: bytes(tr.now.upTotal) })}</span>
        </div>
        <div className="tstat">
          <span className="tstat-label">
            <ArrowDown size={12} color="var(--graph-down, var(--accent))" /> {t("common.download")}
          </span>
          <span className="tstat-value">{rate(tr.now.down)}</span>
          <span className="tstat-sub">{t("home.totalOf", { n: bytes(tr.now.downTotal) })}</span>
        </div>
        <div className="tstat">
          <span className="tstat-label">{t("home.connections")}</span>
          <span className="tstat-value">{conns}</span>
          <span className="tstat-sub">{t("home.connSub")}</span>
        </div>
        {showMem && (
          <div className="tstat">
            <span className="tstat-label">{t("home.memory")}</span>
            <span className="tstat-value">{bytes(tr.memory)}</span>
            <span className="tstat-sub">mihomo</span>
          </div>
        )}
      </div>
      <div className="chart">
        <div className="chart-canvas">
          <TrafficGraph up={tr.up} down={tr.down} />
        </div>
        <span className="chart-note">
          {t("home.window")}
          {peak > 0 && ` · ${t("home.peak", { rate: rate(peak) })}`}
        </span>
      </div>
    </Card>
  );
}

/** chain follows a group's selection down to the proxy that carries traffic. */
function chain(g: ProxyGroup, groups: Map<string, ProxyGroup>, now: string): string[] {
  const out = [g.name];
  let name = now;
  for (let i = 0; name && i < 8; i++) {
    out.push(name);
    const next = groups.get(name);
    if (!next || !next.now) break;
    name = next.now;
  }
  return out;
}

function ProxyCard() {
  const t = useT();
  const runtime = useApp((s) => s.runtime);
  const selection = useApp((s) => s.selection);
  const mode = useApp((s) => s.settings?.clash.mode ?? "rule");
  const ready = useApp((s) => s.state?.core.status === "running");
  const { data, reload } = useAsync<ProxiesView | undefined>(() => (ready ? Proxies.view() : Promise.resolve(undefined)), [runtime, selection, ready, mode]);
  const [group, setGroup] = useState(() => {
    try {
      return localStorage.getItem("home.group") ?? "";
    } catch {
      return "";
    }
  });
  const [testing, setTesting] = useState(false);
  const [delays, setDelays] = useState<Record<string, number>>({});
  const [pending, setPending] = useState<string | null>(null);
  const [listRef, listWidth, list] = useWidth<HTMLDivElement>();
  const [more, setMore] = useState(false);
  const measure = () => {
    const el = list.current;
    setMore(!!el && el.scrollHeight - el.scrollTop - el.clientHeight > 2);
  };
  const groups = useMemo(() => {
    const all = (data?.groups ?? []).filter((g) => !g.hidden && (g.type === "Selector" || g.type === "URLTest" || g.type === "Fallback"));
    if (mode === "global" && data?.global) return [data.global, ...all.filter((g) => g.name !== data.global!.name)];
    return all;
  }, [data, mode]);
  const byName = useMemo(() => new Map([...(data?.groups ?? []), ...(data?.global ? [data.global] : [])].map((g) => [g.name, g])), [data]);
  const g = groups.find((x) => x.name === group) ?? groups[0];
  useEffect(() => setPending(null), [g?.now, g?.name]);
  const now = pending ?? g?.now ?? "";
  // The selection stays in view when the group or the selection changes.
  useLayoutEffect(() => {
    const el = list.current?.querySelector<HTMLElement>(".node-row.active");
    const box = list.current;
    if (!el || !box) return;
    if (el.offsetTop < box.scrollTop || el.offsetTop + el.offsetHeight > box.scrollTop + box.clientHeight) {
      box.scrollTop = el.offsetTop - box.clientHeight / 2 + el.offsetHeight / 2;
    }
    measure();
  }, [g?.name, now, g?.all.length]);
  const choose = (name: string) => {
    setGroup(name);
    setDelays({});
    try {
      localStorage.setItem("home.group", name);
    } catch {
      // only a convenience
    }
  };
  const select = async (name: string) => {
    if (!g || name === now) return;
    setPending(name);
    const ok = await run(() => Proxies.select(g.name, name).then(() => true), t("proxies.selectFailed"));
    if (!ok) setPending(null);
    void reload();
  };
  const testAll = async () => {
    if (!g) return;
    setTesting(true);
    const res = await run(() => Proxies.groupDelay(g.name, g.testUrl ?? ""), t("proxies.testFailed"));
    if (res) setDelays(Object.fromEntries(g.all.map((m) => [m.name, res[m.name] ?? 0])));
    setTesting(false);
    void reload();
  };
  const path = g ? chain(g, byName, now) : [];
  const exit = path[path.length - 1] ?? "";
  const exitItem = g?.all.find((m) => m.name === now);
  return (
    <Card
      title={t("home.currentProxy")}
      icon={<Globe2 size={15} />}
      actions={
        <>
          {g && mode !== "direct" && <Button size="sm" variant="ghost" icon={<Zap size={14} />} loading={testing} tip={t("proxies.testGroup")} onClick={testAll} />}
          <GoTo page="proxies" label={t("nav.proxies")} />
        </>
      }
    >
      {mode === "direct" ? (
        <div className="card-empty">
          <Lead title={t("mode.direct")} sub={t("mode.directDesc")} />
        </div>
      ) : !g ? (
        <div className="card-empty">
          <Empty title={ready ? t("home.noGroups") : t("common.coreNotRunning")} />
        </div>
      ) : (
        <>
          <Lead
            title={exit || "—"}
            sub={path.length > 2 ? path.slice(0, -1).join(" → ") : `${g.name} · ${exitItem?.type ?? g.type}`}
            extra={<Delay value={delays[now] ?? exitItem?.delay} loading={testing} />}
          />
          {groups.length > 1 && <Select value={g.name} onChange={choose} options={groups.map((x) => ({ value: x.name, label: `${x.name} · ${x.type}` }))} />}
          <div className={`node-list${more ? " more" : ""}`} ref={listRef} role="listbox" aria-label={g.name} onScroll={measure}>
            {g.all.map((m) => (
              <button
                key={m.name}
                type="button"
                role="option"
                aria-selected={m.name === now}
                className={`node-row${m.name === now ? " active" : ""}`}
                onClick={() => select(m.name)}
                title={m.name}
              >
                <span className="grow ellipsis">{m.name}</span>
                {listWidth >= 340 && <span className="node-type">{m.type}</span>}
                <Delay value={delays[m.name] ?? m.delay} loading={testing} />
              </button>
            ))}
          </div>
        </>
      )}
    </Card>
  );
}

function ProfileCard() {
  const t = useT();
  const lang = useApp((s) => s.lang);
  const profiles = useApp((s) => s.profiles);
  const navigate = useApp((s) => s.navigate);
  const [busy, setBusy] = useState(false);
  useNow();
  const cur = profiles?.items.find((p) => p.uid === profiles.current);
  const link = <GoTo page="profiles" label={t("home.manage")} />;
  if (!cur)
    return (
      <Card title={t("home.profile")} icon={<FileStack size={15} />} actions={link}>
        <div className="card-empty">
          <Empty title={t("home.noProfile")}>
            <Button variant="primary" size="sm" onClick={() => navigate("profiles")}>
              {t("home.importProfile")}
            </Button>
          </Empty>
        </div>
      </Card>
    );
  const remote = cur.type === "remote";
  const used = (cur.usage?.upload ?? 0) + (cur.usage?.download ?? 0);
  const total = cur.usage?.total ?? 0;
  const pct = percent(used, total);
  const next = profiles?.nextUpdates[cur.uid];
  return (
    <Card
      title={t("home.profile")}
      icon={<FileStack size={15} />}
      actions={
        <>
          {remote && (
            <Button
              size="sm"
              variant="ghost"
              icon={<RefreshCw size={14} />}
              loading={busy}
              tip={t("profiles.update")}
              onClick={async () => {
                setBusy(true);
                await run(() => Profiles.update(cur.uid, null), t("profiles.updateFailed"), t("profiles.updated"));
                setBusy(false);
              }}
            />
          )}
          {link}
        </>
      }
    >
      <Lead
        title={cur.name}
        sub={remote ? `${t("profiles.remote")} · ${t("profiles.nodes", { n: cur.proxies })}` : t("profiles.local")}
        extra={total > 0 ? <span className="lead-figure">{Math.round(pct)}%</span> : undefined}
      />
      {total > 0 && <Progress value={pct} tone={pct > 90 ? "danger" : pct > 75 ? "warning" : undefined} />}
      {remote ? (
        <dl className="kv">
          {total > 0 && (
            <>
              <dt>{t("home.used")}</dt>
              <dd>
                {bytes(used)} / {bytes(total)}
              </dd>
            </>
          )}
          <dt>{t("home.expire")}</dt>
          <dd>{cur.usage?.expire ? dateOnly(cur.usage.expire) : t("profiles.noExpiry")}</dd>
          <dt>{t("home.updated")}</dt>
          <dd>{relative(cur.updated * 1000, lang)}</dd>
          <dt>{t("home.nextUpdate")}</dt>
          <dd>{next ? relative(next, lang) : t("common.off")}</dd>
        </dl>
      ) : (
        <dl className="kv">
          <dt>{t("home.nodes")}</dt>
          <dd>{cur.proxies}</dd>
          <dt>{t("home.groups")}</dt>
          <dd>{cur.groups}</dd>
          <dt>{t("home.updated")}</dt>
          <dd>{relative(cur.updated * 1000, lang)}</dd>
        </dl>
      )}
      {cur.lastError && <div className="card-error ellipsis" title={cur.lastError}>{cur.lastError}</div>}
    </Card>
  );
}

function IPCard() {
  const t = useT();
  const [info, setInfo] = useState<IPInfo | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [shown, setShown] = useState(false);
  const runtime = useApp((s) => s.runtime);
  const load = async () => {
    setLoading(true);
    setError("");
    try {
      setInfo(await Tools.ipInfo(false));
    } catch (e) {
      setError(String(e instanceof Error ? e.message : e));
    }
    setLoading(false);
  };
  useEffect(() => {
    if (!useApp.getState().preview) void load();
  }, [runtime]);
  const place = info ? [...new Set([info.city, info.region].filter(Boolean))].join(" · ") : "";
  return (
    <Card
      title={t("home.ip")}
      icon={<MapPin size={15} />}
      actions={
        <>
          <Button size="sm" variant="ghost" icon={shown ? <EyeOff size={14} /> : <Eye size={14} />} onClick={() => setShown(!shown)} tip={shown ? t("common.hide") : t("common.show")} />
          <Button size="sm" variant="ghost" icon={<RefreshCw size={14} />} loading={loading} onClick={load} tip={t("common.refresh")} />
        </>
      }
    >
      {error ? (
        <Lead title={t("home.ipFailed")} sub={error} />
      ) : !info ? (
        <>
          <div className="skeleton" style={{ height: 38 }} />
          <div className="skeleton" style={{ height: 64 }} />
        </>
      ) : (
        <>
          <Lead title={`${flag(info.countryCode)}  ${info.country || "—"}`} sub={place || info.timezone || "—"} />
          <dl className="kv">
            <dt>IP</dt>
            <dd className="mono selectable" style={{ filter: shown ? "none" : "blur(5px)", transition: "filter .2s" }}>
              {info.ip}
            </dd>
            <dt>{t("home.isp")}</dt>
            <dd title={info.isp}>{info.isp || "—"}</dd>
            <dt>ASN</dt>
            <dd>{info.asn ? `AS${info.asn}` : "—"}</dd>
          </dl>
        </>
      )}
    </Card>
  );
}

function TailscaleCard() {
  const t = useT();
  const ts = useApp((s) => s.tailscale);
  const mode = useApp((s) => s.settings?.tailscale.mode ?? "off");
  const navigate = useApp((s) => s.navigate);
  const online = ts?.peers.filter((p) => p.online).length ?? 0;
  const exit = ts?.peers.find((p) => p.id === ts.exitNodeId);
  const state = ts?.backendState ?? "NoState";
  const tone = state === "Running" ? "success" : state === "NeedsLogin" || state === "NeedsMachineAuth" ? "warning" : "";
  return (
    <Card title="Tailscale" icon={<Waypoints size={15} />} actions={<GoTo page="tailscale" label={t("common.open")} />}>
      {mode === "off" ? (
        <>
          <Lead
            title={t("ts.state.NoState")}
            sub={t("ts.offHint")}
            extra={
              <Button variant="primary" size="sm" onClick={() => navigate("tailscale")}>
                {t("ts.setUp")}
              </Button>
            }
          />
        </>
      ) : (
        <>
          <Lead
            title={
              <span className="row" style={{ gap: 8 }}>
                <span className={`dot ${tone}`} />
                {t(`ts.state.${state}` as never) || state}
              </span>
            }
            sub={ts?.tailnetName || ts?.user?.loginName || t(`ts.mode.${mode}` as never)}
            extra={
              (state === "NeedsLogin" || state === "NeedsMachineAuth") && (
                <Button variant="primary" size="sm" onClick={() => navigate("tailscale")}>
                  {t("home.signIn")}
                </Button>
              )
            }
          />
          <dl className="kv">
            <dt>{t("ts.address")}</dt>
            <dd className="mono selectable">{ts?.self?.tailscaleIps[0] ?? "—"}</dd>
            <dt>{t("ts.peers")}</dt>
            <dd>{t("ts.peersOnline", { online, total: ts?.peers.length ?? 0 })}</dd>
            <dt>{t("ts.exitNode")}</dt>
            <dd>{exit?.hostName ?? t("common.none")}</dd>
          </dl>
        </>
      )}
    </Card>
  );
}

function TestCard() {
  const t = useT();
  const [results, setResults] = useState<Record<string, SiteResult | "pending">>({});
  const { data: sites } = useAsync(() => Tools.sites(), []);
  const [ref, width] = useWidth<HTMLDivElement>();
  const running = Object.values(results).some((r) => r === "pending");
  const test = async () => {
    if (!sites) return;
    setResults(Object.fromEntries(sites.map((s) => [s.id, "pending" as const])));
    const ch = new Channel<SiteResult>((r) => setResults((old) => ({ ...old, [r.id]: r })));
    await Tools.testSites(sites, ch).catch((e) => toastError(t("common.failed"), e));
  };
  useEffect(() => {
    if (sites && !useApp.getState().preview) void test();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sites]);
  // Six sites: one row of six, rows of three or two, or a list.
  const cols = width >= 840 ? 6 : width >= 480 ? 3 : width >= 300 ? 2 : 0;
  return (
    <Card
      title={t("home.sites")}
      icon={<Timer size={15} />}
      actions={
        <>
          <Button size="sm" variant="ghost" icon={<RefreshCw size={14} />} loading={running} onClick={test} tip={t("common.retest")} />
          <GoTo page="unlock" label={t("nav.unlock")} />
        </>
      }
    >
      <div ref={ref} className={cols ? "site-grid" : "site-list"} style={cols ? { gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))` } : undefined}>
        {(sites ?? []).map((s) => {
          const r = results[s.id];
          return (
            <div key={s.id} className="site" title={r && r !== "pending" && r.error ? r.error : s.url}>
              <span className="grow ellipsis">{s.name}</span>
              {r === "pending" ? <Spinner size={13} /> : r ? <Delay value={r.error ? 0 : r.delayMs} /> : <span className="faint">—</span>}
            </div>
          );
        })}
      </div>
    </Card>
  );
}

function CoreCard() {
  const t = useT();
  const s = useApp((st) => st.state);
  const info = useApp((st) => st.info);
  const lang = useApp((st) => st.lang);
  useNow(10000);
  if (!s) return null;
  const c = s.core;
  const tone = c.status === "running" ? "success" : c.status === "error" ? "danger" : "warning";
  return (
    <Card
      title={t("home.core")}
      icon={<Cpu size={15} />}
      actions={
        <>
          <Button size="sm" variant="ghost" icon={<RefreshCw size={14} />} onClick={() => run(() => Core.restart(), t("common.failed"))} tip={t("core.restart")} />
          <GoTo page="logs" label={t("nav.logs")} />
        </>
      }
    >
      <Lead title={`mihomo ${c.coreVersion || info?.coreVersion || ""}`} sub={`${c.mode === "service" ? t("core.service") : t("core.sidecar")}${c.pid ? ` · PID ${c.pid}` : ""}`} extra={<Badge tone={tone}>{t(`core.${c.status}` as never)}</Badge>} />
      <dl className="kv">
        <dt>{t("core.uptime")}</dt>
        <dd>{c.startedAt ? duration(Date.now() - new Date(c.startedAt).getTime()) : "—"}</dd>
        <dt>{t("home.applied")}</dt>
        <dd>{s.appliedAt ? relative(s.appliedAt, lang) : "—"}</dd>
        <dt>{t("home.restarts")}</dt>
        <dd>{c.restarts}</dd>
      </dl>
      {(c.error || s.configError) && <div className="card-error ellipsis" title={s.configError || c.error}>{s.configError || c.error}</div>}
    </Card>
  );
}

function SystemCard() {
  const t = useT();
  const info = useApp((s) => s.info);
  const { data: autostart, reload } = useAsync(() => App.autoLaunch(), []);
  if (!info) return null;
  return (
    <Card title={t("home.system")} icon={<Monitor size={15} />} actions={<GoTo page="settings" tab="advanced" label={t("nav.settings")} />}>
      <Lead title={info.hostname} sub={`${info.os} / ${info.arch}`} />
      <dl className="kv">
        <dt>{t("home.version")}</dt>
        <dd>v{info.version}</dd>
        <dt>{t("settings.autoLaunch")}</dt>
        <dd>
          <Switch
            checked={!!autostart}
            onChange={(v) =>
              run(async () => {
                await App.setAutoLaunch(v);
                await reload();
              }, t("common.failed"))
            }
          />
        </dd>
        <dt>{t("home.keyring")}</dt>
        <dd>
          <Badge tone={info.keyringSecure ? "success" : "warning"}>{info.keyring}</Badge>
        </dd>
      </dl>
    </Card>
  );
}

// NO_CARDS is stable, so that selecting it does not change on every render.
const NO_CARDS: HomeCard[] = [];

const CARDS: Record<string, () => ReactNode> = {
  control: () => <ControlCard />,
  traffic: () => <TrafficCard />,
  proxy: () => <ProxyCard />,
  profile: () => <ProfileCard />,
  ip: () => <IPCard />,
  tailscale: () => <TailscaleCard />,
  test: () => <TestCard />,
  core: () => <CoreCard />,
  system: () => <SystemCard />,
};

function Customize({ open, onClose }: { open: boolean; onClose: () => void }) {
  const t = useT();
  const cards = useApp((s) => s.settings?.ui.homeCards ?? NO_CARDS);
  const [list, setList] = useState<HomeCard[]>(cards);
  useEffect(() => setList(cards), [open, cards]);
  const move = (i: number, d: number) => {
    const j = i + d;
    if (j < 0 || j >= list.length) return;
    const next = [...list];
    [next[i], next[j]] = [next[j]!, next[i]!];
    setList(next);
  };
  return (
    <Dialog
      open={open}
      onClose={onClose}
      size="narrow"
      title={t("home.customize")}
      footer={
        <>
          <Button onClick={onClose}>{t("common.cancel")}</Button>
          <Button
            variant="primary"
            onClick={async () => {
              await run(() => patchSettings({ ui: { homeCards: list } }), t("common.failed"));
              onClose();
            }}
          >
            {t("common.save")}
          </Button>
        </>
      }
    >
      <div className="col" style={{ gap: 6 }}>
        <div className="muted" style={{ fontSize: 12, marginBottom: 4 }}>
          {t("home.customizeHint")}
        </div>
        {list.map((c, i) => (
          <div key={c.id} className="row" style={{ padding: "6px 8px", border: "1px solid var(--border)", borderRadius: 8 }}>
            <Switch checked={c.visible} onChange={(v) => setList(list.map((x) => (x.id === c.id ? { ...x, visible: v } : x)))} />
            <span className="grow">{t(`home.card.${c.id}` as never)}</span>
            <Button size="sm" variant="ghost" onClick={() => move(i, -1)} disabled={i === 0}>
              ↑
            </Button>
            <Button size="sm" variant="ghost" onClick={() => move(i, 1)} disabled={i === list.length - 1}>
              ↓
            </Button>
          </div>
        ))}
      </div>
    </Dialog>
  );
}

export default function Home() {
  const t = useT();
  const cards = useApp((s) => s.settings?.ui.homeCards ?? NO_CARDS);
  const state = useApp((s) => s.state);
  const navigate = useApp((s) => s.navigate);
  const [editing, setEditing] = useState(false);
  const [ref, width] = useWidth<HTMLDivElement>();
  const ids = cards.filter((c) => c.visible && CARDS[c.id]).map((c) => c.id);
  const layout = pack(ids, density(width || 1000));
  return (
    <>
      <PageHeader title={t("nav.home")}>
        <Button size="sm" variant="ghost" icon={<LayoutGrid size={14} />} onClick={() => setEditing(true)}>
          {t("home.customize")}
        </Button>
      </PageHeader>
      <div className="page-body">
        {state?.configError && (
          <div className="banner danger">
            <AlertTriangle size={16} />
            <span className="grow">
              <b>{t("home.configError")}</b> {state.configError}
            </span>
            <Button size="sm" onClick={() => navigate("profiles")}>
              {t("nav.profiles")}
            </Button>
          </div>
        )}
        <div className="home-grid" ref={ref}>
          {layout.map(({ id, span }) => (
            <div key={id} className="home-cell" style={{ gridColumn: `span ${span}` }}>
              {CARDS[id]!()}
            </div>
          ))}
        </div>
        {ids.length === 0 && (
          <Empty art title={t("home.nothingShown")}>
            <Button size="sm" onClick={() => setEditing(true)}>
              {t("home.customize")}
            </Button>
          </Empty>
        )}
      </div>
      <Customize open={editing} onClose={() => setEditing(false)} />
    </>
  );
}
