import { Laptop, Monitor, MoreHorizontal, Pencil, Server, Trash2, Waypoints } from "lucide-react";
import { useMemo, useState } from "react";
import { relative } from "../../lib/format";
import { useAsync } from "../../lib/hooks";
import { useT } from "../../lib/i18n";
import { run, useApp } from "../../lib/store";
import { Sync as API, type DeviceInfo, type Event as SyncEvent, type SyncActivity } from "../../mygo";
import { Badge, Banner, Button, Collapse, Dialog, Field, Input, Menu, prompt, Section, Segmented, Select, Switch } from "../../ui";

const DAY = 24 * 3600 * 1000;

function osName(os: string): string {
  return ({ darwin: "macOS", windows: "Windows", linux: "Linux" } as Record<string, string>)[os] ?? os;
}

function OSIcon({ os }: { os: string }) {
  if (os === "windows") return <Monitor size={16} className="muted" />;
  if (os === "linux") return <Server size={16} className="muted" />;
  return <Laptop size={16} className="muted" />;
}

export function deviceName(d: DeviceInfo): string {
  return d.label || d.name;
}

/** usePathLabel names a path of the vault for people. */
function usePathLabel() {
  const t = useT();
  return (path: string, label?: string): string => {
    if (label) return label;
    if (path === "settings") return t("sync.cat.settings");
    if (path === "dns") return t("sync.cat.dns");
    if (path === "tailscale") return t("sync.cat.tailscale");
    if (path === "tailscale/authkey") return t("sync.path.tsKey");
    if (path === "profiles/order") return t("sync.path.order");
    if (path.startsWith("usage/")) return t("sync.cat.stats");
    return path;
  };
}

/** meaningful leaves out the statistics, which every device sends hour by hour. */
function meaningful(c: { path: string }): boolean {
  return !c.path.startsWith("usage/");
}

/** listOf joins the first few names and says how many more. */
function useListOf() {
  const t = useT();
  return (names: string[]): string => {
    const uniq = [...new Set(names)];
    const shown = uniq.slice(0, 3).join(t("sync.listSep"));
    return uniq.length > 3 ? t("sync.andMore", { items: shown, n: uniq.length - 3 }) : shown;
  };
}

/** RemoveDialog takes devices out of sync, and with them, optionally, the key they hold. */
function RemoveDialog({ devices, all, plain, onClose }: { devices: DeviceInfo[]; all: DeviceInfo[]; plain: boolean; onClose: () => void }) {
  const t = useT();
  const [revoke, setRevoke] = useState(false);
  const [pass, setPass] = useState("");
  const [pass2, setPass2] = useState("");
  const [busy, setBusy] = useState(false);
  const open = devices.length > 0;
  const ids = new Set(devices.map((d) => d.id));
  // The devices that stay and cannot follow a new key by themselves.
  const behind = all.filter((d) => !ids.has(d.id) && !d.this && !d.hasKey);
  const ok = !revoke || (pass.length >= 8 && pass === pass2);
  const title = devices.length === 1 ? t("sync.removeTitle", { name: deviceName(devices[0]!) }) : t("sync.removeTitleN", { n: devices.length });
  const submit = async () => {
    setBusy(true);
    const done = await run(
      async () => (await API.removeDevices([...ids], revoke, revoke ? pass : ""), true),
      t("common.failed"),
      revoke ? t("sync.revoked") : t("sync.devicesRemoved"),
    );
    setBusy(false);
    if (done) {
      setRevoke(false);
      setPass("");
      setPass2("");
      onClose();
    }
  };
  return (
    <Dialog
      open={open}
      onClose={onClose}
      size="narrow"
      title={title}
      icon={<Trash2 size={16} />}
      footer={
        <>
          <Button onClick={onClose}>{t("common.cancel")}</Button>
          <Button variant="danger" loading={busy} disabled={!ok} onClick={submit}>
            {t("sync.remove")}
          </Button>
        </>
      }
    >
      <div className="form">
        <p className="muted" style={{ margin: 0 }}>
          {t("sync.removeMsg")}
        </p>
        {!plain && (
          <>
            <div className="setting" style={{ padding: 0, border: 0 }}>
              <div className="setting-text">
                <div className="setting-label">{t("sync.revoke")}</div>
                <div className="setting-desc">{t("sync.revokeDesc")}</div>
              </div>
              <Switch checked={revoke} onChange={setRevoke} />
            </div>
            <Collapse open={revoke}>
              <div className="form">
                <Field label={t("sync.newPass")}>
                  <Input type="password" value={pass} onChange={(e) => setPass(e.target.value)} />
                </Field>
                <Field label={t("sync.passphrase2")} error={pass2 && pass !== pass2 ? t("sync.mismatch") : pass && pass.length < 8 ? t("sync.tooShort") : undefined}>
                  <Input type="password" value={pass2} onChange={(e) => setPass2(e.target.value)} />
                </Field>
                {behind.length > 0 && <Banner tone="warning">{t("sync.revokeBehind", { names: behind.map(deviceName).join(t("sync.listSep")) })}</Banner>}
              </div>
            </Collapse>
          </>
        )}
      </div>
    </Dialog>
  );
}

