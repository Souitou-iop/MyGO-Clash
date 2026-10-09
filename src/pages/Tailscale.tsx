import {
  Activity,
  AppWindow,
  Copy,
  ExternalLink,
  Globe,
  KeyRound,
  LogIn,
  LogOut,
  Monitor,
  Network,
  Radar,
  Router,
  Server,
  Settings2,
  Smartphone,
  Waypoints,
} from "lucide-react";
import { useMemo, useState } from "react";
import { PageHeader } from "../components/Page";
import { QRCode } from "../components/QRCode";
import { bytes, ltr, relative } from "../lib/format";
import { useAsync, useNow } from "../lib/hooks";
import { useT } from "../lib/i18n";
import { patchSettings, run, toast, toastError, useApp } from "../lib/store";
import { App, Proxies, Tailscale as API, type TailscalePeer, type TailscalePingResult, type TailscaleStatus } from "../mygo";
import { Badge, Banner, Button, Card, confirm, Empty, Field, Input, NumberInput, reflow, Row, SearchInput, Section, Segmented, Select, Switch } from "../ui";
import { CoreDown } from "../components/CoreDown";

type Mode = "off" | "embedded" | "system";

function copy(text: string, done: string) {
  void App.copyText(text).then(() => toast({ level: "success", message: done, detail: text }));
}

function osIcon(os: string) {
  const o = os.toLowerCase();
  if (o.includes("ios") || o.includes("android")) return <Smartphone size={15} />;
  if (o.includes("linux") && !o.includes("android")) return <Server size={15} />;
  return <Monitor size={15} />;
}

function stateTone(s: string): "success" | "warning" | "danger" | undefined {
  if (s === "Running") return "success";
  if (s === "NeedsLogin" || s === "NeedsMachineAuth" || s === "Starting") return "warning";
  if (s === "Stopped") return "danger";
  return undefined;
}

function Login({ status }: { status: TailscaleStatus }) {
  const t = useT();
  const [url, setUrl] = useState(status.authUrl ?? "");
  const [key, setKey] = useState("");
  const [share, setShare] = useState(true);
  const [busy, setBusy] = useState("");
  const shown = url || status.authUrl || "";
  return (
    <Card title={t("ts.signIn")} icon={<LogIn size={15} />}>
      <div className="row" style={{ gap: 24, alignItems: "flex-start", flexWrap: "wrap" }}>
        <div className="col grow" style={{ minWidth: 260, gap: 12 }}>
          <div className="muted" style={{ fontSize: 12.5 }}>
            {status.backendState === "NeedsMachineAuth" ? t("ts.needsApproval") : t("ts.signInHint")}
          </div>
          <Button
            variant="primary"
            icon={<Globe size={15} />}
            loading={busy === "browser"}
            style={{ alignSelf: "flex-start" }}
            onClick={async () => {
              setBusy("browser");
              try {
                const u = await API.login("");
                if (u) {
                  setUrl(u);
                  void App.openURL(u);
                }
              } catch (e) {
                toastError(t("ts.loginFailed"), e);
              }
              setBusy("");
            }}
          >
            {t("ts.signInBrowser")}
          </Button>
          <div className="row" style={{ gap: 8 }}>
            <div style={{ flex: 1, height: 1, background: "var(--border)" }} />
            <span className="faint" style={{ fontSize: 11.5 }}>
              {t("common.or")}
            </span>
            <div style={{ flex: 1, height: 1, background: "var(--border)" }} />
          </div>
          <Field label={t("ts.authKey")} hint={t("ts.authKeyHint")}>
            <div className="row">
              <Input type="password" value={key} onChange={(e) => setKey(e.target.value)} placeholder="tskey-auth-…" style={{ flex: 1 }} />
              <Button
                icon={<KeyRound size={14} />}
                loading={busy === "key"}
                disabled={!key.trim()}
                onClick={async () => {
                  setBusy("key");
                  await run(async () => {
                    await API.login(key);
                    if (share) await API.shareAuthKey(key);
                  }, t("ts.loginFailed"));
                  setKey("");
                  setBusy("");
                }}
              >
                {t("ts.useKey")}
              </Button>
            </div>
            <label className="row" style={{ gap: 8, marginTop: 8, alignItems: "center", cursor: "pointer" }}>
              <Switch checked={share} onChange={setShare} />
              <span className="muted" style={{ fontSize: 12 }}>
                {t("ts.shareKey")}
              </span>
            </label>
          </Field>
        </div>
        {shown && (
          <div className="col" style={{ alignItems: "center", gap: 8 }}>
            <QRCode text={shown} size={168} />
            <div className="faint" style={{ fontSize: 11.5, textAlign: "center", maxWidth: 180 }}>
              {t("ts.scanHint")}
            </div>
            <div className="row">
              <Button size="sm" icon={<ExternalLink size={13} />} onClick={() => App.openURL(shown)}>
                {t("common.open")}
              </Button>
              <Button size="sm" icon={<Copy size={13} />} onClick={() => copy(shown, t("common.copied"))}>
                {t("common.copy")}
              </Button>
            </div>
          </div>
        )}
      </div>
    </Card>
  );
}

