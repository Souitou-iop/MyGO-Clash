import {
  Activity,
  ArrowDown,
  ArrowUp,
  BarChart3,
  BookOpen,
  Cable,
  Check,
  ChevronDown,
  EyeOff,
  Globe,
  LayoutGrid,
  List,
  ListTree,
  Network,
  PanelTop,
  Pause,
  Power,
  RefreshCw,
  ScrollText,
  Search,
  Settings,
  Settings2,
  Shield,
  Trash2,
  Waypoints,
  Wifi,
  Zap,
} from "lucide-react";
import { type ReactNode, useState } from "react";
import { Roll, Spinner, Stage, Window, type Act } from "../kit";
import compass from "./compass.svg";
import flagHK from "./flags/hk.svg";
import flagJP from "./flags/jp.svg";
import flagSG from "./flags/sg.svg";
import flagUS from "./flags/us.svg";

type ThemeName = "dark" | "light";
type Lang = "zh" | "en";
type NodeId = "auto" | "direct" | "hk" | "jp" | "us" | "sg";

type Strings = {
  file: string;
  edit: string;
  view: string;
  home: string;
  proxies: string;
  profiles: string;
  connections: string;
  traffic: string;
  rules: string;
  logs: string;
  reachability: string;
  settings: string;
  pageConnections: string;
  filterConnections: string;
  byDownload: string;
  connectionCount: string;
  host: string;
  rule: string;
  chain: string;
  running: string;
  systemProxy: string;
  on: string;
  off: string;
  tun: string;
  tunSub: string;
  modeRule: string;
  modeGlobal: string;
  modeDirect: string;
  tailscaleSignIn: string;
};

const STRINGS: Record<Lang, Strings> = {
  zh: {
    file: "文件",
    edit: "编辑",
    view: "显示",
    home: "首页",
    proxies: "代理",
    profiles: "订阅",
    connections: "连接",
    traffic: "流量统计",
    rules: "规则",
    logs: "日志",
    reachability: "连通性",
    settings: "设置",
    pageConnections: "连接",
    filterConnections: "筛选连接",
    byDownload: "按下载速度",
    connectionCount: "共 5 个连接",
    host: "主机",
    rule: "规则",
    chain: "链路",
    running: "运行中",
    systemProxy: "系统代理",
    on: "开",
    off: "关",
    tun: "虚拟网卡模式",
    tunSub: "虚拟网卡模式（需安装服务）",
    modeRule: "规则",
    modeGlobal: "全局",
    modeDirect: "直连",
    tailscaleSignIn: "Tailscale: 需要登录",
  },
  en: {
    file: "File",
    edit: "Edit",
    view: "View",
    home: "Home",
    proxies: "Proxies",
    profiles: "Profiles",
    connections: "Connections",
    traffic: "Traffic",
    rules: "Rules",
    logs: "Logs",
    reachability: "Reachability",
    settings: "Settings",
    pageConnections: "Connections",
    filterConnections: "Filter connections",
    byDownload: "By download",
    connectionCount: "5 connections",
    host: "Host",
    rule: "Rule",
    chain: "Chain",
    running: "Running",
    systemProxy: "System Proxy",
    on: "On",
    off: "Off",
    tun: "TUN",
    tunSub: "TUN (service required)",
    modeRule: "Rule",
    modeGlobal: "Global",
    modeDirect: "Direct",
    tailscaleSignIn: "Tailscale: sign in",
  },
};

type State = {
  panelOpen: boolean;
  systemProxy: boolean;
  selected: NodeId;
  testing: boolean;
  tested: boolean;
};

type Node = {
  id: NodeId;
  name: string;
  type: string;
  flag?: string;
  delay?: number;
};

const NODES: Node[] = [
  { id: "auto", name: "Auto", type: "URLTest → HK 01" },
  { id: "direct", name: "DIRECT", type: "Direct · UDP" },
  { id: "hk", name: "HK 01", type: "Shadowsocks", flag: flagHK, delay: 42 },
  { id: "jp", name: "JP 01", type: "Shadowsocks", flag: flagJP, delay: 58 },
  { id: "us", name: "US 01", type: "Trojan", flag: flagUS, delay: 76 },
  { id: "sg", name: "SG 01", type: "Vmess", flag: flagSG, delay: 63 },
];

