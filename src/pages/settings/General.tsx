import { Keyboard, LayoutPanelLeft, Paintbrush } from "lucide-react";
import { lazy, Suspense, useEffect, useState } from "react";
import { useAsync } from "../../lib/hooks";
import { LANGUAGES, useT } from "../../lib/i18n";
import { type DeepPartial, patchSettings, run, toast, useApp, PAGES } from "../../lib/store";
import { App, type Settings } from "../../mygo";
import { Button, Dialog, Input, Row, Section, Segmented, Select, Spinner, Switch } from "../../ui";

const CodeEditor = lazy(() => import("../../components/CodeEditor"));

// The accents: MyGO's own, which follows the theme, then one per member,
// toned to read in both themes.
const ACCENTS = [
  { value: "", name: "accent.mygo" },
  { value: "#4fa3d1", name: "accent.tomori" },
  { value: "#f07891", name: "accent.anon" },
  { value: "#4fbf6b", name: "accent.raana" },
  { value: "#e8b84a", name: "accent.soyo" },
  { value: "#7d7bc0", name: "accent.taki" },
] as const;

export function usePatch() {
  const t = useT();
  return (p: DeepPartial<Settings>) => run(() => patchSettings(p), t("settings.saveFailed"));
}

function Hotkeys({ open, onClose }: { open: boolean; onClose: () => void }) {
  const t = useT();
  const settings = useApp((s) => s.settings!);
  const [bindings, setBindings] = useState<Record<string, string>>(settings.hotkeys.bindings);
  const [recording, setRecording] = useState<string | null>(null);
  useEffect(() => setBindings(settings.hotkeys.bindings), [open, settings.hotkeys.bindings]);
  useEffect(() => {
    if (!recording) return;
    const onKey = (e: KeyboardEvent) => {
      e.preventDefault();
      if (e.key === "Escape") return setRecording(null);
      if (e.key === "Backspace" || e.key === "Delete") {
        setBindings((b) => ({ ...b, [recording]: "" }));
        return setRecording(null);
      }
      if (["Meta", "Control", "Alt", "Shift"].includes(e.key)) return;
      const mods = [e.metaKey || e.ctrlKey ? "CmdOrCtrl" : "", e.altKey ? "Alt" : "", e.shiftKey ? "Shift" : ""].filter(Boolean);
      if (mods.length === 0) return;
      let key = e.code.startsWith("Key") ? e.code.slice(3) : e.code.startsWith("Digit") ? e.code.slice(5) : e.key.length === 1 ? e.key.toUpperCase() : e.key;
      if (key === " ") key = "Space";
      setBindings((b) => ({ ...b, [recording]: [...mods, key].join("+") }));
      setRecording(null);
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [recording]);
  const actions = ["toggle-window", "quick-panel", "toggle-system-proxy", "toggle-tun", "mode-rule", "mode-global", "mode-direct", "lightweight", "reactivate-profile"];
  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={t("settings.hotkeys")}
      footer={
        <>
          <Button onClick={onClose}>{t("common.cancel")}</Button>
          <Button
            variant="primary"
            onClick={async () => {
              await run(() => patchSettings({ hotkeys: { bindings } }), t("settings.saveFailed"));
              onClose();
            }}
          >
            {t("common.save")}
          </Button>
        </>
      }
    >
      <div className="muted" style={{ fontSize: 12.5, marginBottom: 10 }}>
        {t("settings.hotkeysHint")}
      </div>
      <div className="rows">
        {actions.map((a) => (
          <div key={a} className="setting">
            <div className="setting-text">{t(`hotkey.${a}` as never)}</div>
            <button className={`chip mono${recording === a ? " active" : ""}`} style={{ minWidth: 140, justifyContent: "center" }} onClick={() => setRecording(a)}>
              {recording === a ? t("settings.pressKeys") : bindings[a] || t("settings.notSet")}
            </button>
          </div>
        ))}
      </div>
    </Dialog>
  );
}

function Layout({ open, onClose }: { open: boolean; onClose: () => void }) {
  const t = useT();
  const s = useApp((st) => st.settings!);
  const patch = usePatch();
  const nav = s.ui.nav;
  return (
    <Dialog open={open} onClose={onClose} title={t("settings.layout")}>
      <div className="rows">
        <Row label={t("settings.trafficGraph")}>
          <Switch checked={s.ui.trafficGraph} onChange={(v) => patch({ ui: { trafficGraph: v } })} />
        </Row>
        <Row label={t("settings.memoryUsage")}>
          <Switch checked={s.ui.memoryUsage} onChange={(v) => patch({ ui: { memoryUsage: v } })} />
        </Row>
        <Row label={t("settings.groupIcons")}>
          <Switch checked={s.ui.groupIcons} onChange={(v) => patch({ ui: { groupIcons: v } })} />
        </Row>
        <Row label={t("settings.collapseNav")}>
          <Switch checked={s.ui.collapseNav} onChange={(v) => patch({ ui: { collapseNav: v } })} />
        </Row>
        <Row label={t("settings.proxyColumns")} desc={t("settings.proxyColumnsHint")}>
          <Select value={s.ui.proxyColumns} onChange={(v) => patch({ ui: { proxyColumns: v } })} options={[0, 1, 2, 3, 4, 5, 6].map((n) => ({ value: n, label: n === 0 ? t("settings.auto") : String(n) }))} width={110} />
        </Row>
        <Row label={t("settings.toastPosition")}>
          <Select
            value={s.ui.toastPosition}
            onChange={(v) => patch({ ui: { toastPosition: v } })}
            options={["top-right", "top-left", "bottom-right", "bottom-left"].map((p) => ({ value: p, label: t(`settings.pos.${p}` as never) }))}
            width={140}
          />
        </Row>
        <Row label={t("settings.pauseOnBlur")}>
          <Switch checked={s.ui.pauseOnBlur} onChange={(v) => patch({ ui: { pauseOnBlur: v } })} />
        </Row>
      </div>
      <div className="section-title" style={{ marginTop: 16 }}>
        {t("settings.navItems")}
      </div>
      <div className="row" style={{ flexWrap: "wrap", gap: 6 }}>
        {PAGES.map((p) => {
          const on = nav.includes(p);
          return (
            <button
              key={p}
              className={`chip${on ? " active" : ""}`}
              disabled={p === "settings"}
              onClick={() => patch({ ui: { nav: on ? nav.filter((x) => x !== p) : PAGES.filter((x) => nav.includes(x) || x === p) } })}
            >
              {t(`nav.${p}`)}
            </button>
          );
        })}
      </div>
    </Dialog>
  );
}

function Appearance({ open, onClose }: { open: boolean; onClose: () => void }) {
  const t = useT();
  const s = useApp((st) => st.settings!);
  const [css, setCss] = useState(s.customCss);
  const [font, setFont] = useState(s.fontFamily);
  useEffect(() => {
    setCss(s.customCss);
    setFont(s.fontFamily);
  }, [open, s.customCss, s.fontFamily]);
  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={t("settings.appearance")}
      size="wide"
      footer={
        <>
          <Button onClick={onClose}>{t("common.cancel")}</Button>
          <Button
            variant="primary"
            onClick={async () => {
              await run(() => patchSettings({ customCss: css, fontFamily: font }), t("settings.saveFailed"));
              onClose();
            }}
          >
            {t("common.save")}
          </Button>
        </>
      }
    >
      <div className="form">
        <div className="field">
          <label>{t("settings.font")}</label>
          <Input value={font} onChange={(e) => setFont(e.target.value)} placeholder="Inter, system-ui" />
        </div>
        <div className="field" style={{ height: 320 }}>
          <label>{t("settings.customCss")}</label>
          <div style={{ border: "1px solid var(--border)", borderRadius: 8, overflow: "hidden", height: "100%", display: "flex" }}>
            <Suspense fallback={<Spinner />}>
              <CodeEditor value={css} onChange={setCss} lang="css" />
            </Suspense>
          </div>
        </div>
      </div>
    </Dialog>
  );
}