function Self({ status, mode }: { status: TailscaleStatus; mode: Mode }) {
  const t = useT();
  const { data: keyShared, reload: reloadShared } = useAsync(() => API.authKeyShared(), [status.backendState]);
  const [shareKey, setShareKey] = useState("");
  const self = status.self;
  if (!self) return null;
  return (
    <Card
      title={t("ts.thisDevice")}
      icon={<Monitor size={15} />}
      actions={
        <>
          <Button size="sm" variant="ghost" icon={<ExternalLink size={13} />} onClick={() => API.openAdmin()}>
            {t("ts.admin")}
          </Button>
          {mode === "embedded" && (
            <Button
              size="sm"
              variant="danger"
              icon={<LogOut size={13} />}
              onClick={async () => {
                const forget = await confirm({ title: t("ts.logoutTitle"), message: t("ts.logoutMsg"), confirm: t("ts.logout"), danger: true });
                if (forget) void run(() => API.logout(true), t("common.failed"));
              }}
            >
              {t("ts.logout")}
            </Button>
          )}
        </>
      }
    >
      <dl className="kv">
        <dt>{t("ts.deviceName")}</dt>
        <dd>{self.hostName}</dd>
        <dt>{t("ts.address")}</dt>
        <dd>
          {self.tailscaleIps.map((ip) => (
            <button key={ip} className="chip mono" style={{ marginInlineStart: 4 }} onClick={() => copy(ip, t("ts.ipCopied"))}>
              {ip}
            </button>
          ))}
        </dd>
        <dt>MagicDNS</dt>
        <dd className="mono selectable">{self.dnsName || "—"}</dd>
        <dt>{t("ts.account")}</dt>
        <dd>{status.user?.loginName ?? "—"}</dd>
        <dt>{t("ts.tailnet")}</dt>
        <dd>{status.tailnetName || "—"}</dd>
        {status.shareProxy && (
          <>
            <dt>{t("ts.sharedProxy")}</dt>
            <dd className="mono">{status.shareProxy}</dd>
          </>
        )}
      </dl>
      {mode === "embedded" && keyShared === false && (
        <div className="row" style={{ marginTop: 10, gap: 8 }}>
          <Input type="password" value={shareKey} onChange={(e) => setShareKey(e.target.value)} placeholder="tskey-auth-…" style={{ flex: 1 }} />
          <Button
            icon={<KeyRound size={14} />}
            disabled={!shareKey.trim()}
            onClick={async () => {
              await run(() => API.shareAuthKey(shareKey), t("common.failed"));
              setShareKey("");
              reloadShared();
            }}
          >
            {t("ts.shareBtn")}
          </Button>
        </div>
      )}
      {mode === "embedded" && keyShared && (
        <div className="row" style={{ marginTop: 10, gap: 8, alignItems: "center", justifyContent: "space-between" }}>
          <span className="muted" style={{ fontSize: 12 }}>
            {t("ts.keyShared")}
          </span>
          <Button size="sm" onClick={async () => { await run(() => API.stopSharingAuthKey(), t("common.failed")); reloadShared(); }}>
            {t("ts.stopSharing")}
          </Button>
        </div>
      )}
      {(status.health?.length ?? 0) > 0 && (
        <div className="col" style={{ marginTop: 10, gap: 4 }}>
          {status.health!.map((h, i) => (
            <div key={i} style={{ fontSize: 12, color: "var(--warning)" }}>
              ⚠ {h}
            </div>
          ))}
        </div>
      )}
    </Card>
  );
}