const INITIAL: State = {
  panelOpen: false,
  systemProxy: false,
  selected: "direct",
  testing: false,
  tested: false,
};

const POSTER: State = {
  panelOpen: true,
  systemProxy: true,
  selected: "hk",
  testing: false,
  tested: true,
};

const REST = { x: 520, y: 15 };

const DARK_TOKENS = {
  "--ss-canvas": "#161618",
  "--ss-surface": "#1a1a1d",
  "--ss-chrome": "#1a1a1d",
  "--ss-line": "#313137",
  "--ss-line-faint": "#29292e",
  "--ss-text": "#8e8e97",
  "--ss-text-strong": "#ececef",
  "--ss-text-faint": "#5f5f68",
  "--ss-icon": "#8e8e97",
  "--ss-icon-faint": "#5f5f68",
  "--ss-accent": "#46acdb",
  "--ss-accent-soft": "rgba(70, 172, 219, 0.18)",
  "--ss-success": "#5cc985",
  "--ss-danger": "#ff6b61",
  "--ss-warning": "#f0c766",
  "--ss-raised": "#26262b",
  "--ss-well": "#0e0e10",
  "--ss-font":
    '-apple-system, BlinkMacSystemFont, "Segoe UI Variable Text", "Segoe UI", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei UI", "Noto Sans CJK SC", "Noto Sans", sans-serif',
} as const;

const LIGHT_TOKENS = {
  "--ss-canvas": "#ededef",
  "--ss-surface": "#ffffff",
  "--ss-chrome": "#ffffff",
  "--ss-line": "#e2e2e6",
  "--ss-line-faint": "#ececef",
  "--ss-text": "#6c6c75",
  "--ss-text-strong": "#18181b",
  "--ss-text-faint": "#a0a0a8",
  "--ss-icon": "#6c6c75",
  "--ss-icon-faint": "#a0a0a8",
  "--ss-accent": "#0b88bb",
  "--ss-accent-soft": "rgba(11, 136, 187, 0.12)",
  "--ss-success": "#1f9254",
  "--ss-danger": "#d1333f",
  "--ss-warning": "#b26b00",
  "--ss-raised": "#f6f6f7",
  "--ss-well": "#f4f4f5",
  "--ss-font":
    '-apple-system, BlinkMacSystemFont, "Segoe UI Variable Text", "Segoe UI", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei UI", "Noto Sans CJK SC", "Noto Sans", sans-serif',
} as const;

function delayText(node: Node, tested: boolean) {
  if (!node.delay) return "—";
  return tested ? `${node.delay} ms` : "超时";
}

function delayTone(node: Node, tested: boolean) {
  if (!node.delay) return "muted";
  return tested ? "good" : "bad";
}

async function script(a: Act<State>) {
  await a.wait(650);

  await a.move("tray");
  await a.wait(280);
  await a.click("tray");
  await a.set({ panelOpen: true });
  await a.wait(720);

  await a.move("panel-sys");
  await a.wait(260);
  await a.click("panel-sys");
  await a.set({ systemProxy: true });
  await a.wait(620);

  await a.move("panel-hk");
  await a.wait(220);
  await a.click("panel-hk");
  await a.set({ selected: "hk" });
  await a.wait(520);

  await a.move("panel-test");
  await a.wait(220);
  await a.click("panel-test");
  await a.set({ testing: true });
  await a.wait(750);
  await a.set({ testing: false, tested: true });

  await a.move(REST, { duration: 520 });
  await a.wait(450);
  a.mark("payoff");
  await a.wait(2300);

  await a.click({ x: 420, y: 300 });
  await a.set({ panelOpen: false });
  await a.wait(520);
  await a.snap({ systemProxy: false, selected: "direct", testing: false, tested: false });
  await a.move(REST, { duration: 420 });
  await a.wait(300);
}

