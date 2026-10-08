import {
  ArrowDown,
  Activity,
  ArrowUp,
  Cable,
  ChartColumn,
  FileStack,
  Globe2,
  LayoutDashboard,
  ListFilter,
  type LucideIcon,
  PanelLeftClose,
  PanelLeftOpen,
  ScrollText,
  Settings as SettingsIcon,
  Waypoints,
} from "lucide-react";
import { lazy, Suspense, useEffect } from "react";
import { BrandMark } from "./components/Brand";
import { TrafficGraph } from "./components/TrafficGraph";
import { rate, shortRate } from "./lib/format";
import { useT } from "./lib/i18n";
import { enableTunWithService, offerServiceUpdate } from "./lib/service";
import { type Page, PAGES, patchSettings, run, useApp, useTraffic } from "./lib/store";
import { App as AppAPI, System } from "./mygo";
import Home from "./pages/Home";
import { AskHost, Button, SideTips, Spinner, Toasts } from "./ui";

const Proxies = lazy(() => import("./pages/Proxies"));
const Profiles = lazy(() => import("./pages/Profiles"));
const Connections = lazy(() => import("./pages/Connections"));
const Stats = lazy(() => import("./pages/Stats"));
const Rules = lazy(() => import("./pages/Rules"));
const Logs = lazy(() => import("./pages/Logs"));
const Tailscale = lazy(() => import("./pages/Tailscale"));
const Connectivity = lazy(() => import("./pages/Connectivity"));
const Settings = lazy(() => import("./pages/settings"));

const icons: Record<Page, LucideIcon> = {
  home: LayoutDashboard,
  proxies: Globe2,
  profiles: FileStack,
  connections: Cable,
  stats: ChartColumn,
  rules: ListFilter,
  logs: ScrollText,
  tailscale: Waypoints,
  connectivity: Activity,
  settings: SettingsIcon,
};

function Sidebar() {
  const t = useT();
  const page = useApp((s) => s.page);
  const navigate = useApp((s) => s.navigate);
  const settings = useApp((s) => s.settings);
  const state = useApp((s) => s.state);
  const sync = useApp((s) => s.sync);
  const ts = useApp((s) => s.tailscale);
  const traffic = useTraffic();
  const collapsed = settings?.ui.collapseNav ?? false;
  const nav = (settings?.ui.nav ?? PAGES).filter((p): p is Page => PAGES.includes(p as Page));
  const dots: Partial<Record<Page, boolean>> = {
    settings: !!state && (!!state.configError || (state.service.installed && state.service.outdated) || (sync?.conflicts.length ?? 0) > 0),
    tailscale: ts?.backendState === "NeedsLogin",
    profiles: !!state?.configError,
  };
  return (
    <aside className="sidebar">
      <div className="sidebar-top" />
      <div className="brand">
        <BrandMark />
        <span className="brand-name">
          <i>MyGO</i>-Clash
        </span>
      </div>
      <nav className="nav">
        {nav.map((p) => {
          const Icon = icons[p];
          return (
            <button key={p} className={`nav-item${page === p ? " active" : ""}`} onClick={() => navigate(p)} data-tip={collapsed ? t(`nav.${p}`) : undefined}>
              <Icon size={17} strokeWidth={page === p ? 2.2 : 1.9} />
              <span className="nav-label">{t(`nav.${p}`)}</span>
              {dots[p] && <span className="nav-dot" />}
            </button>
          );
        })}
      </nav>
      <div className="side-foot">
        {settings?.ui.trafficGraph !== false && <TrafficGraph up={traffic.up.slice(-40)} down={traffic.down.slice(-40)} height={34} minimal />}
        <div className="side-traffic">
          <div className="side-rate" title={`${t("common.upload")} ${rate(traffic.now.up)}`}>
            <ArrowUp size={13} color="var(--graph-up)" />
            <span className="side-num">
              <span className="side-traffic-text">{rate(traffic.now.up)}</span>
              <span className="side-traffic-short">{shortRate(traffic.now.up)}</span>
            </span>
          </div>
          <div className="side-rate" title={`${t("common.download")} ${rate(traffic.now.down)}`}>
            <ArrowDown size={13} color="var(--graph-down)" />
            <span className="side-num">
              <span className="side-traffic-text">{rate(traffic.now.down)}</span>
              <span className="side-traffic-short">{shortRate(traffic.now.down)}</span>
            </span>
          </div>
        </div>
        <Button
          variant="ghost"
          size="sm"
          icon={collapsed ? <PanelLeftOpen size={15} /> : <PanelLeftClose size={15} />}
          onClick={() => run(() => patchSettings({ ui: { collapseNav: !collapsed } }), t("common.failed"))}
          tip={collapsed ? t("nav.expand") : t("nav.collapse")}
        />
      </div>
      <SideTips />
    </aside>
  );
}

function PageView({ page }: { page: Page }) {
  switch (page) {
    case "home":
      return <Home />;
    case "proxies":
      return <Proxies />;
    case "profiles":
      return <Profiles />;
    case "connections":
      return <Connections />;
    case "stats":
      return <Stats />;
    case "rules":
      return <Rules />;
    case "logs":
      return <Logs />;
    case "tailscale":
      return <Tailscale />;
    case "connectivity":
      return <Connectivity />;
    case "settings":
      return <Settings />;
  }
}

export default function App() {
  const page = useApp((s) => s.page);
  const booted = useApp((s) => s.booted);
  const collapsed = useApp((s) => s.settings?.ui.collapseNav ?? false);
  const navigate = useApp((s) => s.navigate);
  const t = useT();
  const outdated = useApp((s) => !!s.state?.service.outdated);
  useEffect(() => {
    if (booted && outdated) offerServiceUpdate();
  }, [booted, outdated]);
  const onAction = (action: string) => {
    switch (action) {
      case "install-service":
      case "repair-service":
        void run(() => System.installService(false), t("service.installFailed"), t("service.installed"));
        break;
      case "install-tun":
        void enableTunWithService();
        break;
      case "relaunch":
        void AppAPI.relaunch();
        break;
      case "open-sync":
        navigate("settings", "sync");
        break;
      default:
        navigate("settings");
    }
  };
  return (
    <div className={`shell${collapsed ? " collapsed" : ""}`}>
      <Sidebar />
      <main className="main">
        {booted ? (
          <Suspense
            fallback={
              <div className="empty" style={{ flex: 1 }}>
                <Spinner />
              </div>
            }
          >
            <PageView page={page} />
          </Suspense>
        ) : (
          <div className="empty boot" style={{ flex: 1 }}>
            <BrandMark size={48} />
            <Spinner size={16} />
          </div>
        )}
      </main>
      <Toasts onAction={onAction} />
      <AskHost />
    </div>
  );
}
