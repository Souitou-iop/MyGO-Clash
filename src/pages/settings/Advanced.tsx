import { Bug, Copy, Cpu, Download, ExternalLink, Feather, FolderOpen, KeyRound, LogOut, Power, RefreshCw, ScrollText } from "lucide-react";
import { useEffect, useState } from "react";
import iconDark from "../../assets/icon-dark.png";
import icon from "../../assets/icon.png";
import { Five } from "../../components/Art";
import { ltr, relative } from "../../lib/format";
import { useT } from "../../lib/i18n";
import { run, toast, useApp } from "../../lib/store";
import { App, Core, events, Updates, type UpdateState } from "../../mygo";
import { Badge, Button, NumberInput, Row, Section, Select, Switch } from "../../ui";
import { usePatch } from "./General";

export default function Advanced() {
  const t = useT();
  const s = useApp((st) => st.settings!);
  const info = useApp((st) => st.info);
  const state = useApp((st) => st.state);
  const patch = usePatch();
  return (
    <>
      <Section title={t("adv.core")}>
        <Row label={t("adv.coreMode")} desc={t("adv.coreModeDesc")} icon={<Cpu size={16} />}>
          <Select
            value={s.coreMode}
            onChange={(v) => patch({ coreMode: v })}
            options={[
              { value: "auto", label: t("adv.coreAuto") },
              { value: "sidecar", label: t("adv.coreSidecar") },
              { value: "service", label: t("adv.coreService") },
            ]}
            width={190}
          />
        </Row>
        <Row label={t("core.status")} desc={state ? `${t(`core.${state.core.status}` as never)} · ${state.core.mode} · PID ${state.core.pid ?? "—"} · mihomo ${state.core.coreVersion ?? info?.coreVersion ?? ""}` : ""}>
          <Button size="sm" icon={<RefreshCw size={13} />} onClick={() => run(() => Core.restart(), t("common.failed"), t("adv.restarting"))}>
            {t("core.restart")}
          </Button>
        </Row>
      </Section>

      <Section title={t("adv.lightweight")}>
        <Row label={t("adv.lightAuto")} desc={t("adv.lightAutoDesc")} icon={<Feather size={16} />}>
          {s.lightweight.autoEnter && (
            <>
              <NumberInput value={s.lightweight.delayMinutes} onChange={(v) => patch({ lightweight: { delayMinutes: v } })} min={1} max={1440} width={80} />
              <span className="muted">{t("common.minutes")}</span>
            </>
          )}
          <Switch checked={s.lightweight.autoEnter} onChange={(v) => patch({ lightweight: { autoEnter: v } })} />
        </Row>
        <Row label={t("adv.lightNow")} desc={t("adv.lightNowDesc")}>
          <Button size="sm" onClick={() => App.enterLightweight()}>
            {t("adv.enter")}
          </Button>
        </Row>
      </Section>

      <Section title={t("adv.logs")}>
        <Row label={t("adv.appLogLevel")} icon={<ScrollText size={16} />}>
          <Select value={s.logs.level} onChange={(v) => patch({ logs: { level: v } })} options={["debug", "info", "warn", "error"].map((l) => ({ value: l, label: l }))} width={120} />
        </Row>
        <Row label={t("adv.logSize")}>
          <NumberInput value={s.logs.maxSizeMb} onChange={(v) => patch({ logs: { maxSizeMb: v } })} min={1} max={512} width={80} />
          <span className="muted">MB ×</span>
          <NumberInput value={s.logs.maxFiles} onChange={(v) => patch({ logs: { maxFiles: v } })} min={1} max={50} width={70} />
        </Row>
        <Row label={t("adv.logClean")}>
          <Select
            value={s.logs.autoCleanDays}
            onChange={(v) => patch({ logs: { autoCleanDays: v } })}
            options={[0, 1, 7, 30, 90].map((d) => ({ value: d, label: d === 0 ? t("adv.never") : t("adv.keepDays", { n: d }) }))}
            width={150}
          />
        </Row>
      </Section>

      <Section title={t("adv.folders")}>
        <Row label={t("adv.dataDir")} desc={info && ltr(info.dataDir)} icon={<FolderOpen size={16} />}>
          <Button size="sm" onClick={() => App.openDir("data")}>
            {t("common.open")}
          </Button>
        </Row>
        <Row label={t("adv.coreDir")} desc={info && ltr(info.coreDir)}>
          <Button size="sm" onClick={() => App.openDir("core")}>
            {t("common.open")}
          </Button>
        </Row>
        <Row label={t("adv.logsDir")} desc={info && ltr(info.logsDir)}>
          <Button size="sm" onClick={() => App.openDir("logs")}>
            {t("common.open")}
          </Button>
        </Row>
      </Section>

      <Section title={t("adv.security")}>
        <Row label={t("adv.atRest")} desc={info?.keyringSecure ? t("adv.atRestSecure", { where: info.keyring }) : t("adv.atRestFile")} icon={<KeyRound size={16} />}>
          <Badge tone={info?.keyringSecure ? "success" : "warning"}>{info?.keyring}</Badge>
        </Row>
        <Row label={t("adv.diagnostics")} desc={t("adv.diagnosticsDesc")} icon={<Bug size={16} />}>
          <Button size="sm" onClick={() => run(() => App.diagnostics(), t("common.failed"), t("adv.diagnosticsCopied"))}>
            {t("common.copy")}
          </Button>
        </Row>
      </Section>

      <div className="section">
        <div className="section-title">{t("adv.about")}</div>
        <div className="card about-hero">
          <img className="about-icon light" src={icon} width={64} height={64} alt="" draggable={false} />
          <img className="about-icon dark" src={iconDark} width={64} height={64} alt="" draggable={false} />
          <div className="grow">
            <div className="about-name">
              <span className="brand-name">
                <i>MyGO</i>-Clash
              </span>
              <Five height={16} />
            </div>
            <div className="about-version selectable">
              v{info?.version ?? ""} · mihomo {info?.coreVersion ?? ""} · {info?.os ?? ""}/{info?.arch ?? ""}
            </div>
            <div className="about-motto">
              <span lang="ja" dir="ltr">迷子でもいい、前へ進め。</span>
              <span className="muted">{t("about.motto")}</span>
            </div>
          </div>
          <Button
            size="sm"
            icon={<Copy size={13} />}
            onClick={() => App.copyText(`MyGO-Clash v${info?.version} (mihomo ${info?.coreVersion}, ${info?.os}/${info?.arch})`).then(() => toast({ level: "success", message: t("common.copied") }))}
          >
            {t("common.copy")}
          </Button>
        </div>
        <div className="about-note faint">{t("about.homage")}</div>
      </div>

      <UpdatesSection />

      <Section title={t("adv.more")}>
        <Row label={t("adv.license")} desc={t("adv.licenseDesc")}>
          <Button size="sm" variant="ghost" icon={<ExternalLink size={13} />} onClick={() => App.openURL("https://github.com/MetaCubeX/mihomo")}>
            mihomo
          </Button>
          <Button size="sm" variant="ghost" icon={<ExternalLink size={13} />} onClick={() => App.openURL("https://github.com/egoist/mygo")}>
            MyGo
          </Button>
        </Row>
        <Row label={t("adv.restartApp")} icon={<Power size={16} />}>
          <Button size="sm" onClick={() => App.relaunch()}>
            {t("adv.restart")}
          </Button>
          <Button size="sm" variant="danger" icon={<LogOut size={13} />} onClick={() => App.quit()}>
            {t("adv.quit")}
          </Button>
        </Row>
      </Section>
    </>
  );
}