function App() {
  const params = new URLSearchParams(window.location.search);
  const theme: ThemeName = params.get("theme") === "light" ? "light" : "dark";
  const lang: Lang = params.get("lang") === "en" ? "en" : "zh";
  const embed = params.get("embed") === "1";
  const [run, setRun] = useState(0);
  const tokens = theme === "dark" ? DARK_TOKENS : LIGHT_TOKENS;
  const t = STRINGS[lang];

  return (
    <main className="mcs-host" data-theme={theme} data-embed={embed || undefined}>
      <Showcase key={`${run}-${lang}`} theme={theme} tokens={tokens} t={t} />
      <div className="mcs-host-actions">
        <button type="button" onClick={() => setRun((n) => n + 1)}>
          Replay
        </button>
        <a href={theme === "dark" ? "?theme=light" : "?theme=dark"}>{theme === "dark" ? "Light" : "Dark"}</a>
        <a href={`?theme=${theme}&lang=${lang === "zh" ? "en" : "zh"}`}>{lang === "zh" ? "EN" : "中文"}</a>
      </div>
    </main>
  );
}

function Showcase({
  theme,
  tokens,
  t,
}: {
  theme: ThemeName;
  tokens: typeof DARK_TOKENS | typeof LIGHT_TOKENS;
  t: Strings;
}) {
  return (
    <Stage
      width={1080}
      height={576}
      initial={INITIAL}
      poster={POSTER}
      script={script}
      play="once"
      theme={theme}
      fidelity="realistic"
      tokens={tokens}
      cursor={{ at: REST, ripple: true, idle: false }}
      className="mcs-stage"
    >
      {(state) => (
        <div className="mcs-scene" data-theme={theme} data-panel={state.panelOpen}>
          <MenuBar t={t} />
          <MainWindow t={t} />
          <QuickPanel state={state} t={t} />
        </div>
      )}
    </Stage>
  );
}

function MenuBar({ t }: { t: Strings }) {
  return (
    <div className="mcs-menubar">
      <div className="mcs-menu-left">
        <img className="mcs-menubar-mark" src={compass} alt="" />
        <span className="mcs-menu-app">MyGO-Clash</span>
        <span className="mcs-menu-item">{t.file}</span>
        <span className="mcs-menu-item">{t.edit}</span>
        <span className="mcs-menu-item">{t.view}</span>
      </div>
      <div className="mcs-menu-right">
        <Wifi size={13} strokeWidth={1.8} />
        <span className="mcs-menu-time">21:41</span>
      </div>
      <button className="mcs-tray" data-target="tray" type="button" aria-label="快捷面板">
        <img src={compass} alt="" />
        <span className="mcs-tray-dot" />
      </button>
    </div>
  );
}

function MainWindow({ t }: { t: Strings }) {
  return (
    <div className="mcs-app" data-crop>
      <Window
        className="mcs-window"
        sidebar={<Sidebar t={t} />}
        sidebarWidth={168}
        title={t.pageConnections}
        toolbar={
          <div className="mcs-title-tools">
            <button className="mcs-icon-button" type="button" aria-label="刷新">
              <RefreshCw size={14} strokeWidth={1.7} />
            </button>
            <button className="mcs-icon-button" type="button" aria-label="隐藏不可用">
              <EyeOff size={14} strokeWidth={1.7} />
            </button>
            <button className="mcs-view-button" type="button" aria-label="列表视图" data-on="true">
              <List size={14} strokeWidth={1.7} />
            </button>
          </div>
        }
      >
        <div className="mcs-page">
          <div className="mcs-page-tools">
            <div className="mcs-search">
              <Search size={13} strokeWidth={1.7} />
              <span>{t.filterConnections}</span>
            </div>
            <div className="mcs-select mcs-select--compact">
              <span>{t.byDownload}</span>
              <ChevronDown size={13} strokeWidth={1.7} />
            </div>
            <span className="mcs-tool-spacer" />
            <span className="mcs-tool-note">{t.connectionCount}</span>
            <button className="mcs-icon-button" type="button" aria-label="暂停">
              <Pause size={14} strokeWidth={1.7} />
            </button>
            <button className="mcs-icon-button" type="button" aria-label="清空">
              <Trash2 size={14} strokeWidth={1.7} />
            </button>
          </div>

          <div className="mcs-connections">
            <div className="mcs-connection-head">
              <span>{t.host}</span>
              <span>{t.rule}</span>
              <span>{t.chain}</span>
            </div>
            <ConnectionRow host="github.com" rule="DOMAIN-SUFFIX" chain="Proxy → HK 01" active />
            <ConnectionRow host="api.openai.com" rule="DOMAIN-SUFFIX" chain="Proxy → HK 01" active />
            <ConnectionRow host="www.youtube.com" rule="DOMAIN-SUFFIX" chain="Streaming → SG 01" />
            <ConnectionRow host="cdn.jsdelivr.net" rule="DOMAIN-SUFFIX" chain="DIRECT" direct />
            <ConnectionRow host="tailscale.com" rule="DOMAIN" chain="DIRECT" direct />
            <ConnectionRow host="registry.npmjs.org" rule="DOMAIN-SUFFIX" chain="Proxy → HK 01" />
            <ConnectionRow host="objects.github" rule="DOMAIN-SUFFIX" chain="Proxy → HK 01" />
            <ConnectionRow host="cdn.discordapp.com" rule="DOMAIN-SUFFIX" chain="Streaming → SG 01" />
            <ConnectionRow host="update.googleapis" rule="DOMAIN-SUFFIX" chain="DIRECT" direct />
            <ConnectionRow host="api.github.com" rule="DOMAIN-SUFFIX" chain="Proxy → HK 01" />
          </div>
        </div>
      </Window>
    </div>
  );
}