export default function General() {
  const t = useT();
  const s = useApp((st) => st.settings!);
  const info = useApp((st) => st.info);
  const patch = usePatch();
  const { data: autostart, reload } = useAsync(() => App.autoLaunch(), []);
  const [dialog, setDialog] = useState<"" | "hotkeys" | "layout" | "appearance">("");
  return (
    <>
      <Section title={t("settings.interface")}>
        <Row label={t("settings.language")}>
          <Select
            value={s.language}
            onChange={(v) => patch({ language: v })}
            options={[{ value: "", label: t("settings.followSystem") }, ...LANGUAGES]}
            width={190}
          />
        </Row>
        <Row label={t("settings.theme")}>
          <Segmented
            value={s.theme}
            onChange={(v) => patch({ theme: v })}
            options={[
              { value: "system", label: t("settings.followSystem") },
              { value: "light", label: t("settings.light") },
              { value: "dark", label: t("settings.dark") },
            ]}
          />
        </Row>
        <Row label={t("settings.accent")}>
          <div className="row" style={{ gap: 7 }}>
            {ACCENTS.map((c) => (
              <button
                key={c.name}
                className={`swatch${c.value ? "" : " mygo"}${s.accent.toLowerCase() === c.value ? " active" : ""}`}
                style={c.value ? { background: c.value } : undefined}
                onClick={() => patch({ accent: c.value })}
                aria-label={t(c.name)}
                data-tip={t(c.name)}
              />
            ))}
            <input
              type="color"
              className="swatch-input"
              value={s.accent || "#2a7ab0"}
              onChange={(e) => patch({ accent: e.target.value })}
              aria-label={t("accent.custom")}
              data-tip={t("accent.custom")}
            />
          </div>
        </Row>
        <Row label={t("settings.startPage")}>
          <Select value={s.startPage} onChange={(v) => patch({ startPage: v })} options={PAGES.map((p) => ({ value: p, label: t(`nav.${p}`) }))} width={150} />
        </Row>
        <Row label={t("settings.layout")} desc={t("settings.layoutDesc")} icon={<LayoutPanelLeft size={16} />} onClick={() => setDialog("layout")}>
          <Button size="sm">{t("common.edit")}</Button>
        </Row>
        <Row label={t("settings.appearance")} desc={t("settings.appearanceDesc")} icon={<Paintbrush size={16} />} onClick={() => setDialog("appearance")}>
          <Button size="sm">{t("common.edit")}</Button>
        </Row>
      </Section>

      <Section title={t("settings.startup")}>
        <Row label={t("settings.autoLaunch")} desc={t("settings.autoLaunchDesc")}>
          <Switch
            checked={!!autostart}
            onChange={(v) =>
              run(async () => {
                await App.setAutoLaunch(v);
                await reload();
              }, t("settings.saveFailed"))
            }
          />
        </Row>
        <Row label={t("settings.silentStart")} desc={t("settings.silentStartDesc")}>
          <Switch checked={s.silentStart} onChange={(v) => patch({ silentStart: v })} />
        </Row>
        <Row label={t("settings.notifications")} desc={t("settings.notificationsDesc")}>
          <Switch checked={s.notifications} onChange={(v) => patch({ notifications: v })} />
        </Row>
      </Section>

      <Section title={t("settings.tray")}>
        <Row label={t("settings.trayClick")}>
          <Select
            value={s.trayClick}
            onChange={(v) => patch({ trayClick: v })}
            options={[
              { value: "menu", label: t("settings.trayMenu") },
              { value: "panel", label: t("settings.trayPanel") },
              { value: "window", label: t("settings.trayWindow") },
              { value: "none", label: t("common.none") },
            ]}
            width={170}
          />
        </Row>
        <Row label={t("settings.trayGroups")}>
          <Select
            value={s.tray.groups}
            onChange={(v) => patch({ tray: { groups: v } })}
            options={[
              { value: "submenu", label: t("settings.traySubmenu") },
              { value: "inline", label: t("settings.trayInline") },
              { value: "off", label: t("settings.trayOff") },
            ]}
            width={170}
          />
        </Row>
        <Row label={t("settings.trayModes")}>
          <Switch checked={s.tray.inlineModes} onChange={(v) => patch({ tray: { inlineModes: v } })} />
        </Row>
        {info?.os === "darwin" && (
          <Row label={t("settings.traySpeed")} desc={t("settings.traySpeedDesc")}>
            <Switch checked={s.tray.showSpeed} onChange={(v) => patch({ tray: { showSpeed: v } })} />
          </Row>
        )}
      </Section>

      <Section title={t("settings.shortcuts")}>
        <Row label={t("settings.hotkeysEnabled")} desc={t("settings.hotkeysDesc")} icon={<Keyboard size={16} />}>
          <Button size="sm" onClick={() => setDialog("hotkeys")}>
            {t("common.edit")}
          </Button>
          <Switch checked={s.hotkeys.enabled} onChange={(v) => patch({ hotkeys: { enabled: v } })} />
        </Row>
        <Row label={t("settings.copyEnv")} desc={t("settings.copyEnvDesc")}>
          <Select
            value={s.copyEnvType}
            onChange={(v) => patch({ copyEnvType: v })}
            options={[
              { value: "posix", label: "bash / zsh" },
              { value: "fish", label: "fish" },
              { value: "powershell", label: "PowerShell" },
              { value: "cmd", label: "cmd" },
              { value: "nushell", label: "Nushell" },
            ]}
            width={140}
          />
          <Button size="sm" onClick={() => App.copyEnv().then((c) => toast({ level: "success", message: t("common.copied"), detail: c }))}>
            {t("common.copy")}
          </Button>
        </Row>
      </Section>
      <Hotkeys open={dialog === "hotkeys"} onClose={() => setDialog("")} />
      <Layout open={dialog === "layout"} onClose={() => setDialog("")} />
      <Appearance open={dialog === "appearance"} onClose={() => setDialog("")} />
    </>
  );
}

