import {
  Activity,
  ArrowDown,
  ArrowUp,
  Cpu,
  Eye,
  EyeOff,
  FileStack,
  Globe2,
  LayoutGrid,
  MapPin,
  Monitor,
  Network,
  RefreshCw,
  Route,
  ShieldCheck,
  Timer,
  Waypoints,
  Zap,
} from "lucide-react";
import { Channel } from "mygo-runtime";
import { useEffect, useMemo, useState } from "react";
import { PageHeader } from "../components/Page";
import { TrafficGraph } from "../components/TrafficGraph";
import { bytes, dateOnly, duration, flag, percent, rate, relative } from "../lib/format";
import { useAsync, useNow } from "../lib/hooks";
import { useT } from "../lib/i18n";
import { patchSettings, run, toastError, useApp, useTraffic } from "../lib/store";
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
  type SiteResult,
} from "../mygo";
import { Badge, Button, Card, Delay, Dialog, Empty, Progress, Segmented, Select, Spinner, Switch } from "../ui";

function ProfileCard() {
  const t = useT();
  const lang = useApp((s) => s.lang);
  const profiles = useApp((s) => s.profiles);
  const navigate = useApp((s) => s.navigate);
  const [busy, setBusy] = useState(false);
  useNow();
  const cur = profiles?.items.find((p) => p.uid === profiles.current);
  if (!cur)
    return (
      <Card title={t("home.profile")} icon={<FileStack size={15} />}>
        <Empty title={t("home.noProfile")}>
          <Button variant="primary" onClick={() => navigate("profiles")}>
            {t("home.importProfile")}
          </Button>
        </Empty>
      </Card>
    );
  const used = (cur.usage?.upload ?? 0) + (cur.usage?.download ?? 0);
  const total = cur.usage?.total ?? 0;
  const pct = percent(used, total);
  const next = profiles?.nextUpdates[cur.uid];
  return (
    <Card
      title={t("home.profile")}
      icon={<FileStack size={15} />}
      actions={
        cur.type === "remote" && (
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
        )
      }
    >
      <div className="col" style={{ gap: 10 }}>
        <div className="row">
          <div className="grow">
            <div style={{ fontWeight: 650, fontSize: 15 }} className="ellipsis">
              {cur.name}
            </div>
            <div className="muted" style={{ fontSize: 12 }}>
              {cur.type === "remote" ? t("profiles.updatedAgo", { when: relative(cur.updated * 1000, lang) }) : t("profiles.local")}
              {cur.proxies > 0 && ` · ${t("profiles.nodes", { n: cur.proxies })}`}
            </div>
          </div>
          <Button size="sm" onClick={() => navigate("profiles")}>
            {t("home.switch")}
          </Button>
        </div>
        {total > 0 && (
          <div className="col" style={{ gap: 5 }}>
            <Progress value={pct} tone={pct > 90 ? "danger" : pct > 75 ? "warning" : undefined} />
            <div className="row" style={{ fontSize: 12 }}>
              <span className="tnum">
                {bytes(used)} / {bytes(total)}
              </span>
              <span className="spacer" />
              {cur.usage?.expire ? (
                <span className="muted">{t("profiles.expires", { date: dateOnly(cur.usage.expire) })}</span>
              ) : (
                <span className="muted">{t("profiles.noExpiry")}</span>
              )}
            </div>
          </div>
        )}
        {cur.lastError && <div style={{ color: "var(--danger)", fontSize: 12 }}>{cur.lastError}</div>}
        {next && <div className="faint" style={{ fontSize: 11.5 }}>{t("profiles.nextUpdate", { when: relative(next, lang) })}</div>}
      </div>
    </Card>
  );
}