export function DeviceList({ devices, plain, reload }: { devices: DeviceInfo[]; plain: boolean; reload: () => void }) {
  const t = useT();
  const lang = useApp((s) => s.lang);
  const [removing, setRemoving] = useState<DeviceInfo[]>([]);
  const dups = devices.filter((d) => d.duplicate);
  const rename = async (d: DeviceInfo) => {
    const v = await prompt({ title: t("sync.renameTitle"), label: t("sync.renameLabel"), value: deviceName(d), placeholder: d.name });
    if (v === null) return;
    const label = v.trim() === d.name ? "" : v.trim();
    await run(() => API.renameDevice(d.id, label), t("common.failed"));
    reload();
  };
  return (
    <>
      <Section
        title={t("sync.deviceList")}
        extra={
          dups.length > 0 && (
            <Button size="sm" icon={<Trash2 size={13} />} onClick={() => setRemoving(dups)}>
              {t("sync.cleanDuplicates", { n: dups.length })}
            </Button>
          )
        }
      >
        {devices.map((d) => {
          const idle = Date.now() - new Date(d.lastSync).getTime();
          const stale = !d.this && idle > 30 * DAY;
          const facts = [osName(d.os), d.arch, d.version && `v${d.version}`, t("sync.lastSeen", { when: relative(d.lastSync, lang) })].filter(Boolean);
          return (
            <div key={d.id} className={`setting sync-device${stale || d.duplicate ? " is-stale" : ""}`}>
              <OSIcon os={d.os} />
              <div className="setting-text">
                <div className="setting-label">
                  {deviceName(d)}
                  {d.this && <Badge tone="accent">{t("sync.thisDevice")}</Badge>}
                  {d.duplicate && (
                    <Badge tone="warning" tip={t("sync.duplicateTip")}>
                      {t("sync.duplicate")}
                    </Badge>
                  )}
                  {d.tailnet && (
                    <span className="sync-device-tailnet" title={t("sync.tailnetTip")}>
                      <Waypoints size={12} />
                    </span>
                  )}
                </div>
                <div className="setting-desc">
                  {d.label && d.label !== d.name && <span className="mono">{d.name} · </span>}
                  {facts.join(" · ")}
                </div>
              </div>
              <Menu
                trigger={(open) => <Button size="sm" variant="ghost" icon={<MoreHorizontal size={14} />} onClick={open} tip={t("sync.manage")} />}
                items={[
                  { label: t("sync.rename"), icon: <Pencil size={14} />, onClick: () => void rename(d) },
                  ...(d.this ? [] : [{ separator: true }, { label: t("sync.remove"), icon: <Trash2 size={14} />, danger: true, onClick: () => setRemoving([d]) }]),
                ]}
              />
            </div>
          );
        })}
      </Section>
      <RemoveDialog
        devices={removing}
        all={devices}
        plain={plain}
        onClose={() => {
          setRemoving([]);
          reload();
        }}
      />
    </>
  );
}

function ActivityRow({ when, who, children, error }: { when: string; who?: string; children: React.ReactNode; error?: boolean }) {
  const lang = useApp((s) => s.lang);
  return (
    <div className={`sync-act${error ? " is-error" : ""}`}>
      <span className="sync-act-when faint tnum" title={new Date(when).toLocaleString()}>
        {relative(when, lang)}
      </span>
      {who && <span className="sync-act-who">{who}</span>}
      <span className="sync-act-what">{children}</span>
    </div>
  );
}

