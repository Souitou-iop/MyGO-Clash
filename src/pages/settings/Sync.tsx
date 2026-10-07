import {
  Archive,
  CheckCircle2,
  Cloud,
  CloudOff,
  Download,
  FileUp,
  HardDrive,
  KeyRound,
  Laptop,
  Lock,
  RefreshCw,
  RotateCcw,
  ShieldAlert,
  ShieldCheck,
  Trash2,
  Upload,
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { CloudUpload } from "../../components/icons/CloudUpload";
import { bytes, dateTime, relative } from "../../lib/format";
import { useAsync, useNow } from "../../lib/hooks";
import { useT } from "../../lib/i18n";
import { run, toast, toastError, useApp } from "../../lib/store";
import { Sync as API, type BackupInfo, type SyncProbe } from "../../mygo";
import { Badge, Banner, Button, Card, confirm, Dialog, Empty, Field, Input, NumberInput, prompt, Row, Section, Select, Spinner, Switch } from "../../ui";
import { usePatch } from "./General";

/** strength scores a passphrase from 0 to 4. */
function strength(p: string): number {
  if (p.length < 8) return 0;
  let score = p.length >= 12 ? 2 : 1;
  if (/[a-z]/.test(p) && /[A-Z]/.test(p)) score++;
  if (/\d/.test(p) && /[^\w]/.test(p)) score++;
  if (p.length >= 20) score++;
  return Math.min(4, score);
}

const PRESETS = [
  { name: "Nextcloud", url: "https://cloud.example.com/remote.php/dav/files/USER/" },
  { name: "坚果云 Jianguoyun", url: "https://dav.jianguoyun.com/dav/" },
  { name: "InfiniCLOUD", url: "https://ogi.teracloud.jp/dav/" },
];

function SetupDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const t = useT();
  const s = useApp((st) => st.settings!);
  const [step, setStep] = useState<1 | 2>(1);
  const [url, setUrl] = useState(s.sync.url);
  const [user, setUser] = useState(s.sync.username);
  const [password, setPassword] = useState("");
  const [dir, setDir] = useState(s.sync.dir);
  const [insecure, setInsecure] = useState(s.sync.allowInsecure);
  const [pin, setPin] = useState(s.sync.pinnedKey);
  const [probe, setProbe] = useState<SyncProbe | null>(null);
  const [pass, setPass] = useState("");
  const [pass2, setPass2] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    if (!open) return;
    setStep(1);
    setUrl(s.sync.url);
    setUser(s.sync.username);
    setDir(s.sync.dir);
    setInsecure(s.sync.allowInsecure);
    setPin(s.sync.pinnedKey);
    setPassword("");
    setPass("");
    setPass2("");
    setProbe(null);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);
  const setup = { url, username: user, password, dir, passphrase: pass, allowInsecure: insecure, pinnedKey: pin };
  const isHttp = url.trim().startsWith("http://");
  const test = async () => {
    setBusy(true);
    try {
      setProbe(await API.probe(setup));
    } catch (e) {
      toastError(t("sync.testFailed"), e);
    }
    setBusy(false);
  };
  const untrusted = probe?.fingerprint && !probe.trusted;
  const canNext = probe?.reachable && (!untrusted || pin === probe.fingerprint);
  const newVault = !probe?.hasVault;
  const st = strength(pass);
  const canFinish = newVault ? st >= 1 && pass === pass2 : pass.length >= 8;
  const finish = async () => {
    setBusy(true);
    try {
      const created = await API.setup(setup);
      toast({ level: "success", message: created ? t("sync.vaultCreated") : t("sync.vaultOpened") });
      onClose();
    } catch (e) {
      toastError(t("sync.setupFailed"), e);
    }
    setBusy(false);
  };
  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={t("sync.setupTitle")}
      icon={<span style={{ display: "flex", color: "var(--accent-fg)" }}><CloudUpload size={18} /></span>}
      size="wide"
      footer={
        step === 1 ? (
          <>
            <Button onClick={test} loading={busy} disabled={!url.trim()} style={{ marginInlineEnd: "auto" }}>
              {t("sync.test")}
            </Button>
            <Button onClick={onClose}>{t("common.cancel")}</Button>
            <Button variant="primary" disabled={!canNext} onClick={() => setStep(2)}>
              {t("common.next")}
            </Button>
          </>
        ) : (
          <>
            <Button onClick={() => setStep(1)} style={{ marginInlineEnd: "auto" }}>
              {t("common.back")}
            </Button>
            <Button onClick={onClose}>{t("common.cancel")}</Button>
            <Button variant="primary" loading={busy} disabled={!canFinish} onClick={finish}>
              {newVault ? t("sync.createVault") : t("sync.openVault")}
            </Button>
          </>
        )
      }
    >
      {step === 1 ? (
        <div className="form">
          <div className="steps">
            <span className="step active">1 · {t("sync.stepServer")}</span>
            <span className="step">2 · {t("sync.stepPassphrase")}</span>
          </div>
          <Field label={t("sync.url")} hint={t("sync.urlHint")}>
            <Input className="mono" value={url} onChange={(e) => (setUrl(e.target.value), setProbe(null))} placeholder="https://dav.example.com/dav/" />
          </Field>
          <div className="row" style={{ gap: 6, flexWrap: "wrap", marginTop: -4 }}>
            {PRESETS.map((p) => (
              <button key={p.name} className="chip" onClick={() => (setUrl(p.url), setProbe(null))}>
                {p.name}
              </button>
            ))}
          </div>
          <div className="form-row">
            <Field label={t("sync.username")}>
              <Input value={user} onChange={(e) => (setUser(e.target.value), setProbe(null))} />
            </Field>
            <Field label={t("sync.password")} hint={s.sync.url ? t("sync.passwordKeep") : t("sync.passwordHint")}>
              <Input type="password" value={password} onChange={(e) => (setPassword(e.target.value), setProbe(null))} />
            </Field>
          </div>
          <Field label={t("sync.folder")} hint={t("sync.folderHint")}>
            <Input value={dir} onChange={(e) => (setDir(e.target.value), setProbe(null))} />
          </Field>
          {isHttp && (
            <Banner tone="warning" action={<Switch checked={insecure} onChange={(v) => (setInsecure(v), setProbe(null))} />}>
              {t("sync.httpWarn")}
            </Banner>
          )}
          {probe && (
            <div className={`banner ${probe.reachable ? "info" : "danger"}`}>
              {probe.reachable ? <CheckCircle2 size={16} color="var(--success)" /> : <ShieldAlert size={16} />}
              <div className="grow">
                {probe.reachable ? (probe.hasVault ? t("sync.foundVault") : t("sync.noVault")) : probe.error}
                {probe.fingerprint && (
                  <div className="mono faint" style={{ fontSize: 11, marginTop: 4, wordBreak: "break-all" }}>
                    SHA-256 {probe.fingerprint}
                  </div>
                )}
              </div>
            </div>
          )}
          {untrusted && (
            <Banner tone="warning" action={<Switch checked={pin === probe!.fingerprint} onChange={(v) => setPin(v ? probe!.fingerprint! : "")} />}>
              {t("sync.untrusted")}
            </Banner>
          )}
        </div>
      ) : (
        <div className="form">
          <div className="steps">
            <span className="step done">1 · {t("sync.stepServer")}</span>
            <span className="step active">2 · {t("sync.stepPassphrase")}</span>
          </div>
          <div className="banner info">
            <Lock size={16} />
            <span>{newVault ? t("sync.newPassHint") : t("sync.existingPassHint")}</span>
          </div>
          <Field label={t("sync.passphrase")}>
            <Input type="password" value={pass} onChange={(e) => setPass(e.target.value)} autoFocus />
          </Field>
          {newVault && (
            <>
              <div className="strength">
                {[0, 1, 2, 3].map((i) => (
                  <div key={i} className={i < st ? `on s${st}` : ""} />
                ))}
                <span className="muted" style={{ fontSize: 11.5 }}>
                  {t(`sync.strength.${st}` as never)}
                </span>
              </div>
              <Field label={t("sync.passphrase2")} error={pass2 && pass !== pass2 ? t("sync.mismatch") : undefined}>
                <Input type="password" value={pass2} onChange={(e) => setPass2(e.target.value)} />
              </Field>
              <div className="muted" style={{ fontSize: 12 }}>
                {t("sync.noRecovery")}
              </div>
            </>
          )}
        </div>
      )}
    </Dialog>
  );
}