function ProxyCard() {
  const t = useT();
  const runtime = useApp((s) => s.runtime);
  const selection = useApp((s) => s.selection);
  const ready = useApp((s) => s.state?.core.status === "running");
  const navigate = useApp((s) => s.navigate);
  const { data, reload } = useAsync<ProxiesView | undefined>(() => (ready ? Proxies.view() : Promise.resolve(undefined)), [runtime, selection, ready]);
  const [group, setGroup] = useState("");
  const [testing, setTesting] = useState(false);
  const selectors = useMemo(() => (data?.groups ?? []).filter((g) => !g.hidden && (g.type === "Selector" || g.type === "URLTest" || g.type === "Fallback")), [data]);
  const g = selectors.find((x) => x.name === group) ?? selectors[0];
  const cur = g?.all.find((m) => m.name === g.now);
  return (
    <Card
      title={t("home.currentProxy")}
      icon={<Globe2 size={15} />}
      actions={
        g && (
          <Button
            size="sm"
            variant="ghost"
            icon={<Zap size={14} />}
            loading={testing}
            tip={t("proxies.testGroup")}
            onClick={async () => {
              setTesting(true);
              await run(() => Proxies.groupDelay(g.name, ""), t("proxies.testFailed"));
              setTesting(false);
              void reload();
            }}
          />
        )
      }
    >
      {!g ? (
        <Empty title={ready ? t("home.noGroups") : t("common.coreNotRunning")} />
      ) : (
        <div className="col" style={{ gap: 10 }}>
          <Select value={g.name} onChange={setGroup} options={selectors.map((x) => ({ value: x.name, label: `${x.name} · ${x.type}` }))} />
          <div className="row" style={{ padding: "8px 10px", background: "var(--surface-2)", borderRadius: 9, border: "1px solid var(--border)" }}>
            <div className="grow">
              <div style={{ fontWeight: 600 }} className="ellipsis">
                {g.now || "—"}
              </div>
              <div className="muted" style={{ fontSize: 11.5 }}>
                {cur?.type ?? ""}
                {cur?.udp ? " · UDP" : ""}
              </div>
            </div>
            <Delay value={cur?.delay} />
          </div>
          {g.type === "Selector" && (
            <Select
              value={g.now}
              onChange={(name) =>
                run(async () => {
                  await Proxies.select(g.name, name);
                  await reload();
                }, t("proxies.selectFailed"))
              }
              options={g.all.map((m) => ({ value: m.name, label: m.delay > 0 ? `${m.name}  (${m.delay} ms)` : m.name }))}
            />
          )}
          <Button size="sm" variant="ghost" onClick={() => navigate("proxies")} style={{ alignSelf: "flex-start" }}>
            {t("home.allProxies")} →
          </Button>
        </div>
      )}
    </Card>
  );
}

function NetworkCard() {
  const t = useT();
  const s = useApp((st) => st.state);
  const settings = useApp((st) => st.settings);
  const [busy, setBusy] = useState<string>("");
  if (!s || !settings) return null;
  const toggle = async (key: "sys" | "tun", on: boolean) => {
    setBusy(key);
    await run(() => patchSettings(key === "sys" ? { systemProxy: { enabled: on } } : { tun: { enabled: on } }), t("common.failed"));
    setBusy("");
  };
  return (
    <Card title={t("home.network")} icon={<Network size={15} />}>
      <div className="col" style={{ gap: 0 }}>
        <div className="row" style={{ padding: "6px 0" }}>
          <div className="grow">
            <div style={{ fontWeight: 600 }}>{t("settings.systemProxy")}</div>
            <div className="muted" style={{ fontSize: 12 }}>
              {s.systemProxyError ? <span style={{ color: "var(--danger)" }}>{s.systemProxyError}</span> : s.systemProxy ? `${settings.systemProxy.host}:${s.mixedPort}${settings.systemProxy.pac ? " · PAC" : ""}` : t("home.sysproxyOff")}
            </div>
          </div>
          <Switch checked={settings.systemProxy.enabled} busy={busy === "sys"} onChange={(v) => toggle("sys", v)} label={t("settings.systemProxy")} />
        </div>
        <div style={{ height: 1, background: "var(--border)", margin: "8px 0" }} />
        <div className="row" style={{ padding: "6px 0" }}>
          <div className="grow">
            <div style={{ fontWeight: 600 }}>{t("settings.tun")}</div>
            <div className="muted" style={{ fontSize: 12 }}>
              {s.tun ? `${t("home.tunOn")} · ${settings.tun.stack}` : s.tunAvailable ? t("home.tunOff") : t("home.tunNeedsService")}
            </div>
          </div>
          {!s.tunAvailable && s.service.supported ? (
            <Button size="sm" variant="primary" icon={<ShieldCheck size={14} />} loading={busy === "svc"} onClick={async () => {
              setBusy("svc");
              await run(() => System.installService(), t("service.installFailed"), t("service.installed"));
              setBusy("");
            }}>
              {t("service.install")}
            </Button>
          ) : (
            <Switch checked={settings.tun.enabled} busy={busy === "tun"} disabled={!s.tunAvailable} onChange={(v) => toggle("tun", v)} label={t("settings.tun")} />
          )}
        </div>
      </div>
    </Card>
  );
}

function ModeCard() {
  const t = useT();
  const mode = useApp((s) => s.settings?.clash.mode ?? "rule");
  const desc: Record<string, string> = { rule: t("mode.ruleDesc"), global: t("mode.globalDesc"), direct: t("mode.directDesc") };
  return (
    <Card title={t("home.mode")} icon={<Route size={15} />}>
      <div className="col" style={{ gap: 12 }}>
        <Segmented
          full
          value={mode}
          onChange={(m) => run(() => Core.setMode(m), t("common.failed"))}
          options={[
            { value: "rule", label: t("mode.rule") },
            { value: "global", label: t("mode.global") },
            { value: "direct", label: t("mode.direct") },
          ]}
        />
        <div className="muted" style={{ fontSize: 12.5 }}>
          {desc[mode]}
        </div>
      </div>
    </Card>
  );
}

