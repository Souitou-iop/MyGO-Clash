import { Dices, ExternalLink, Plus, RefreshCw, Trash2 } from "lucide-react";
import { lazy, Suspense, useEffect, useState } from "react";
import { useAsync } from "../../lib/hooks";
import { useT } from "../../lib/i18n";
import { patchSettings, run, useApp } from "../../lib/store";
import { App, Core, Settings as SettingsAPI, System, type Clash as ClashSettings, type Controller } from "../../mygo";
import { Button, Collapse, Dialog, Field, Input, NumberInput, Row, Section, Select, Spinner, Switch } from "../../ui";
import { usePatch } from "./General";

const CodeEditor = lazy(() => import("../../components/CodeEditor"));

function PortsDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const t = useT();
  const s = useApp((st) => st.settings!);
  const os = useApp((st) => st.info?.os);
  const [c, setC] = useState<ClashSettings>(s.clash);
  const [inUse, setInUse] = useState<Record<string, boolean>>({});
  useEffect(() => setC(s.clash), [open, s.clash]);
  const set = <K extends keyof ClashSettings>(k: K, v: ClashSettings[K]) => setC((x) => ({ ...x, [k]: v }));
  const check = async (key: string, port: number) => {
    const used = await Core.portInUse(port).catch(() => false);
    setInUse((u) => ({ ...u, [key]: used }));
  };
  const random = () => 20000 + Math.floor(Math.random() * 40000);
  const ports: { key: string; label: string; port: keyof ClashSettings; on?: keyof ClashSettings; show: boolean }[] = [
    { key: "mixed", label: t("settings.mixedPort"), port: "mixedPort", show: true },
    { key: "socks", label: t("settings.socksPort"), port: "socksPort", on: "socksEnabled", show: true },
    { key: "http", label: t("settings.httpPort"), port: "httpPort", on: "httpEnabled", show: true },
    { key: "redir", label: t("settings.redirPort"), port: "redirPort", on: "redirEnabled", show: os !== "windows" },
    { key: "tproxy", label: t("settings.tproxyPort"), port: "tproxyPort", on: "tproxyEnabled", show: os === "linux" },
  ];
  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={t("settings.ports")}
      footer={
        <>
          <Button onClick={onClose}>{t("common.cancel")}</Button>
          <Button
            variant="primary"
            onClick={async () => {
              const ok = await run(() => patchSettings({ clash: c }), t("settings.saveFailed"));
              if (ok) onClose();
            }}
          >
            {t("common.save")}
          </Button>
        </>
      }
    >
      <div className="rows">
        {ports
          .filter((p) => p.show)
          .map((p) => (
            <div key={p.key} className="setting">
              <div className="setting-text">
                <div className="setting-label">{p.label}</div>
                {inUse[p.key] && <div className="setting-desc" style={{ color: "var(--danger)" }}>{t("settings.portInUse")}</div>}
              </div>
              <NumberInput
                value={c[p.port] as number}
                min={1}
                max={65535}
                onChange={(v) => {
                  set(p.port, v as never);
                  void check(p.key, v);
                }}
              />
              <Button
                size="sm"
                variant="ghost"
                icon={<Dices size={14} />}
                tip={t("settings.randomPort")}
                onClick={() => {
                  const v = random();
                  set(p.port, v as never);
                  void check(p.key, v);
                }}
              />
              {p.on ? <Switch checked={c[p.on] as boolean} onChange={(v) => set(p.on!, v as never)} /> : <span style={{ width: 36 }} />}
            </div>
          ))}
      </div>
    </Dialog>
  );
}

function ControllerDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const t = useT();
  const s = useApp((st) => st.settings!);
  const [c, setC] = useState<Controller>(s.clash.controller);
  const [origins, setOrigins] = useState(s.clash.controller.allowOrigins.join("\n"));
  const [uis, setUis] = useState<string[]>(s.webUis);
  const [newUi, setNewUi] = useState("");
  useEffect(() => {
    setC(s.clash.controller);
    setOrigins(s.clash.controller.allowOrigins.join("\n"));
    setUis(s.webUis);
  }, [open, s.clash.controller, s.webUis]);
  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={t("settings.controller")}
      size="wide"
      footer={
        <>
          <Button onClick={onClose}>{t("common.cancel")}</Button>
          <Button
            variant="primary"
            onClick={async () => {
              const ok = await run(
                () => patchSettings({ clash: { controller: { ...c, allowOrigins: origins.split(/\s+/).filter(Boolean) } }, webUis: uis }),
                t("settings.saveFailed"),
              );
              if (ok) onClose();
            }}
          >
            {t("common.save")}
          </Button>
        </>
      }
    >
      <div className="form">
        <div className="rows">
          <Row label={t("settings.controllerEnable")} desc={t("settings.controllerDesc")}>
            <Switch checked={c.enabled} onChange={(v) => setC({ ...c, enabled: v })} />
          </Row>
        </div>
        <div className="form-row">
          <Field label={t("settings.address")} hint={t("settings.addressHint")}>
            <Input className="mono" value={c.address} onChange={(e) => setC({ ...c, address: e.target.value })} />
          </Field>
          <Field label={t("settings.secret")}>
            <div className="row">
              <Input className="mono" value={c.secret} onChange={(e) => setC({ ...c, secret: e.target.value })} style={{ flex: 1 }} />
              <Button icon={<Dices size={14} />} onClick={() => SettingsAPI.randomSecret().then((sec) => setC({ ...c, secret: sec }))} tip={t("settings.randomSecret")} />
            </div>
          </Field>
        </div>
        <Field label={t("settings.origins")} hint={t("settings.originsHint")}>
          <textarea className="textarea mono" rows={3} value={origins} onChange={(e) => setOrigins(e.target.value)} spellCheck={false} />
        </Field>
        <div className="rows">
          <Row label={t("settings.privateNetwork")} desc={t("settings.privateNetworkDesc")}>
            <Switch checked={c.allowPrivateNetwork} onChange={(v) => setC({ ...c, allowPrivateNetwork: v })} />
          </Row>
        </div>
        <div className="section-title" style={{ margin: "6px 0 0" }}>
          {t("settings.webUis")}
        </div>
        <div className="rows">
          {uis.map((u, i) => (
            <div key={i} className="setting">
              <span className="grow mono ellipsis" style={{ fontSize: 12 }}>
                {u}
              </span>
              <Button size="sm" variant="ghost" icon={<ExternalLink size={13} />} disabled={!s.clash.controller.enabled} onClick={() => run(() => Core.openWebUI(u), t("common.failed"))} />
              <Button size="sm" variant="ghost" icon={<Trash2 size={13} />} onClick={() => setUis(uis.filter((_, k) => k !== i))} />
            </div>
          ))}
          <div className="setting">
            <Input className="mono" value={newUi} onChange={(e) => setNewUi(e.target.value)} placeholder="https://…?hostname=%host&port=%port&secret=%secret" style={{ flex: 1 }} />
            <Button
              size="sm"
              icon={<Plus size={13} />}
              disabled={!/^https?:\/\//.test(newUi)}
              onClick={() => {
                setUis([...uis, newUi]);
                setNewUi("");
              }}
            >
              {t("common.add")}
            </Button>
          </div>
        </div>
      </div>
    </Dialog>
  );
}

function DNSDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const t = useT();
  const s = useApp((st) => st.settings!);
  const [text, setText] = useState(s.dns.config);
  useEffect(() => setText(s.dns.config), [open, s.dns.config]);
  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={t("settings.dnsOverride")}
      size="xwide"
      flush
      footer={
        <>
          <Button variant="ghost" onClick={() => SettingsAPI.defaults().then((d) => setText(d.dns.config))} style={{ marginInlineEnd: "auto" }}>
            {t("settings.resetDefault")}
          </Button>
          <Button onClick={onClose}>{t("common.cancel")}</Button>
          <Button
            variant="primary"
            onClick={async () => {
              const ok = await run(() => patchSettings({ dns: { config: text } }), t("settings.saveFailed"));
              if (ok) onClose();
            }}
          >
            {t("common.save")}
          </Button>
        </>
      }
    >
      <div className="banner warning" style={{ margin: "0 16px 10px" }}>
        {t("settings.dnsWarn")}
      </div>
      <Suspense fallback={<Spinner />}>
        <CodeEditor value={text} onChange={setText} lang="yaml" />
      </Suspense>
    </Dialog>
  );
}

export default function Clash() {
  const t = useT();
  const s = useApp((st) => st.settings!);
  const os = useApp((st) => st.info?.os);
  const patch = usePatch();
  const [dialog, setDialog] = useState<"" | "ports" | "controller" | "dns">("");
  const [busy, setBusy] = useState("");
  const [testUrl, setTestUrl] = useState(s.latency.url);
  useEffect(() => setTestUrl(s.latency.url), [s.latency.url]);
  const { data: ifaces } = useAsync(() => App.networkInterfaces(), []);
  const c = s.clash;
  const act = async (key: string, fn: () => Promise<unknown>, ok: string) => {
    setBusy(key);
    await run(fn, t("common.failed"), ok);
    setBusy("");
  };
  const portsDesc = [`mixed ${c.mixedPort}`, c.socksEnabled && `socks ${c.socksPort}`, c.httpEnabled && `http ${c.httpPort}`, c.redirEnabled && os !== "windows" && `redir ${c.redirPort}`, c.tproxyEnabled && os === "linux" && `tproxy ${c.tproxyPort}`]
    .filter(Boolean)
    .join(" · ");
  return (
    <>
      <Section title={t("settings.core")}>
        <Row label={t("settings.ports")} desc={portsDesc}>
          <Button size="sm" onClick={() => setDialog("ports")}>
            {t("common.edit")}
          </Button>
        </Row>
        <Row label="IPv6" desc={t("settings.ipv6Desc")}>
          <Switch checked={c.ipv6} onChange={(v) => patch({ clash: { ipv6: v } })} />
        </Row>
        <Row label={t("settings.unifiedDelay")} desc={t("settings.unifiedDelayDesc")}>
          <Switch checked={c.unifiedDelay} onChange={(v) => patch({ clash: { unifiedDelay: v } })} />
        </Row>
        <Row label={t("settings.tcpConcurrent")} desc={t("settings.tcpConcurrentDesc")}>
          <Switch checked={c.tcpConcurrent} onChange={(v) => patch({ clash: { tcpConcurrent: v } })} />
        </Row>
        <Row label={t("settings.findProcess")} desc={t("settings.findProcessDesc")}>
          <Select
            value={c.findProcessMode}
            onChange={(v) => patch({ clash: { findProcessMode: v } })}
            options={[
              { value: "", label: t("settings.followProfile") },
              { value: "strict", label: t("settings.findProcess.strict") },
              { value: "always", label: t("settings.findProcess.always") },
              { value: "off", label: t("settings.findProcess.off") },
            ]}
            width={150}
          />
        </Row>
        <Row label={t("settings.interface")} desc={t("settings.interfaceDesc")}>
          <Select
            value={c.interface}
            onChange={(v) => patch({ clash: { interface: v } })}
            options={[{ value: "", label: t("settings.auto") }, ...(ifaces ?? []).map((i) => ({ value: i.name, label: `${i.name} ${i.addrs[0] ?? ""}` }))]}
            width={200}
          />
        </Row>
        <Row label={t("settings.logLevel")} desc={t("settings.logLevelDesc")}>
          <Select value={c.logLevel} onChange={(v) => patch({ clash: { logLevel: v } })} options={["debug", "info", "warning", "error", "silent"].map((l) => ({ value: l, label: t(`logs.${l}` as never) }))} width={130} />
        </Row>
        <Row label={t("settings.controller")} desc={c.controller.enabled ? `${c.controller.address}` : t("settings.controllerOff")}>
          <Button size="sm" onClick={() => setDialog("controller")}>
            {t("common.configure")}
          </Button>
        </Row>
      </Section>

      <Section title="DNS">
        <Row label={t("settings.dnsOverrideDefault")} desc={t("settings.dnsOverrideDesc")}>
          <Button size="sm" onClick={() => setDialog("dns")}>
            {t("common.edit")}
          </Button>
          <Switch checked={s.dns.defaultOverride} onChange={(v) => patch({ dns: { defaultOverride: v } })} />
        </Row>
        <Row label={t("settings.flushDns")} desc={t("settings.flushDnsDesc")}>
          <Button size="sm" loading={busy === "dns"} onClick={() => act("dns", () => Core.flushDNS(), t("settings.flushed"))}>
            DNS
          </Button>
          <Button size="sm" loading={busy === "fakeip"} onClick={() => act("fakeip", () => Core.flushFakeIP(), t("settings.flushed"))}>
            Fake IP
          </Button>
        </Row>
      </Section>

      <Section title={t("settings.latency")}>
        <Row label={t("settings.testUrl")} desc={t("settings.testUrlDesc")}>
          <Input className="mono" value={testUrl} onChange={(e) => setTestUrl(e.target.value)} onBlur={() => testUrl !== s.latency.url && patch({ latency: { url: testUrl } })} style={{ width: 280 }} />
        </Row>
        <Row label={t("settings.testTimeout")} desc={t("settings.testTimeoutDesc")}>
          <NumberInput value={s.latency.timeoutMs} onChange={(v) => patch({ latency: { timeoutMs: v } })} min={100} max={60000} />
          <span className="muted">ms</span>
        </Row>
        <Row label={t("settings.autoCheck")} desc={t("settings.autoCheckDesc")}>
          <Collapse inline open={s.latency.autoCheck}>
            <NumberInput value={s.latency.autoCheckMinutes} onChange={(v) => patch({ latency: { autoCheckMinutes: v } })} min={1} max={1440} width={80} />
            <span className="muted">{t("common.minutes")}</span>
          </Collapse>
          <Switch checked={s.latency.autoCheck} onChange={(v) => patch({ latency: { autoCheck: v } })} />
        </Row>
      </Section>

      <Section title={t("settings.behavior")}>
        <Row label={t("settings.autoClose")} desc={t("settings.autoCloseDesc")}>
          <Switch checked={s.autoCloseConnections} onChange={(v) => patch({ autoCloseConnections: v })} />
        </Row>
        <Row label={t("settings.builtin")} desc={t("settings.builtinDesc")}>
          <Switch checked={s.builtinEnhanced} onChange={(v) => patch({ builtinEnhanced: v })} />
        </Row>
        <Row label={t("settings.geo")} desc={t("settings.geoDesc")}>
          <Button size="sm" icon={<RefreshCw size={13} />} loading={busy === "geo"} onClick={() => act("geo", () => Core.updateGeo(), t("settings.geoUpdated"))}>
            {t("common.update")}
          </Button>
        </Row>
        {os === "windows" && (
          <Row label={t("settings.uwp")} desc={t("settings.uwpDesc")}>
            <Button size="sm" loading={busy === "uwp"} onClick={() => act("uwp", () => System.uwpLoopback(), t("settings.uwpDone"))}>
              {t("settings.uwpRun")}
            </Button>
          </Row>
        )}
      </Section>
      <PortsDialog open={dialog === "ports"} onClose={() => setDialog("")} />
      <ControllerDialog open={dialog === "controller"} onClose={() => setDialog("")} />
      <DNSDialog open={dialog === "dns"} onClose={() => setDialog("")} />
    </>
  );
}