function Backups() {
  const t = useT();
  const configured = useApp((s) => s.sync?.configured ?? false);
  const s = useApp((st) => st.settings!);
  const patch = usePatch();
  const { data, loading, reload, error } = useAsync(() => API.backups(), [configured]);
  const [busy, setBusy] = useState("");
  const act = async (key: string, fn: () => Promise<unknown>, ok: string) => {
    setBusy(key);
    await run(fn, t("common.failed"), ok);
    setBusy("");
    void reload();
  };
  const exportOne = async (b: BackupInfo | null) => {
    const pw = await prompt({ title: t("backup.exportTitle"), message: t("backup.exportMsg"), label: t("backup.password"), password: true });
    if (!pw) return;
    if (pw.length < 8) return toast({ level: "warning", message: t("sync.tooShort") });
    await act("export", () => API.exportBackup(b?.name ?? "", b?.remote ?? false, pw), t("backup.exported"));
  };
  return (
    <>
      <Section
        title={t("backup.title")}
        extra={
          <div className="row" style={{ textTransform: "none", letterSpacing: 0 }}>
            <Button size="sm" icon={<HardDrive size={13} />} loading={busy === "local"} onClick={() => act("local", () => API.createBackup(false), t("backup.created"))}>
              {t("backup.local")}
            </Button>
            {configured && (
              <Button size="sm" icon={<Upload size={13} />} loading={busy === "remote"} onClick={() => act("remote", () => API.createBackup(true), t("backup.uploaded"))}>
                {t("backup.remote")}
              </Button>
            )}
            <Button size="sm" icon={<Download size={13} />} onClick={() => exportOne(null)}>
              {t("backup.exportNow")}
            </Button>
            <Button
              size="sm"
              icon={<FileUp size={13} />}
              onClick={async () => {
                const pw = await prompt({ title: t("backup.importTitle"), message: t("backup.importMsg"), label: t("backup.password"), password: true });
                if (pw) await act("import", () => API.importBackup(pw), t("backup.restored"));
              }}
            >
              {t("backup.import")}
            </Button>
          </div>
        }
      >
        {loading && !data ? (
          <div className="empty" style={{ padding: 20 }}>
            <Spinner />
          </div>
        ) : (data?.length ?? 0) === 0 ? (
          <Empty title={t("backup.empty")} icon={<Archive size={22} />} />
        ) : (
          data!.map((b) => (
            <div key={`${b.remote}-${b.name}`} className="setting">
              {b.remote ? <Cloud size={16} className="muted" /> : <HardDrive size={16} className="muted" />}
              <div className="setting-text">
                <div className="setting-label">
                  {dateTime(b.created)}
                  <Badge tone={b.remote ? "info" : undefined}>{b.remote ? "WebDAV" : t("backup.device")}</Badge>
                </div>
                <div className="setting-desc">
                  {b.device} · {bytes(b.size)}
                </div>
              </div>
              <Button
                size="sm"
                icon={<RotateCcw size={13} />}
                onClick={async () => {
                  if (await confirm({ title: t("backup.restoreTitle"), message: t("backup.restoreMsg"), confirm: t("backup.restore") })) await act("restore", () => API.restoreBackup(b.name, b.remote), t("backup.restored"));
                }}
              >
                {t("backup.restore")}
              </Button>
              <Button size="sm" variant="ghost" icon={<Download size={13} />} onClick={() => exportOne(b)} tip={t("backup.export")} />
              <Button
                size="sm"
                variant="ghost"
                icon={<Trash2 size={13} />}
                onClick={async () => {
                  if (await confirm({ title: t("backup.deleteTitle"), message: dateTime(b.created), confirm: t("common.delete"), danger: true })) await act("delete", () => API.deleteBackup(b.name, b.remote), t("backup.deleted"));
                }}
              />
            </div>
          ))
        )}
        {!!error && <div className="setting" style={{ color: "var(--danger)", fontSize: 12 }}>{String(error instanceof Error ? error.message : error)}</div>}
      </Section>
      <Section title={t("backup.auto")}>
        <Row label={t("backup.interval")} desc={t("backup.intervalDesc")}>
          <Select
            value={s.backup.autoIntervalHours}
            onChange={(v) => patch({ backup: { autoIntervalHours: v } })}
            options={[0, 6, 12, 24, 72, 168].map((h) => ({ value: h, label: h === 0 ? t("common.off") : h < 24 ? t("backup.everyHours", { n: h }) : t("backup.everyDays", { n: h / 24 }) }))}
            width={150}
          />
        </Row>
        <Row label={t("backup.keep")}>
          <NumberInput value={s.backup.keep} onChange={(v) => patch({ backup: { keep: v } })} min={1} max={100} width={80} />
        </Row>
      </Section>
    </>
  );
}