function ExitNode({ status, mode }: { status: TailscaleStatus; mode: Mode }) {
  const t = useT();
  const settings = useApp((s) => s.settings);
  const options = status.peers.filter((p) => p.exitNodeOption);
  const [busy, setBusy] = useState(false);
  const current = status.exitNodeId ?? "";
  return (
    <Card title={t("ts.exitNode")} icon={<Router size={15} />}>
      <div className="col" style={{ gap: 10 }}>
        <div className="muted" style={{ fontSize: 12.5 }}>
          {mode === "embedded" ? t("ts.exitHintEmbedded") : t("ts.exitHintSystem")}
        </div>
        <Select
          value={current}
          disabled={busy}
          onChange={async (id) => {
            setBusy(true);
            await run(() => API.setExitNode(id), t("common.failed"));
            setBusy(false);
          }}
          options={[
            { value: "", label: t("common.none") },
            ...options.map((p) => ({ value: p.id, label: `${p.hostName}${p.online ? "" : ` (${t("ts.offline")})`}` })),
          ]}
        />
        {options.length === 0 && <div className="faint" style={{ fontSize: 12 }}>{t("ts.noExitNodes")}</div>}
        {mode === "embedded" && settings && (
          <div className="row">
            <span className="grow">{t("ts.allowLan")}</span>
            <Switch checked={settings.tailscale.exitNodeAllowLan} onChange={(v) => run(() => API.setPrefs({ exitNodeAllowLan: v }), t("common.failed"))} />
          </div>
        )}
      </div>
    </Card>
  );
}

function Peer({ p, mode }: { p: TailscalePeer; mode: Mode }) {
  const t = useT();
  const lang = useApp((s) => s.lang);
  const [ping, setPing] = useState<TailscalePingResult | null>(null);
  const [pinging, setPinging] = useState(false);
  const direct = !!p.curAddr;
  return (
    <div className="peer-row">
      <span className={`dot ${p.online ? "success" : ""}`} />
      <div className="muted" style={{ display: "flex" }}>
        {osIcon(p.os)}
      </div>
      <div className="grow" style={{ minWidth: 0 }}>
        <div className="row" style={{ gap: 6 }}>
          <span style={{ fontWeight: 600 }} className="ellipsis">
            {p.hostName}
          </span>
          {p.exitNode && <Badge tone="accent">{t("ts.usingExit")}</Badge>}
          {p.exitNodeOption && !p.exitNode && <Badge>{t("ts.exitCapable")}</Badge>}
          {(p.primaryRoutes?.length ?? 0) > 0 && <Badge tone="info">{t("ts.subnets", { n: p.primaryRoutes!.length })}</Badge>}
          {p.expired && <Badge tone="danger">{t("ts.keyExpired")}</Badge>}
          {p.tags?.map((tag) => (
            <Badge key={tag}>{tag.replace("tag:", "#")}</Badge>
          ))}
        </div>
        <div className="faint ellipsis" style={{ fontSize: 11.5 }}>
          {p.os || "—"}
          {p.user ? ` · ${p.user}` : ""}
          {p.online
            ? p.active
              ? ` · ${direct ? t("ts.direct") : `${t("ts.relay")} ${p.relay ?? ""}`} · ↓${bytes(p.rxBytes)} ↑${bytes(p.txBytes)}`
              : ` · ${t("ts.idle")}`
            : p.lastSeen
              ? ` · ${t("ts.lastSeen", { when: relative(p.lastSeen, lang) })}`
              : ""}
        </div>
      </div>
      <div className="row" style={{ gap: 4 }}>
        {p.tailscaleIps[0] && (
          <button className="chip mono" onClick={() => copy(p.tailscaleIps[0]!, t("ts.ipCopied"))} title={t("common.copy")}>
            {p.tailscaleIps[0]}
          </button>
        )}
        {ping && (
          <span className="delay good" title={ping.endpoint || ping.derp} style={{ minWidth: 70, textAlign: "end" }}>
            {ping.err ? <span className="delay bad">{t("common.failed")}</span> : `${ltr(`${ping.latencyMs.toFixed(0)} ms`)} ${ping.endpoint ? "↔" : "⇢"}`}
          </span>
        )}
        <Button
          size="sm"
          variant="ghost"
          icon={<Radar size={14} />}
          loading={pinging}
          tip={t("ts.ping")}
          disabled={!p.online || !p.tailscaleIps[0] || mode === "off"}
          onClick={async () => {
            setPinging(true);
            try {
              setPing(await API.ping(p.tailscaleIps[0]!));
            } catch (e) {
              setPing({ latencyMs: 0, err: String(e) });
            }
            setPinging(false);
          }}
        />
      </div>
    </div>
  );
}