function TrafficCard() {
  const t = useT();
  const tr = useTraffic();
  const showMem = useApp((s) => s.settings?.ui.memoryUsage ?? true);
  const [conns, setConns] = useState(0);
  useEffect(() => {
    if (useApp.getState().preview) return;
    const ch = new Channel<{ connections: unknown[] }>((c) => setConns(c.connections?.length ?? 0));
    Connections.stream(ch as never).catch(() => {});
    return () => ch.close();
  }, []);
  return (
    <Card title={t("home.traffic")} icon={<Activity size={15} />}>
      <div className="col" style={{ gap: 12 }}>
        <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 10 }}>
          <div className="stat">
            <span className="stat-label row" style={{ gap: 4 }}>
              <ArrowUp size={12} color="var(--accent-2)" /> {t("common.upload")}
            </span>
            <span className="stat-value" style={{ fontSize: 17 }}>
              {rate(tr.now.up)}
            </span>
          </div>
          <div className="stat">
            <span className="stat-label row" style={{ gap: 4 }}>
              <ArrowDown size={12} color="var(--accent)" /> {t("common.download")}
            </span>
            <span className="stat-value" style={{ fontSize: 17 }}>
              {rate(tr.now.down)}
            </span>
          </div>
          <div className="stat">
            <span className="stat-label">{t("home.totals")}</span>
            <span className="tnum" style={{ fontWeight: 600 }}>
              ↑ {bytes(tr.now.upTotal)}
              <br />↓ {bytes(tr.now.downTotal)}
            </span>
          </div>
          <div className="stat">
            <span className="stat-label">{showMem ? t("home.memory") : t("home.connections")}</span>
            <span className="tnum" style={{ fontWeight: 600 }}>
              {showMem ? bytes(tr.memory) : conns}
              {showMem && (
                <>
                  <br />
                  <span className="muted" style={{ fontWeight: 500 }}>
                    {t("home.connCount", { n: conns })}
                  </span>
                </>
              )}
            </span>
          </div>
        </div>
        <TrafficGraph up={tr.up} down={tr.down} height={128} />
      </div>
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
  return (
    <Card
      title="Tailscale"
      icon={<Waypoints size={15} />}
      actions={
        <Button size="sm" variant="ghost" onClick={() => navigate("tailscale")}>
          {t("common.open")} →
        </Button>
      }
    >
      {mode === "off" ? (
        <div className="col" style={{ gap: 10 }}>
          <div className="muted" style={{ fontSize: 12.5 }}>
            {t("ts.offHint")}
          </div>
          <Button variant="primary" size="sm" style={{ alignSelf: "flex-start" }} onClick={() => navigate("tailscale")}>
            {t("ts.setUp")}
          </Button>
        </div>
      ) : (
        <dl className="kv">
          <dt>{t("ts.status")}</dt>
          <dd>
            <Badge tone={state === "Running" ? "success" : state === "NeedsLogin" ? "warning" : undefined}>{t(`ts.state.${state}` as never) || state}</Badge>
          </dd>
          <dt>{t("ts.address")}</dt>
          <dd className="mono selectable">{ts?.self?.tailscaleIps[0] ?? "—"}</dd>
          <dt>{t("ts.peers")}</dt>
          <dd>{t("ts.peersOnline", { online, total: ts?.peers.length ?? 0 })}</dd>
          <dt>{t("ts.exitNode")}</dt>
          <dd>{exit?.hostName ?? t("common.none")}</dd>
        </dl>
      )}
    </Card>
  );
}

function TestCard() {
  const t = useT();
  const [results, setResults] = useState<Record<string, SiteResult | "pending">>({});
  const { data: sites } = useAsync(() => Tools.sites(), []);
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
  return (
    <Card
      title={t("home.sites")}
      icon={<Timer size={15} />}
      actions={<Button size="sm" variant="ghost" icon={<RefreshCw size={14} />} loading={running} onClick={test} tip={t("common.retest")} />}
    >
      <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 8 }}>
        {(sites ?? []).map((s) => {
          const r = results[s.id];
          return (
            <div key={s.id} className="row" style={{ padding: "8px 10px", borderRadius: 9, background: "var(--surface-2)", border: "1px solid var(--border)" }}>
              <span className="grow ellipsis" style={{ fontWeight: 560 }}>
                {s.name}
              </span>
              {r === "pending" ? <Spinner size={13} /> : r ? <Delay value={r.error ? 0 : r.delayMs} /> : <span className="faint">—</span>}
            </div>
          );
        })}
      </div>
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
        <div className="muted" style={{ fontSize: 12.5 }}>
          {t("home.ipFailed")}
          <div className="faint" style={{ fontSize: 11.5, marginTop: 4 }}>
            {error}
          </div>
        </div>
      ) : !info ? (
        <div className="skeleton" style={{ height: 70 }} />
      ) : (
        <div className="row" style={{ gap: 14, alignItems: "flex-start" }}>
          <div style={{ fontSize: 34, lineHeight: 1 }}>{flag(info.countryCode)}</div>
          <dl className="kv grow">
            <dt>IP</dt>
            <dd className="mono selectable" style={{ filter: shown ? "none" : "blur(5px)", transition: "filter .2s" }}>
              {info.ip}
            </dd>
            <dt>{t("home.location")}</dt>
            <dd>{[info.city, info.region, info.country].filter(Boolean).join(", ") || "—"}</dd>
            <dt>{t("home.isp")}</dt>
            <dd>
              {info.isp || "—"}
              {info.asn ? ` · AS${info.asn}` : ""}
            </dd>
          </dl>
        </div>
      )}
    </Card>
  );
}