export default function SyncTab() {
  const t = useT();
  const lang = useApp((s) => s.lang);
  const sync = useApp((s) => s.sync);
  const s = useApp((st) => st.settings!);
  const patch = usePatch();
  const [setup, setSetup] = useState(false);
  const [syncing, setSyncing] = useState(false);
  useNow(15000);
  const { data: devices, reload: reloadDevices } = useAsync(() => (sync?.configured ? API.devices() : Promise.resolve([])), [sync?.configured, sync?.lastSync]);
  const cats = useMemo(() => new Set(s.sync.categories), [s.sync.categories]);
  if (!sync) return <Spinner />;
  const syncNow = async () => {
    setSyncing(true);
    const r = await run(() => API.now(), t("sync.failed"));
    setSyncing(false);
    if (r) {
      toast({ level: "success", message: t("sync.done"), detail: t("sync.report", { pushed: r.pushed.length, pulled: r.pulled.length, merged: r.merged.length }) });
      void reloadDevices();
    }
  };
  return (
    <>
      {!sync.configured ? (
        <div className="card sync-intro cloud-upload-hover">
          <div className="sync-intro-icon">
            <CloudUpload size={18} strokeWidth={1.8} />
          </div>
          <div className="grow sync-intro-text">
            <h2>{t("sync.heroTitle")}</h2>
            <p>{t("sync.heroDesc")}</p>
            <p className="sync-intro-facts">{[t("sync.point1"), t("sync.point2"), t("sync.point3")].join(" · ")}</p>
          </div>
          <Button variant="primary" onClick={() => setSetup(true)}>
            {t("sync.setUp")}
          </Button>
        </div>
      ) : (
        <>
          {sync.lastError && <Banner tone="danger">{sync.lastError}</Banner>}
          <Card
            title={t("sync.title")}
            icon={sync.enabled ? <Cloud size={15} /> : <CloudOff size={15} />}
            actions={
              <>
                <Button size="sm" variant="ghost" onClick={() => setSetup(true)}>
                  {t("sync.edit")}
                </Button>
                <Button size="sm" variant="primary" icon={<RefreshCw size={13} className={sync.running ? "spin" : ""} />} loading={syncing} disabled={sync.running} onClick={syncNow}>
                  {t("sync.now")}
                </Button>
              </>
            }
          >
            <div className="sync-stats">
              <div className="stat">
                <span className="stat-label">{t("sync.status")}</span>
                <span style={{ fontWeight: 620 }}>
                  {sync.running ? t("sync.running") : sync.enabled ? (sync.pending ? t("sync.pending") : t("sync.upToDate")) : t("sync.paused")}
                </span>
              </div>
              <div className="stat">
                <span className="stat-label">{t("sync.last")}</span>
                <span style={{ fontWeight: 620 }}>{sync.lastSync ? relative(sync.lastSync, lang) : t("sync.never")}</span>
              </div>
              <div className="stat">
                <span className="stat-label">{t("sync.next")}</span>
                <span style={{ fontWeight: 620 }}>{sync.nextSync ? relative(sync.nextSync, lang) : "—"}</span>
              </div>
              <div className="stat">
                <span className="stat-label">{t("sync.devices")}</span>
                <span style={{ fontWeight: 620 }}>{devices?.length ?? sync.devices}</span>
              </div>
            </div>
            {sync.report && (
              <div className="muted" style={{ fontSize: 12, marginTop: 8 }}>
                {t("sync.report", { pushed: sync.report.pushed.length, pulled: sync.report.pulled.length, merged: sync.report.merged.length })}
              </div>
            )}
          </Card>

          {sync.conflicts.length > 0 && (
            <Section title={t("sync.conflicts", { n: sync.conflicts.length })}>
              {sync.conflicts.map((c) => (
                <div key={c.path} className="setting" style={{ flexWrap: "wrap" }}>
                  <div className="setting-text">
                    <div className="setting-label">{sync.labels[c.path] ?? c.path}</div>
                    <div className="setting-desc">
                      {t("sync.conflictDesc", { here: relative(c.localModified, lang), there: relative(c.remoteModified, lang), device: c.remoteDevice })}
                    </div>
                  </div>
                  <Button size="sm" onClick={() => run(() => API.resolve(c.path, "local"), t("common.failed"))}>
                    {t("sync.keepLocal")}
                  </Button>
                  <Button size="sm" onClick={() => run(() => API.resolve(c.path, "remote"), t("common.failed"))}>
                    {t("sync.keepRemote")}
                  </Button>
                  <Button size="sm" variant="primary" onClick={() => run(() => API.resolve(c.path, "both"), t("common.failed"))}>
                    {t("sync.keepBoth")}
                  </Button>
                </div>
              ))}
            </Section>
          )}

          <Section title={t("sync.options")}>
            <Row label={t("sync.enabled")} desc={t("sync.enabledDesc")}>
              <Switch checked={s.sync.enabled} onChange={(v) => patch({ sync: { enabled: v } })} />
            </Row>
            <Row label={t("sync.interval")}>
              <Select
                value={s.sync.intervalMinutes}
                onChange={(v) => patch({ sync: { intervalMinutes: v } })}
                options={[0, 15, 30, 60, 180, 720].map((m) => ({ value: m, label: m === 0 ? t("sync.manual") : t("sync.everyMinutes", { n: m }) }))}
                width={150}
              />
            </Row>
            <Row label={t("sync.onChange")} desc={t("sync.onChangeDesc")}>
              <Switch checked={s.sync.onChange} onChange={(v) => patch({ sync: { onChange: v } })} />
            </Row>
            <Row label={t("sync.policy")} desc={t("sync.policyDesc")}>
              <Select
                value={s.sync.conflictPolicy}
                onChange={(v) => patch({ sync: { conflictPolicy: v } })}
                options={["ask", "newest", "local", "remote"].map((p) => ({ value: p, label: t(`sync.policy.${p}` as never) }))}
                width={170}
              />
            </Row>
            <Row label={t("sync.what")} desc={t("sync.whatDesc")}>
              <div className="row" style={{ gap: 6 }}>
                {["profiles", "settings", "dns", "tailscale"].map((c) => (
                  <button
                    key={c}
                    className={`chip${cats.has(c) ? " active" : ""}`}
                    onClick={() => patch({ sync: { categories: cats.has(c) ? s.sync.categories.filter((x) => x !== c) : [...s.sync.categories, c] } })}
                  >
                    {t(`sync.cat.${c}` as never)}
                  </button>
                ))}
              </div>
            </Row>
          </Section>

          <Section title={t("sync.security")}>
            <Row label={t("sync.encryption")} desc={t("sync.encryptionDesc")} icon={<ShieldCheck size={16} color="var(--success)" />}>
              <Badge tone="success" tip={sync.keyId ? `key ${sync.keyId}` : undefined}>
                E2EE
              </Badge>
            </Row>
            <Row label={t("sync.transport")} desc={s.sync.allowInsecure ? t("sync.transportHttp") : s.sync.pinnedKey ? t("sync.transportPinned") : t("sync.transportTls")}>
              <Badge tone={s.sync.allowInsecure ? "warning" : "success"}>{s.sync.allowInsecure ? "HTTP" : s.sync.pinnedKey ? "TLS · pinned" : "TLS"}</Badge>
            </Row>
            <Row label={t("sync.changePass")} desc={t("sync.changePassDesc")} icon={<KeyRound size={16} />}>
              <Button
                size="sm"
                onClick={async () => {
                  const p1 = await prompt({ title: t("sync.changePass"), label: t("sync.newPass"), password: true });
                  if (!p1) return;
                  if (strength(p1) < 1) return toast({ level: "warning", message: t("sync.tooShort") });
                  const p2 = await prompt({ title: t("sync.changePass"), label: t("sync.passphrase2"), password: true });
                  if (p1 !== p2) return toast({ level: "warning", message: t("sync.mismatch") });
                  await run(() => API.changePassphrase(p1), t("common.failed"), t("sync.passChanged"));
                }}
              >
                {t("common.change")}
              </Button>
            </Row>
            <Row label={t("sync.disconnect")} desc={t("sync.disconnectDesc")}>
              <Button
                size="sm"
                variant="danger"
                onClick={async () => {
                  if (await confirm({ title: t("sync.disconnect"), message: t("sync.disconnectMsg"), confirm: t("sync.disconnect"), danger: true })) await run(() => API.disable(true), t("common.failed"));
                }}
              >
                {t("sync.disconnect")}
              </Button>
            </Row>
          </Section>

          {(devices?.length ?? 0) > 0 && (
            <Section title={t("sync.deviceList")}>
              {devices!.map((d) => (
                <div key={d.id} className="setting">
                  <Laptop size={16} className="muted" />
                  <div className="setting-text">
                    <div className="setting-label">
                      {d.name}
                      {d.this && <Badge tone="accent">{t("sync.thisDevice")}</Badge>}
                    </div>
                    <div className="setting-desc">
                      {d.os} · {t("sync.lastSeen", { when: relative(d.lastSync, lang) })}
                    </div>
                  </div>
                </div>
              ))}
            </Section>
          )}
        </>
      )}
      <Backups />
      <SetupDialog open={setup} onClose={() => setSetup(false)} />
    </>
  );
}