export function SyncActivityList({ configured, lastSync }: { configured: boolean; lastSync?: string }) {
  const t = useT();
  const label = usePathLabel();
  const listOf = useListOf();
  const [scope, setScope] = useState<"all" | "here">("all");
  const [who, setWho] = useState("");
  const [more, setMore] = useState(false);
  const { data: acts } = useAsync(() => (configured ? API.activity() : Promise.resolve([] as SyncActivity[])), [configured, lastSync]);
  const { data: hist } = useAsync(() => API.history(), [lastSync]);
  const describe = (a: SyncActivity): string => {
    switch (a.kind) {
      case "join":
        return t("sync.act.join");
      case "rename":
        return a.detail ? t("sync.act.rename", { target: a.target ?? "", name: a.detail }) : t("sync.act.renameReset", { target: a.target ?? "" });
      case "remove":
        return t("sync.act.remove", { target: a.target || a.detail || "" });
      case "rotate":
        return t("sync.act.rotate");
    }
    const changes = a.changes.filter(meaningful);
    const sent = changes.filter((c) => !c.deleted).map((c) => label(c.path, c.label));
    const gone = changes.filter((c) => c.deleted).map((c) => label(c.path, c.label));
    return [sent.length > 0 && t("sync.act.sent", { items: listOf(sent) }), gone.length > 0 && t("sync.act.deleted", { items: listOf(gone) })].filter(Boolean).join(t("sync.listSep"));
  };
  const name = (p: string) => {
    const [path, l] = p.split("\0");
    return label(path!, l);
  };
  const summary = (e: SyncEvent): string => {
    const parts: string[] = [];
    const keep = (l?: string[]) => l?.filter((p) => !p.startsWith("usage/"));
    e = { ...e, pushed: keep(e.pushed), pulled: keep(e.pulled), merged: keep(e.merged) };
    if (e.pushed?.length) parts.push(t("sync.hist.pushed", { items: listOf(e.pushed.map(name)) }));
    if (e.pulled?.length) parts.push(t("sync.hist.pulled", { items: listOf(e.pulled.map(name)) }));
    if (e.merged?.length) parts.push(t("sync.hist.merged", { items: listOf(e.merged.map(name)) }));
    if (e.conflicts) parts.push(t("sync.conflicts", { n: e.conflicts }));
    return parts.join(t("sync.listSep"));
  };
  const rows = useMemo(() => {
    const list =
      scope === "all"
        ? (acts ?? []).filter((a) => (!who || a.deviceId === who) && (a.kind !== "sync" || a.changes.some(meaningful)))
        : (hist ?? []).filter((e) => e.error || e.conflicts || [...(e.pushed ?? []), ...(e.pulled ?? []), ...(e.merged ?? [])].some((p) => !p.startsWith("usage/")));
    return more ? list : list.slice(0, 8);
  }, [scope, acts, hist, more, who]);
  const total = scope === "all" ? (acts ?? []).filter((a) => (!who || a.deviceId === who) && (a.kind !== "sync" || a.changes.some(meaningful))).length : (hist?.length ?? 0);
  const people = useMemo(() => {
    const m = new Map<string, string>();
    for (const a of acts ?? []) if (!m.has(a.deviceId)) m.set(a.deviceId, a.this ? t("sync.thisDevice") : a.device);
    return [...m.entries()];
  }, [acts, t]);
  return (
    <Section
      title={t("sync.activity")}
      extra={
        <div className="row" style={{ gap: 8, textTransform: "none", letterSpacing: 0 }}>
          {scope === "all" && people.length > 1 && (
            <Select
              value={who}
              onChange={(v) => (setWho(v), setMore(false))}
              options={[{ value: "", label: t("sync.everyDevice") }, ...people.map(([id, name]) => ({ value: id, label: name }))]}
              width={150}
            />
          )}
          <Segmented
            value={scope}
            onChange={(v) => (setScope(v), setMore(false))}
            options={[
              { value: "all", label: t("sync.activityAll") },
              { value: "here", label: t("sync.activityHere") },
            ]}
          />
        </div>
      }
    >
      <div className="sync-acts">
        {rows.length === 0 ? (
          <div className="faint sync-acts-none">{t("sync.noActivity")}</div>
        ) : scope === "all" ? (
          (rows as SyncActivity[]).map((a, i) => (
            <ActivityRow key={`${a.at}-${i}`} when={a.at} who={a.this ? t("sync.thisDevice") : a.device}>
              {describe(a)}
            </ActivityRow>
          ))
        ) : (
          (rows as SyncEvent[]).map((e, i) => (
            <ActivityRow key={`${e.at}-${i}`} when={e.at} error={!!e.error}>
              {e.error || summary(e)}
            </ActivityRow>
          ))
        )}
        {!more && total > 8 && (
          <button className="sync-acts-more" onClick={() => setMore(true)}>
            {t("sync.showMore", { n: total - 8 })}
          </button>
        )}
      </div>
    </Section>
  );
}

/** KeyReplaced asks for the new passphrase after another device replaced the key. */
export function KeyReplaced() {
  const t = useT();
  return (
    <Banner
      tone="warning"
      action={
        <Button
          size="sm"
          onClick={async () => {
            const p = await prompt({ title: t("sync.enterNewPass"), label: t("sync.passphrase"), password: true });
            if (p) await run(() => API.rejoin(p), t("common.failed"), t("sync.rejoined"));
          }}
        >
          {t("sync.enterNewPass")}
        </Button>
      }
    >
      {t("sync.keyReplaced")}
    </Banner>
  );
}