function CoreCard() {
  const t = useT();
  const s = useApp((st) => st.state);
  const info = useApp((st) => st.info);
  useNow(10000);
  if (!s) return null;
  const c = s.core;
  const tone = c.status === "running" ? "success" : c.status === "error" ? "danger" : "warning";
  return (
    <Card
      title={t("home.core")}
      icon={<Cpu size={15} />}
      actions={<Button size="sm" variant="ghost" icon={<RefreshCw size={14} />} onClick={() => run(() => Core.restart(), t("common.failed"))} tip={t("core.restart")} />}
    >
      <dl className="kv">
        <dt>{t("core.status")}</dt>
        <dd>
          <Badge tone={tone}>{t(`core.${c.status}` as never)}</Badge>
        </dd>
        <dt>{t("core.mode")}</dt>
        <dd>{c.mode === "service" ? t("core.service") : t("core.sidecar")}</dd>
        <dt>mihomo</dt>
        <dd className="mono">{c.coreVersion || info?.coreVersion || "—"}</dd>
        <dt>{t("core.uptime")}</dt>
        <dd>{c.startedAt ? duration(Date.now() - new Date(c.startedAt).getTime()) : "—"}</dd>
      </dl>
      {(c.error || s.configError) && <div style={{ color: "var(--danger)", fontSize: 12, marginTop: 8 }}>{s.configError || c.error}</div>}
    </Card>
  );
}

function SystemCard() {
  const t = useT();
  const info = useApp((s) => s.info);
  const { data: autostart, reload } = useAsync(() => App.autoLaunch(), []);
  if (!info) return null;
  return (
    <Card title={t("home.system")} icon={<Monitor size={15} />}>
      <dl className="kv">
        <dt>{t("home.os")}</dt>
        <dd>
          {info.os} / {info.arch}
        </dd>
        <dt>{t("home.hostname")}</dt>
        <dd>{info.hostname}</dd>
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
        <dt>{t("home.version")}</dt>
        <dd>v{info.version}</dd>
      </dl>
    </Card>
  );
}

const CARDS: Record<string, { span: string; render: () => React.ReactNode }> = {
  profile: { span: "span-6", render: () => <ProfileCard /> },
  proxy: { span: "span-6", render: () => <ProxyCard /> },
  network: { span: "span-6", render: () => <NetworkCard /> },
  mode: { span: "span-6", render: () => <ModeCard /> },
  traffic: { span: "span-8", render: () => <TrafficCard /> },
  tailscale: { span: "span-4", render: () => <TailscaleCard /> },
  test: { span: "span-6", render: () => <TestCard /> },
  ip: { span: "span-6", render: () => <IPCard /> },
  core: { span: "span-6", render: () => <CoreCard /> },
  system: { span: "span-6", render: () => <SystemCard /> },
};

function Customize({ open, onClose }: { open: boolean; onClose: () => void }) {
  const t = useT();
  const cards = useApp((s) => s.settings?.ui.homeCards ?? []);
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
  const cards = useApp((s) => s.settings?.ui.homeCards ?? []);
  const state = useApp((s) => s.state);
  const navigate = useApp((s) => s.navigate);
  const [editing, setEditing] = useState(false);
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
            <span className="grow">
              <b>{t("home.configError")}</b> {state.configError}
            </span>
            <Button size="sm" onClick={() => navigate("profiles")}>
              {t("nav.profiles")}
            </Button>
          </div>
        )}
        <div className="home-grid">
          {cards
            .filter((c) => c.visible && CARDS[c.id])
            .map((c) => (
              <div key={c.id} className={CARDS[c.id]!.span}>
                {CARDS[c.id]!.render()}
              </div>
            ))}
        </div>
      </div>
      <Customize open={editing} onClose={() => setEditing(false)} />
    </>
  );
}