function Peers({ status, mode }: { status: TailscaleStatus; mode: Mode }) {
  const t = useT();
  const [filter, setFilter] = useState<"all" | "online">("online");
  const [search, setSearch] = useState("");
  useNow(15000);
  const peers = useMemo(() => {
    const q = search.trim().toLowerCase();
    return status.peers.filter((p) => (filter === "all" || p.online) && (!q || [p.hostName, p.dnsName, p.os, p.user, ...p.tailscaleIps].some((v) => v?.toLowerCase().includes(q))));
  }, [status.peers, filter, search]);
  const online = status.peers.filter((p) => p.online).length;
  return (
    <Card
      title={t("ts.peers")}
      icon={<Network size={15} />}
      actions={
        <div className="row">
          <SearchInput value={search} onChange={setSearch} placeholder={t("common.search")} width={180} />
          <Segmented
            value={filter}
            onChange={setFilter}
            options={[
              { value: "online", label: `${t("ts.online")} ${online}` },
              { value: "all", label: `${t("common.all")} ${status.peers.length}` },
            ]}
          />
        </div>
      }
    >
      {peers.length === 0 ? (
        <Empty title={t("ts.noPeers")} art />
      ) : (
        <div className="col" style={{ gap: 0 }}>
          {peers.map((p) => (
            <Peer key={p.id} p={p} mode={mode} />
          ))}
        </div>
      )}
    </Card>
  );
}

function EmbeddedSettings() {
  const t = useT();
  const s = useApp((st) => st.settings?.tailscale);
  const running = useApp((st) => st.state?.core.status === "running");
  const { data: proxies } = useAsync(() => (running ? Proxies.view() : Promise.resolve(undefined)), [running]);
  const [host, setHost] = useState(s?.hostname ?? "");
  const [control, setControl] = useState(s?.controlUrl ?? "");
  if (!s) return null;
  const patch = (p: Partial<typeof s>) => run(() => patchSettings({ tailscale: p }), t("common.failed"));
  const viaOptions = [{ value: "", label: t("ts.direct") }, ...(proxies?.groups ?? []).map((g) => ({ value: g.name, label: g.name }))];
  return (
    <Section title={t("ts.nodeSettings")}>
      <Row label={t("ts.deviceName")} desc={t("ts.deviceNameHint")}>
        <Input value={host} onChange={(e) => setHost(e.target.value)} onBlur={() => host !== s.hostname && patch({ hostname: host })} placeholder={t("ts.deviceNameAuto")} style={{ width: 200 }} />
      </Row>
      <Row label={t("ts.controlUrl")} desc={t("ts.controlUrlHint")}>
        <Input value={control} onChange={(e) => setControl(e.target.value)} onBlur={() => control !== s.controlUrl && patch({ controlUrl: control })} placeholder="https://controlplane.tailscale.com" style={{ width: 260 }} />
      </Row>
      <Row label={t("ts.controlVia")} desc={t("ts.controlViaHint")}>
        <Select value={s.controlVia} onChange={(v) => patch({ controlVia: v })} options={viaOptions} width={200} />
      </Row>
      <Row label={t("ts.acceptRoutes")} desc={t("ts.acceptRoutesHint")}>
        <Switch checked={s.acceptRoutes} onChange={(v) => patch({ acceptRoutes: v })} />
      </Row>
      <Row label={t("ts.routeSubnets")} desc={t("ts.routeSubnetsHint")}>
        <Switch checked={s.routeSubnets} onChange={(v) => patch({ routeSubnets: v })} />
      </Row>
      <Row label="MagicDNS" desc={t("ts.magicDnsHint")}>
        <Switch checked={s.magicDns} onChange={(v) => patch({ magicDns: v })} />
      </Row>
      <Row label={t("ts.joinSelector")} desc={t("ts.joinSelectorHint", { name: s.proxyName })}>
        <Switch checked={s.joinSelector} onChange={(v) => patch({ joinSelector: v })} />
      </Row>
      <Row label={t("ts.shareProxy")} desc={t("ts.shareProxyHint")}>
        {s.shareProxy && <NumberInput value={s.shareProxyPort} onChange={(v) => patch({ shareProxyPort: v })} min={1} max={65535} width={90} />}
        <Switch checked={s.shareProxy} onChange={(v) => patch({ shareProxy: v })} />
      </Row>
      <Row label={t("ts.ephemeral")} desc={t("ts.ephemeralHint")}>
        <Switch checked={s.ephemeral} onChange={(v) => patch({ ephemeral: v })} />
      </Row>
    </Section>
  );
}