function Sidebar({ t }: { t: Strings }) {
  return (
    <aside className="mcs-sidebar">
        <div className="mcs-brand">
          <img src={compass} alt="" />
          <span>MyGO-Clash</span>
        </div>
        <nav className="mcs-nav">
          <NavItem icon={<LayoutGrid size={15} />} label={t.home} />
          <NavItem icon={<Waypoints size={15} />} label={t.proxies} />
          <NavItem icon={<BookOpen size={15} />} label={t.profiles} />
          <NavItem icon={<Cable size={15} />} label={t.connections} active />
          <NavItem icon={<BarChart3 size={15} />} label={t.traffic} />
          <NavItem icon={<ListTree size={15} />} label={t.rules} />
          <NavItem icon={<ScrollText size={15} />} label={t.logs} />
          <NavItem icon={<Network size={15} />} label="Tailscale" />
          <NavItem icon={<Activity size={15} />} label={t.reachability} />
          <NavItem icon={<Settings size={15} />} label={t.settings} />
        </nav>
        <div className="mcs-sidebar-foot">
          <div className="mcs-mini-chart" aria-hidden>
            {[5, 8, 6, 12, 10, 16, 13, 20, 17, 24, 19, 14, 22, 18, 26, 21, 15, 23].map((h, i) => (
              <span key={i} style={{ height: `${h}px` }} />
            ))}
          </div>
          <div className="mcs-rate-line">
            <ArrowUp size={11} />
            <span>0 B/s</span>
          </div>
          <div className="mcs-rate-line mcs-rate-line--down">
            <ArrowDown size={11} />
            <span>0 B/s</span>
          </div>
        </div>
    </aside>
  );
}

function NavItem({ icon, label, active }: { icon: ReactNode; label: string; active?: boolean }) {
  return (
    <div className="mcs-nav-item" data-active={active || undefined}>
      {icon}
      <span>{label}</span>
    </div>
  );
}

function ConnectionRow({
  host,
  rule,
  chain,
  active,
  direct,
}: {
  host: string;
  rule: string;
  chain: string;
  active?: boolean;
  direct?: boolean;
}) {
  return (
    <div className="mcs-connection-row" data-active={active || undefined}>
      <span className="mcs-connection-host">
        <span className="mcs-connection-host-dot" data-direct={direct || undefined} />
        <span className="mcs-connection-hostname">{host}</span>
      </span>
      <span className="mcs-rule">{rule}</span>
      <span className="mcs-chain" data-direct={direct || undefined}>
        {chain}
      </span>
    </div>
  );
}

