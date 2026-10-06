import { Bug, Cpu, ExternalLink, Feather, FolderOpen, Info, KeyRound, LogOut, Power, RefreshCw, ScrollText } from "lucide-react";
import { useT } from "../../lib/i18n";
import { run, toast, useApp } from "../../lib/store";
import { App, Core } from "../../mygo";
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
        <Row label={t("adv.dataDir")} desc={info?.dataDir} icon={<FolderOpen size={16} />}>
          <Button size="sm" onClick={() => App.openDir("data")}>
            {t("common.open")}
          </Button>
        </Row>
        <Row label={t("adv.coreDir")} desc={info?.coreDir}>
          <Button size="sm" onClick={() => App.openDir("core")}>
            {t("common.open")}
          </Button>
        </Row>
        <Row label={t("adv.logsDir")} desc={info?.logsDir}>
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

      <Section title={t("adv.about")}>
        <Row label="MyGO-Clash" desc={`v${info?.version ?? ""} · mihomo ${info?.coreVersion ?? ""} · ${info?.os ?? ""}/${info?.arch ?? ""}`} icon={<Info size={16} />}>
          <Button size="sm" variant="ghost" onClick={() => App.copyText(`MyGO-Clash v${info?.version} (mihomo ${info?.coreVersion}, ${info?.os}/${info?.arch})`).then(() => toast({ level: "success", message: t("common.copied") }))}>
            {t("common.copy")}
          </Button>
        </Row>
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