function SystemSettings({ status }: { status: TailscaleStatus }) {
  const t = useT();
  const s = useApp((st) => st.settings?.tailscale);
  if (!s) return null;
  const patch = (p: Partial<typeof s>) => run(() => patchSettings({ tailscale: p }), t("common.failed"));
  return (
    <Section title={t("ts.coexistence")}>
      <Row label={t("ts.connected")} desc={t("ts.connectedHint")}>
        <Switch checked={status.backendState === "Running"} onChange={(v) => run(() => API.setRunning(v), t("common.failed"))} />
      </Row>
      <Row label={t("ts.coexist")} desc={t("ts.coexistHint")}>
        <Switch checked={s.coexist} onChange={(v) => patch({ coexist: v })} />
      </Row>
      <Row label="MagicDNS" desc={t("ts.magicDnsSystemHint")}>
        <Switch checked={s.magicDns} onChange={(v) => patch({ magicDns: v })} />
      </Row>
      <Row label={t("ts.acceptRoutes")} desc={t("ts.acceptRoutesHint")}>
        <Switch checked={status.acceptRoutes} onChange={(v) => run(() => API.setPrefs({ acceptRoutes: v }), t("common.failed"))} />
      </Row>
    </Section>
  );
}

function Routing({ status, mode }: { status: TailscaleStatus; mode: Mode }) {
  const t = useT();
  const s = useApp((st) => st.settings?.tailscale);
  if (!s) return null;
  const via = mode === "embedded" ? s.proxyName : "DIRECT";
  const rows = [
    ["100.64.0.0/10", via],
    ["fd7a:115c:a1e0::/48", via],
    ["*.ts.net" + (status.magicDnsSuffix ? `, *.${status.magicDnsSuffix}` : ""), via],
    ...(mode === "embedded" && s.routeSubnets && s.acceptRoutes ? (status.routes ?? []).map((r) => [r, via]) : []),
  ];
  return (
    <Card title={t("ts.routing")} icon={<Waypoints size={15} />}>
      <div className="muted" style={{ fontSize: 12.5, marginBottom: 10 }}>
        {mode === "embedded" ? t("ts.routingEmbedded", { name: s.proxyName }) : s.coexist ? t("ts.routingSystem") : t("ts.routingSystemOff")}
      </div>
      {(mode === "embedded" || s.coexist) && (
        <div style={{ border: "1px solid var(--border)", borderRadius: 9, overflow: "hidden" }}>
          {rows.map(([dst, out]) => (
            <div key={dst} className="row table-row" style={{ display: "flex" }}>
              <span className="mono grow ellipsis">{dst}</span>
              <span className="faint">→</span>
              <Badge tone={out === "DIRECT" ? undefined : "accent"}>{out}</Badge>
            </div>
          ))}
        </div>
      )}
    </Card>
  );
}

