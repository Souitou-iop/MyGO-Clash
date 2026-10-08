import { Globe, Monitor, Network as NetIcon, ShieldCheck, ShieldOff, Webcam, Wrench } from "lucide-react";
import { lazy, Suspense, useEffect, useState } from "react";
import { useAsync } from "../../lib/hooks";
import { useT } from "../../lib/i18n";
import { enableTunWithService } from "../../lib/service";
import { patchSettings, run, useApp } from "../../lib/store";
import { Settings as SettingsAPI, System, type SystemProxy, type Tun } from "../../mygo";
import { Badge, Banner, Button, confirm, Dialog, Field, Input, NumberInput, Row, Section, Segmented, Spinner, Switch } from "../../ui";
import { usePatch } from "./General";

const CodeEditor = lazy(() => import("../../components/CodeEditor"));

function SystemProxyDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const t = useT();
  const s = useApp((st) => st.settings!);
  const [sp, setSp] = useState<SystemProxy>(s.systemProxy);
  const [pacOpen, setPacOpen] = useState(false);
  const { data: current, reload } = useAsync(() => SettingsAPI.systemProxy(), [open]);
  const { data: defaults } = useAsync(() => SettingsAPI.defaultBypass(), []);
  useEffect(() => setSp(s.systemProxy), [open, s.systemProxy]);
  const set = <K extends keyof SystemProxy>(k: K, v: SystemProxy[K]) => setSp((x) => ({ ...x, [k]: v }));
  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={t("settings.sysproxySettings")}
      size="wide"
      footer={
        <>
          <Button onClick={onClose}>{t("common.cancel")}</Button>
          <Button
            variant="primary"
            onClick={async () => {
              await run(() => patchSettings({ systemProxy: { ...sp, enabled: s.systemProxy.enabled } }), t("settings.saveFailed"));
              void reload();
              onClose();
            }}
          >
            {t("common.save")}
          </Button>
        </>
      }
    >
      <div className="form">
        <div className="card card-pad" style={{ background: "var(--surface-2)" }}>
          <div className="section-title" style={{ margin: "0 0 6px" }}>
            {t("settings.osProxyNow")}
          </div>
          {current ? (
            <dl className="kv">
              <dt>{t("settings.enabled")}</dt>
              <dd>{current.enabled ? <Badge tone="success">{t("common.on")}</Badge> : <Badge>{t("common.off")}</Badge>}</dd>
              {current.pac ? (
                <>
                  <dt>PAC</dt>
                  <dd className="mono">{current.pac}</dd>
                </>
              ) : (
                <>
                  <dt>{t("settings.server")}</dt>
                  <dd className="mono">{current.host ? `${current.host}:${current.port}` : "—"}</dd>
                </>
              )}
            </dl>
          ) : (
            <Spinner />
          )}
        </div>
        <div className="form-row">
          <Field label={t("settings.proxyHost")} hint={t("settings.proxyHostHint")}>
            <Input value={sp.host} onChange={(e) => set("host", e.target.value)} />
          </Field>
          <Field label={t("settings.guardInterval")} hint={t("settings.guardHint")}>
            <div className="row">
              <Switch checked={sp.guard} onChange={(v) => set("guard", v)} />
              <NumberInput value={sp.guardInterval} onChange={(v) => set("guardInterval", v)} min={1} max={3600} width={90} />
              <span className="muted">{t("common.seconds")}</span>
            </div>
          </Field>
        </div>
        <div className="rows">
          <Row label={t("settings.pacMode")} desc={t("settings.pacDesc")}>
            <Button size="sm" onClick={() => setPacOpen(true)}>
              {t("settings.editPac")}
            </Button>
            <Switch checked={sp.pac} onChange={(v) => set("pac", v)} />
          </Row>
          <Row label={t("settings.defaultBypass")} desc={(defaults ?? []).join(", ")}>
            <Switch checked={sp.useDefaultBypass} onChange={(v) => set("useDefaultBypass", v)} />
          </Row>
        </div>
        <Field label={t("settings.bypass")} hint={t("settings.bypassHint")}>
          <textarea className="textarea mono" rows={4} value={sp.bypass} onChange={(e) => set("bypass", e.target.value)} placeholder="*.example.com, 10.0.0.0/8" spellCheck={false} />
        </Field>
      </div>
      <Dialog
        open={pacOpen}
        onClose={() => setPacOpen(false)}
        title={t("settings.editPac")}
        size="wide"
        flush
        footer={
          <Button variant="primary" onClick={() => setPacOpen(false)}>
            {t("common.done")}
          </Button>
        }
      >
        <div style={{ height: 380, display: "flex" }}>
          <Suspense fallback={<Spinner />}>
            <CodeEditor value={sp.pacScript} onChange={(v) => set("pacScript", v)} lang="javascript" />
          </Suspense>
        </div>
        <div className="muted" style={{ fontSize: 12, padding: "8px 16px" }}>
          {t("settings.pacPlaceholders")}
        </div>
      </Dialog>
    </Dialog>
  );
}

function TunDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const t = useT();
  const s = useApp((st) => st.settings!);
  const os = useApp((st) => st.info?.os);
  const [tun, setTun] = useState<Tun>(s.tun);
  const [hijack, setHijack] = useState(s.tun.dnsHijack.join(", "));
  const [exclude, setExclude] = useState(s.tun.routeExcludeAddress.join("\n"));
  useEffect(() => {
    setTun(s.tun);
    setHijack(s.tun.dnsHijack.join(", "));
    setExclude(s.tun.routeExcludeAddress.join("\n"));
  }, [open, s.tun]);
  const set = <K extends keyof Tun>(k: K, v: Tun[K]) => setTun((x) => ({ ...x, [k]: v }));
  const split = (v: string) => v.split(/[\s,]+/).map((x) => x.trim()).filter(Boolean);
  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={t("settings.tunSettings")}
      size="wide"
      footer={
        <>
          <Button onClick={onClose}>{t("common.cancel")}</Button>
          <Button
            variant="primary"
            onClick={async () => {
              const ok = await run(() => patchSettings({ tun: { ...tun, enabled: s.tun.enabled, dnsHijack: split(hijack), routeExcludeAddress: split(exclude) } }), t("settings.saveFailed"));
              if (ok) onClose();
            }}
          >
            {t("common.save")}
          </Button>
        </>
      }
    >
      <div className="form">
        <Field label={t("settings.stack")} hint={t("settings.stackHint")}>
          <Segmented
            value={tun.stack}
            onChange={(v) => set("stack", v)}
            options={[
              { value: "mixed", label: "Mixed" },
              { value: "gvisor", label: "gVisor" },
              { value: "system", label: "System" },
            ]}
          />
        </Field>
        <div className="form-row">
          <Field label={t("settings.device")} hint={t("settings.deviceHint")}>
            <Input value={tun.device} onChange={(e) => set("device", e.target.value)} placeholder={os === "darwin" ? "utun1024" : os === "windows" ? "MyGO" : "mygo0"} />
          </Field>
          <Field label="MTU">
            <NumberInput value={tun.mtu} onChange={(v) => set("mtu", v)} min={576} max={65535} width={120} />
          </Field>
        </div>
        <div className="rows">
          <Row label={t("settings.autoRoute")} desc={t("settings.autoRouteDesc")}>
            <Switch checked={tun.autoRoute} onChange={(v) => set("autoRoute", v)} />
          </Row>
          <Row label={t("settings.strictRoute")} desc={t("settings.strictRouteDesc")}>
            <Switch checked={tun.strictRoute} onChange={(v) => set("strictRoute", v)} />
          </Row>
          <Row label={t("settings.autoDetect")} desc={t("settings.autoDetectDesc")}>
            <Switch checked={tun.autoDetectInterface} onChange={(v) => set("autoDetectInterface", v)} />
          </Row>
          {os === "linux" && (
            <Row label={t("settings.autoRedirect")} desc={t("settings.autoRedirectDesc")}>
              <Switch checked={tun.autoRedirect} onChange={(v) => set("autoRedirect", v)} />
            </Row>
          )}
        </div>
        <Field label={t("settings.dnsHijack")} hint={t("settings.dnsHijackHint")}>
          <Input value={hijack} onChange={(e) => setHijack(e.target.value)} className="mono" />
        </Field>
        <Field label={t("settings.excludeRoutes")} hint={t("settings.excludeRoutesHint")}>
          <textarea className="textarea mono" rows={3} value={exclude} onChange={(e) => setExclude(e.target.value)} placeholder="192.168.0.0/16" spellCheck={false} />
        </Field>
      </div>
    </Dialog>
  );
}