function QuickPanel({ state, t }: { state: State; t: Strings }) {
  return (
    <section
      className="mcs-panel ss-popover"
      data-open={state.panelOpen}
      data-show={state.panelOpen ? "" : undefined}
      aria-hidden={!state.panelOpen}
    >
      <div className="mcs-panel-head">
        <span className="mcs-status-dot" />
        <span className="mcs-panel-title">MyGO-Clash</span>
        <span className="mcs-panel-state">{t.running}</span>
        <span className="mcs-panel-spacer" />
        <button className="mcs-panel-icon" type="button" aria-label="设置">
          <Settings2 size={16} strokeWidth={1.7} />
        </button>
        <button className="mcs-panel-icon" type="button" aria-label="主窗口">
          <PanelTop size={16} strokeWidth={1.7} />
        </button>
      </div>

      <div className="mcs-panel-speed">
        <ArrowUp size={14} strokeWidth={1.8} className="mcs-up" />
        <span>0 B/s</span>
        <ArrowDown size={14} strokeWidth={1.8} className="mcs-down" />
        <span>0 B/s</span>
        <span className="mcs-panel-spacer" />
        <div className="mcs-speed-bars" aria-hidden>
          {[2, 2, 3, 2, 4, 2, 3, 2, 2, 4, 3, 2, 2, 3, 2, 4, 2, 3, 2, 2, 3, 4, 2, 2, 3, 2, 4, 2, 3, 2].map((h, i) => (
            <span key={i} style={{ height: `${h}px`, opacity: 0.3 + (i / 30) * 0.7 }} />
          ))}
        </div>
      </div>

      <div className="mcs-mode">
        <span data-on="true">{t.modeRule}</span>
        <span>{t.modeGlobal}</span>
        <span>{t.modeDirect}</span>
      </div>

      <div className="mcs-tiles">
        <div className="mcs-tile" data-on={state.systemProxy || undefined} data-target="panel-sys">
          <Globe size={16} strokeWidth={1.6} />
          <div>
            <strong>{t.systemProxy}</strong>
            <small>
              <Roll variant="fade">{state.systemProxy ? t.on : t.off}</Roll>
            </small>
          </div>
        </div>
        <div className="mcs-tile" data-target="panel-tun">
          <Shield size={16} strokeWidth={1.6} />
          <div>
            <strong>{t.tun}</strong>
            <small>{t.tunSub}</small>
          </div>
        </div>
      </div>

      <div className="mcs-group-row">
        <div className="mcs-select">
          <span>Proxy</span>
          <ChevronDown size={14} strokeWidth={1.7} />
        </div>
        <button className="mcs-test-button" type="button" data-target="panel-test" aria-label="测速" data-testing={state.testing}>
          <Zap className="mcs-test-icon" size={16} strokeWidth={1.7} />
          <Spinner className="mcs-test-spinner" size={14} />
        </button>
      </div>

      <div className="mcs-panel-list">
        {NODES.map((node) => (
          <PanelNode key={node.id} node={node} state={state} />
        ))}
      </div>

      <div className="mcs-panel-foot">
        <div className="mcs-tailscale-line">{t.tailscaleSignIn}</div>
        <div className="mcs-profile-row">
          <div className="mcs-select">
            <span>Test Airport</span>
            <ChevronDown size={14} strokeWidth={1.7} />
          </div>
          <button className="mcs-power" type="button" aria-label="退出">
            <Power size={15} strokeWidth={1.7} />
          </button>
        </div>
      </div>
    </section>
  );
}

function PanelNode({ node, state }: { node: Node; state: State }) {
  const selected = state.selected === node.id;
  return (
    <div className="mcs-panel-row" data-selected={selected || undefined} data-target={node.id === "hk" ? "panel-hk" : undefined}>
      <span className="mcs-check-slot">{selected && <Check size={14} strokeWidth={2} />}</span>
      {node.flag ? <img className="mcs-flag" src={node.flag} alt="" /> : <span className="mcs-flag-gap" />}
      <span className="mcs-panel-node">{node.name}</span>
      <Roll className={`mcs-delay mcs-delay--${delayTone(node, state.tested)}`}>{delayText(node, state.tested)}</Roll>
    </div>
  );
}

export { App };