/**
 * UpdatesSection checks for new versions of the app, and keeps the choices
 * that Sparkle offers on the Mac: whether and how often to check, and
 * whether to install updates without asking.
 */
function UpdatesSection() {
  const t = useT();
  const lang = useApp((st) => st.lang);
  const s = useApp((st) => st.settings!);
  const patch = usePatch();
  const [u, setU] = useState<UpdateState>();
  useEffect(() => {
    void Updates.state().then(setU);
    return events.update.on(setU);
  }, []);
  if (!u) return null;
  const status = !u.supported
    ? t(u.reason === "dev" ? "updates.dev" : "updates.package")
    : u.checking
      ? t("updates.checking")
      : u.lastCheck
        ? t("updates.lastCheck", { when: relative(u.lastCheck, lang) })
        : t("updates.never");
  const auto = s.updates.autoCheck;
  return (
    <Section title={t("updates.title")}>
      <Row label={t("updates.check")} desc={status} icon={<Download size={16} />}>
        {!u.supported && u.reason !== "dev" && (
          <Button size="sm" variant="ghost" icon={<ExternalLink size={13} />} onClick={() => Updates.releasesURL().then(App.openURL)}>
            {t("updates.releases")}
          </Button>
        )}
        <Button size="sm" onClick={() => Updates.check()}>
          {t("updates.checkNow")}
        </Button>
      </Row>
      {u.ready && (
        <Row label={t("updates.ready", { v: u.ready })} desc={t("updates.readyDesc")}>
          <Button size="sm" variant="primary" icon={<RefreshCw size={13} />} onClick={() => App.relaunch()}>
            {t("updates.relaunch")}
          </Button>
        </Row>
      )}
      {u.supported && (
        <>
          <Row label={t("updates.auto")} desc={t("updates.autoDesc")}>
            <Switch checked={auto} onChange={(v) => patch({ updates: { autoCheck: v } })} />
          </Row>
          <Row label={t("updates.interval")}>
            <Select
              value={s.updates.interval}
              disabled={!auto}
              onChange={(v) => patch({ updates: { interval: v } })}
              options={(["hourly", "daily", "weekly", "monthly"] as const).map((i) => ({ value: i, label: t(`updates.${i}`) }))}
              width={150}
            />
          </Row>
          <Row label={t("updates.autoInstall")} desc={t("updates.autoInstallDesc")}>
            <Switch checked={u.automaticDownloads} disabled={!auto} onChange={(v) => run(() => Updates.setAutomaticDownloads(v), t("common.failed"))} />
          </Row>
        </>
      )}
    </Section>
  );
}