function ServiceSection() {
  const t = useT();
  const state = useApp((s) => s.state);
  const [busy, setBusy] = useState("");
  if (!state?.service.supported) return null;
  const svc = state.service;
  const act = async (key: string, fn: () => Promise<void>, ok: string, fail: string) => {
    setBusy(key);
    await run(fn, fail, ok);
    setBusy("");
  };
  return (
    <Section title={t("service.title")}>
      <Row
        label={t("service.status")}
        icon={svc.installed ? <ShieldCheck size={16} color="var(--success)" /> : <ShieldOff size={16} />}
        desc={svc.installed ? (svc.error ? svc.error : svc.outdated ? t("service.outdated", { v: svc.version ?? "?" }) : t("service.ok", { v: svc.version ?? "" })) : t("service.notInstalled")}
      >
        {svc.installed ? (
          <>
            <Button size="sm" icon={<Wrench size={13} />} loading={busy === "repair"} onClick={() => act("repair", () => System.installService(false), t("service.installed"), t("service.installFailed"))}>
              {svc.outdated ? t("service.update") : t("service.repair")}
            </Button>
            <Button
              size="sm"
              variant="danger"
              loading={busy === "remove"}
              onClick={async () => {
                if (await confirm({ title: t("service.uninstall"), message: t("service.uninstallMsg"), confirm: t("service.uninstall"), danger: true }))
                  await act("remove", () => System.uninstallService(), t("service.uninstalled"), t("service.uninstallFailed"));
              }}
            >
              {t("service.uninstall")}
            </Button>
          </>
        ) : (
          <Button size="sm" variant="primary" icon={<ShieldCheck size={13} />} loading={busy === "install"} onClick={() => act("install", () => System.installService(false), t("service.installed"), t("service.installFailed"))}>
            {t("service.install")}
          </Button>
        )}
      </Row>
      <Row label={t("service.coreRuns")} desc={state.core.mode === "service" ? t("service.coreInService") : t("service.coreInSidecar")}>
        <Badge tone={state.core.privileged ? "success" : undefined}>{state.core.privileged ? t("service.privileged") : t("service.unprivileged")}</Badge>
      </Row>
    </Section>
  );
}

export default function Network() {
  const t = useT();
  const s = useApp((st) => st.settings!);
  const state = useApp((st) => st.state);
  const os = useApp((st) => st.info?.os);
  const patch = usePatch();
  const [dialog, setDialog] = useState<"" | "sysproxy" | "tun">("");
  const [busy, setBusy] = useState("");
  const toggle = async (key: string, p: Parameters<typeof patch>[0]) => {
    setBusy(key);
    await patch(p);
    setBusy("");
  };
  return (
    <>
      {state?.systemProxyError && <Banner tone="danger">{state.systemProxyError}</Banner>}
      <Section title={t("settings.proxy")}>
        <Row label={t("settings.systemProxy")} desc={t("settings.systemProxyDesc")} icon={<Globe size={16} />}>
          <Button size="sm" onClick={() => setDialog("sysproxy")}>
            {t("common.configure")}
          </Button>
          <Switch checked={s.systemProxy.enabled} busy={busy === "sys"} onChange={(v) => toggle("sys", { systemProxy: { enabled: v } })} />
        </Row>
        <Row
          label={t("settings.tun")}
          desc={state?.tunAvailable ? t("settings.tunDesc") : state?.service.outdated ? t("home.tunServiceOutdated") : t("settings.tunNeedsService")}
          icon={<NetIcon size={16} />}
        >
          <Button size="sm" onClick={() => setDialog("tun")}>
            {t("common.configure")}
          </Button>
          <Switch
            checked={s.tun.enabled && !!state?.tunAvailable}
            busy={busy === "tun"}
            onChange={(v) => (v && !state?.tunAvailable ? enableTunWithService() : toggle("tun", { tun: { enabled: v } }))}
          />
        </Row>
        <Row label={t("settings.allowLan")} desc={t("settings.allowLanDesc")} icon={<Monitor size={16} />}>
          <Switch checked={s.clash.allowLan} onChange={(v) => patch({ clash: { allowLan: v } })} />
        </Row>
      </Section>
      {os === "windows" && (
        <Section title={t("settings.leakProtection")}>
          <Row label={t("settings.dnsLeak")} desc={t("settings.dnsLeakDesc")} icon={<ShieldCheck size={16} />}>
            <Switch checked={s.tun.strictRoute} onChange={(v) => patch({ tun: { strictRoute: v } })} />
          </Row>
          <Row label={t("settings.webrtcGuard")} desc={t("settings.webrtcGuardDesc")} icon={<Webcam size={16} />}>
            <Switch checked={s.webrtcGuard} onChange={(v) => patch({ webrtcGuard: v })} />
          </Row>
        </Section>
      )}
      <ServiceSection />
      <SystemProxyDialog open={dialog === "sysproxy"} onClose={() => setDialog("")} />
      <TunDialog open={dialog === "tun"} onClose={() => setDialog("")} />
    </>
  );
}