export default function Tailscale() {
  const t = useT();
  const mode = (useApp((s) => s.settings?.tailscale.mode) ?? "off") as Mode;
  const status = useApp((s) => s.tailscale);
  const coreRunning = useApp((s) => s.state?.core.status === "running");
  const { data: sys } = useAsync(() => API.systemStatus(), [mode]);
  const setMode = (m: Mode) => run(() => patchSettings({ tailscale: { mode: m } }), t("common.failed"));
  const st = status && status.source === mode ? status : null;
  return (
    <>
      <PageHeader title="Tailscale" sub={st && <Badge tone={stateTone(st.backendState)}>{t(`ts.state.${st.backendState}` as never) || st.backendState}</Badge>}>
        <Segmented
          value={mode}
          onChange={setMode}
          options={[
            { value: "off", label: t("ts.mode.off") },
            { value: "embedded", label: t("ts.mode.embedded") },
            { value: "system", label: t("ts.mode.system") },
          ]}
        />
      </PageHeader>
      <div className="page-body">
        {mode === "off" && (
          <div className="col" style={{ gap: 14, maxWidth: 900 }}>
            <div className="ts-hero card card-pad">
              <div className="ts-hero-icon">
                <Waypoints size={26} />
              </div>
              <div className="grow">
                <h2 style={{ fontSize: 17 }}>{t("ts.heroTitle")}</h2>
                <p className="muted" style={{ marginTop: 4 }}>
                  {t("ts.heroDesc")}
                </p>
              </div>
            </div>
            <div className="grid-cards" ref={reflow}>
              <div className="card card-pad col ts-choice">
                <div className="row">
                  <Activity size={18} color="var(--accent-fg)" />
                  <h3 style={{ fontSize: 14 }}>{t("ts.mode.embedded")}</h3>
                  <Badge tone="accent">{t("ts.recommended")}</Badge>
                </div>
                <p className="muted" style={{ fontSize: 12.5 }}>
                  {t("ts.embeddedDesc")}
                </p>
                <ul className="ts-points">
                  <li>{t("ts.embeddedP1")}</li>
                  <li>{t("ts.embeddedP2")}</li>
                  <li>{t("ts.embeddedP3")}</li>
                </ul>
                <Button variant="primary" onClick={() => setMode("embedded")} style={{ alignSelf: "flex-start" }}>
                  {t("ts.useEmbedded")}
                </Button>
              </div>
              <div className="card card-pad col ts-choice">
                <div className="row">
                  <AppWindow size={18} color="var(--info)" />
                  <h3 style={{ fontSize: 14 }}>{t("ts.mode.system")}</h3>
                  {sys?.available ? <Badge tone="success">{t("ts.detected")}</Badge> : <Badge>{t("ts.notDetected")}</Badge>}
                </div>
                <p className="muted" style={{ fontSize: 12.5 }}>
                  {t("ts.systemDesc")}
                </p>
                <ul className="ts-points">
                  <li>{t("ts.systemP1")}</li>
                  <li>{t("ts.systemP2")}</li>
                  <li>{t("ts.systemP3")}</li>
                </ul>
                <Button onClick={() => setMode("system")} style={{ alignSelf: "flex-start" }}>
                  {t("ts.useSystem")}
                </Button>
              </div>
            </div>
          </div>
        )}

        {mode === "embedded" && (
          <div className="col" style={{ gap: 12 }}>
            {!coreRunning && <CoreDown banner />}
            {st && (st.backendState === "NeedsLogin" || st.backendState === "NeedsMachineAuth") && <Login status={st} />}
            {st?.backendState === "Starting" || (!st && coreRunning) ? (
              <Card title={t("ts.starting")} icon={<Settings2 size={15} className="spin" />}>
                <div className="muted">{t("ts.startingHint")}</div>
              </Card>
            ) : null}
            {st?.backendState === "Running" && (
              <>
                <div className="split">
                  <div>
                    <Self status={st} mode={mode} />
                  </div>
                  <div>
                    <ExitNode status={st} mode={mode} />
                  </div>
                </div>
                <Routing status={st} mode={mode} />
                <Peers status={st} mode={mode} />
              </>
            )}
            <EmbeddedSettings />
          </div>
        )}

        {mode === "system" && (
          <div className="col" style={{ gap: 12 }}>
            {!st?.available ? (
              <Card title={t("ts.notDetected")} icon={<AppWindow size={15} />}>
                <div className="col" style={{ gap: 10 }}>
                  <div className="muted">{t("ts.installHint")}</div>
                  {st?.error && <div className="faint mono" style={{ fontSize: 11.5 }}>{st.error}</div>}
                  <Button icon={<ExternalLink size={14} />} onClick={() => App.openURL("https://tailscale.com/download")} style={{ alignSelf: "flex-start" }}>
                    {t("ts.download")}
                  </Button>
                </div>
              </Card>
            ) : st.backendState !== "Running" ? (
              <Banner tone="warning" action={<Button size="sm" onClick={() => run(() => API.setRunning(true), t("common.failed"))}>{t("ts.connect")}</Button>}>
                {t("ts.systemNotRunning", { state: t(`ts.state.${st.backendState}` as never) })}
              </Banner>
            ) : (
              <>
                <div className="split">
                  <div>
                    <Self status={st} mode={mode} />
                  </div>
                  <div>
                    <ExitNode status={st} mode={mode} />
                  </div>
                </div>
                <Routing status={st} mode={mode} />
                <Peers status={st} mode={mode} />
              </>
            )}
            {st && <SystemSettings status={st} />}
          </div>
        )}
      </div>
    </>
  );
}

