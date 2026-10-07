import { Cloud, Network, Settings2, SlidersHorizontal, Wrench } from "lucide-react";
import { useRef } from "react";
import { PageHeader } from "../../components/Page";
import { useT } from "../../lib/i18n";
import { useApp } from "../../lib/store";
import { Spinner, Tabs } from "../../ui";
import Advanced from "./Advanced";
import Clash from "./Clash";
import General from "./General";
import NetworkTab from "./Network";
import SyncTab from "./Sync";

export default function Settings() {
  const t = useT();
  const tab = useApp((s) => s.settingsTab);
  const navigate = useApp((s) => s.navigate);
  const settings = useApp((s) => s.settings);
  const conflicts = useApp((s) => s.sync?.conflicts.length ?? 0);
  const switched = useRef(false);
  return (
    <>
      <PageHeader title={t("nav.settings")} />
      <div className="page-body">
        <div className="settings-wrap">
          <Tabs
            value={tab}
            onChange={(v) => {
              switched.current = true;
              navigate("settings", v);
            }}
            tabs={[
              { value: "general", label: t("settings.tab.general"), icon: <Settings2 size={14} /> },
              { value: "network", label: t("settings.tab.network"), icon: <Network size={14} /> },
              { value: "clash", label: t("settings.tab.clash"), icon: <SlidersHorizontal size={14} /> },
              { value: "sync", label: `${t("settings.tab.sync")}${conflicts ? ` (${conflicts})` : ""}`, icon: <Cloud size={14} /> },
              { value: "advanced", label: t("settings.tab.advanced"), icon: <Wrench size={14} /> },
            ]}
          />
          {!settings ? (
            <div className="empty">
              <Spinner />
            </div>
          ) : (
            // A tab switched to rises in, as a page does; the first one comes with the page.
            <div key={tab} className={switched.current ? "tab-pane" : undefined}>
              {tab === "network" ? (
                <NetworkTab />
              ) : tab === "clash" ? (
                <Clash />
              ) : tab === "sync" ? (
                <SyncTab />
              ) : tab === "advanced" ? (
                <Advanced />
              ) : (
                <General />
              )}
            </div>
          )}
        </div>
      </div>
    </>
  );
}
