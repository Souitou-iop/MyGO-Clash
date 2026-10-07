import { ChevronRight, EyeOff, LayoutGrid, List, Pin, PinOff, RefreshCw, Server, Stethoscope, Zap } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { PageHeader } from "../components/Page";
import { bytes, dateOnly, groupType, percent, relative } from "../lib/format";
import { useAsync } from "../lib/hooks";
import { useT } from "../lib/i18n";
import { patchSettings, run, useApp } from "../lib/store";
import { Core, Proxies as API, type ProxyGroup, type ProxyItem } from "../mygo";
import { Badge, Banner, Button, Collapse, Delay, Empty, Progress, reflow, SearchInput, Segmented, Select, Spinner } from "../ui";
import { CoreDown } from "../components/CoreDown";

type Sort = "default" | "delay" | "name";

function loadOpen(): Record<string, boolean> {
  try {
    return JSON.parse(localStorage.getItem("proxies.open") ?? "{}");
  } catch {
    return {};
  }
}

function sortItems(items: ProxyItem[], sort: Sort, delays: Record<string, number>): ProxyItem[] {
  if (sort === "default") return items;
  const out = [...items];
  if (sort === "name") return out.sort((a, b) => a.name.localeCompare(b.name));
  const d = (p: ProxyItem) => {
    const v = delays[p.name] ?? p.delay;
    return v <= 0 ? Number.MAX_SAFE_INTEGER - (v === 0 ? 0 : 1) : v;
  };
  return out.sort((a, b) => d(a) - d(b));
}

function Member({
  item,
  active,
  selectable,
  delay,
  testing,
  onSelect,
  onTest,
  layout,
}: {
  item: ProxyItem;
  active: boolean;
  selectable: boolean;
  delay: number;
  testing: boolean;
  onSelect: () => void;
  onTest: () => void;
  layout: string;
}) {
  // Only a node chosen while the page is open pops, not those chosen before.
  const wasActive = useRef(active);
  const [picked, setPicked] = useState(false);
  useEffect(() => {
    if (active && !wasActive.current) setPicked(true);
    wasActive.current = active;
  }, [active]);
  return (
    <div
      className={`proxy-item ${layout}${active ? " active" : ""}${active && picked ? " picked" : ""}${selectable ? " selectable" : ""}`}
      onClick={() => selectable && !active && onSelect()}
      role={selectable ? "button" : undefined}
      title={item.name}
    >
      <div className="grow" style={{ minWidth: 0 }}>
        <div className="proxy-name ellipsis">{item.name}</div>
        <div className="proxy-meta">
          <span>{item.type}</span>
          {item.udp && <span className="tag">UDP</span>}
          {item.xudp && <span className="tag">XUDP</span>}
          {item.tfo && <span className="tag">TFO</span>}
          {item.provider && <span className="tag">{item.provider}</span>}
          {item.isGroup && item.now && <span className="ellipsis">→ {item.now}</span>}
        </div>
      </div>
      <button
        className="delay-btn"
        onClick={(e) => {
          e.stopPropagation();
          onTest();
        }}
      >
        <Delay value={delay} loading={testing} />
      </button>
    </div>
  );
}

function Group({
  group,
  open,
  onToggle,
  search,
  sort,
  layout,
  columns,
  reload,
  showIcon,
  flash,
  hideDead,
}: {
  group: ProxyGroup;
  open: boolean;
  onToggle: () => void;
  search: string;
  sort: Sort;
  layout: string;
  columns: number;
  reload: () => void;
  showIcon: boolean;
  flash?: boolean;
  hideDead: boolean;
}) {
  const t = useT();
  const [delays, setDelays] = useState<Record<string, number>>({});
  const [testing, setTesting] = useState<Record<string, boolean>>({});
  const [groupTesting, setGroupTesting] = useState(false);
  // The selection shows at once; the core's answer replaces it.
  const [pending, setPending] = useState<string | null>(null);
  useEffect(() => setPending(null), [group.now]);
  const now = pending ?? group.now;
  const selectable = group.type === "Selector" || group.type === "URLTest" || group.type === "Fallback";
  const items = useMemo(() => {
    const q = search.trim().toLowerCase();
    // Hiding the unavailable keeps the selected one, so the group still shows where it goes.
    const dead = (m: ProxyItem) => m.name !== group.now && (delays[m.name] === 0 || (delays[m.name] === undefined && !m.alive));
    const list = group.all.filter((m) => (!q || m.name.toLowerCase().includes(q) || m.type.toLowerCase().includes(q)) && !(hideDead && dead(m)));
    return sortItems(list, sort, delays);
  }, [group.all, group.now, search, sort, delays, hideDead]);
  if (search && items.length === 0) return null;
  const testOne = async (name: string) => {
    setTesting((s) => ({ ...s, [name]: true }));
    // A node that does not answer gives 0, a timeout; an error is the app's.
    const d = await run(() => API.delay(name, group.testUrl ?? ""), t("proxies.testFailed"));
    if (d !== undefined) setDelays((s) => ({ ...s, [name]: d }));
    setTesting((s) => ({ ...s, [name]: false }));
  };
  const testAll = async () => {
    setGroupTesting(true);
    const res = await run(() => API.groupDelay(group.name, group.testUrl ?? ""), t("proxies.testFailed"));
    if (res) {
      const all: Record<string, number> = {};
      for (const m of group.all) all[m.name] = res[m.name] ?? 0;
      setDelays(all);
    }
    setGroupTesting(false);
    reload();
  };
  const select = async (name: string) => {
    setPending(name);
    const ok = await run(() => API.select(group.name, name).then(() => true), t("proxies.selectFailed"));
    if (!ok) setPending(null);
    reload();
  };
  const current = group.all.find((m) => m.name === group.now);
  return (
    <section className={`card proxy-group${open ? " open" : ""}${flash ? " flash" : ""}`} data-group={group.name}>
      <div className="proxy-group-head" onClick={onToggle}>
        <ChevronRight size={16} className="muted group-chev" />
        {showIcon && group.icon && <img src={group.icon} alt="" className="group-icon" />}
        <div className="grow" style={{ minWidth: 0 }}>
          <div className="row" style={{ gap: 7 }}>
            <span style={{ fontWeight: 650 }} className="ellipsis">
              {group.name}
            </span>
            <Badge tip={group.type}>{groupType(t, group.type)}</Badge>
            {group.fixed && (
              <Badge tone="info" tip={t("proxies.fixedTip")}>
                <Pin size={10} /> {t("proxies.fixed")}
              </Badge>
            )}
            <span className="faint" style={{ fontSize: 11.5 }}>
              {group.all.length}
            </span>
          </div>
          <div className="muted ellipsis" style={{ fontSize: 12 }}>
            {group.now ? `${group.now}` : "—"}
          </div>
        </div>
        <div className="row" onClick={(e) => e.stopPropagation()}>
          <Delay value={delays[group.now] ?? current?.delay} />
          {group.fixed && <Button size="sm" variant="ghost" icon={<PinOff size={14} />} tip={t("proxies.unfix")} onClick={() => run(() => API.unfix(group.name), t("common.failed")).then(reload)} />}
          <Button size="sm" variant="ghost" icon={<Zap size={14} />} loading={groupTesting} onClick={testAll} tip={t("proxies.testGroup")} />
        </div>
      </div>
      <Collapse open={open}>
        <div className={`proxy-grid ${layout}`} ref={reflow} style={layout === "card" && columns > 0 ? { gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))` } : undefined}>
          {items.map((m) => (
            <Member
              key={m.name}
              item={m}
              layout={layout}
              active={m.name === now}
              selectable={selectable}
              delay={delays[m.name] ?? m.delay}
              testing={!!testing[m.name]}
              onSelect={() => select(m.name)}
              onTest={() => testOne(m.name)}
            />
          ))}
        </div>
      </Collapse>
    </section>
  );
}

function ProvidersPanel({ providers, reload }: { providers: import("../mygo").ProviderView[]; reload: () => void }) {
  const t = useT();
  const lang = useApp((s) => s.lang);
  const [busy, setBusy] = useState<Record<string, string>>({});
  if (providers.length === 0) return null;
  const act = async (name: string, kind: string, fn: () => Promise<void>, fail: string) => {
    setBusy((b) => ({ ...b, [name]: kind }));
    await run(fn, fail);
    setBusy((b) => ({ ...b, [name]: "" }));
    reload();
  };
  return (
    <section className="section" style={{ marginTop: 20 }}>
      <div className="section-title">
        <Server size={13} /> {t("proxies.providers")}
        <div className="spacer" />
        <Button
          size="sm"
          variant="ghost"
          icon={<RefreshCw size={13} />}
          onClick={async () => {
            for (const p of providers) await act(p.name, "update", () => API.updateProvider(p.name), t("proxies.providerUpdateFailed"));
          }}
        >
          {t("proxies.updateAll")}
        </Button>
      </div>
      <div className="grid-cards" ref={reflow}>
        {providers.map((p) => {
          const info = p.subscriptionInfo;
          const used = (info?.Upload ?? 0) + (info?.Download ?? 0);
          return (
            <div key={p.name} className="card card-pad col" style={{ gap: 8 }}>
              <div className="row">
                <div className="grow">
                  <div style={{ fontWeight: 620 }} className="ellipsis">
                    {p.name}
                  </div>
                  <div className="muted" style={{ fontSize: 12 }}>
                    {p.vehicleType} · {t("profiles.nodes", { n: p.count })} · {relative(p.updatedAt, lang)}
                  </div>
                </div>
                <Button size="sm" variant="ghost" icon={<Stethoscope size={14} />} loading={busy[p.name] === "check"} tip={t("proxies.healthcheck")} onClick={() => act(p.name, "check", () => API.healthcheckProvider(p.name), t("common.failed"))} />
                <Button size="sm" variant="ghost" icon={<RefreshCw size={14} />} loading={busy[p.name] === "update"} tip={t("proxies.updateProvider")} onClick={() => act(p.name, "update", () => API.updateProvider(p.name), t("proxies.providerUpdateFailed"))} />
              </div>
              {info && info.Total > 0 && (
                <>
                  <Progress value={percent(used, info.Total)} />
                  <div className="row muted" style={{ fontSize: 11.5 }}>
                    <span className="tnum">
                      {bytes(used)} / {bytes(info.Total)}
                    </span>
                    <span className="spacer" />
                    {info.Expire > 0 && t("profiles.expires", { date: dateOnly(info.Expire) })}
                  </div>
                </>
              )}
            </div>
          );
        })}
      </div>
    </section>
  );
}

export default function Proxies() {
  const t = useT();
  const settings = useApp((s) => s.settings);
  const runtime = useApp((s) => s.runtime);
  const selection = useApp((s) => s.selection);
  const running = useApp((s) => s.state?.core.status === "running");
  const { data, loading, error, reload } = useAsync(() => API.view(), [runtime, selection, running]);
  const [search, setSearch] = useState("");
  const [sort, setSort] = useState<Sort>(() => (localStorage.getItem("proxies.sort") as Sort) || "default");
  const [open, setOpen] = useState<Record<string, boolean>>(loadOpen);
  const [hideDead, setHideDead] = useState(() => localStorage.getItem("proxies.hideDead") === "1");
  const layout = settings?.ui.proxyLayout ?? "card";
  const mode = settings?.clash.mode ?? "rule";
  const toggle = (name: string) => {
    const next = { ...open, [name]: !isOpen(name) };
    setOpen(next);
    localStorage.setItem("proxies.open", JSON.stringify(next));
  };
  const groups = useMemo(() => {
    if (!data) return [];
    if (mode === "global" && data.global) return [data.global];
    if (mode === "direct") return [];
    return data.groups.filter((g) => !g.hidden);
  }, [data, mode]);
  const isOpen = (name: string) => open[name] ?? (groups.length <= 3 || !!search);
  // A group another page pointed to, such as the target of a rule: open it,
  // bring it into view, and light it up for a moment.
  const focus = useApp((s) => s.focusGroup);
  const [flash, setFlash] = useState<string | null>(null);
  useEffect(() => {
    if (!focus || !groups.some((g) => g.name === focus)) return;
    useApp.setState({ focusGroup: null });
    setSearch("");
    if (!isOpen(focus)) toggle(focus);
    setFlash(focus);
    requestAnimationFrame(() => document.querySelector(`[data-group="${CSS.escape(focus)}"]`)?.scrollIntoView({ block: "start", behavior: "smooth" }));
    setTimeout(() => setFlash(null), 1400);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [focus, groups]);
  return (
    <>
      <PageHeader title={t("nav.proxies")}>
        <Segmented
          value={mode}
          onChange={(m) => run(() => Core.setMode(m), t("common.failed"))}
          options={[
            { value: "rule", label: t("mode.rule") },
            { value: "global", label: t("mode.global") },
            { value: "direct", label: t("mode.direct") },
          ]}
        />
      </PageHeader>
      <div className="page-body">
        <div className="list-toolbar">
          <SearchInput value={search} onChange={setSearch} placeholder={t("proxies.search")} />
          <Select
            value={sort}
            onChange={(v) => {
              setSort(v);
              localStorage.setItem("proxies.sort", v);
            }}
            options={[
              { value: "default", label: t("proxies.sortDefault") },
              { value: "delay", label: t("proxies.sortDelay") },
              { value: "name", label: t("proxies.sortName") },
            ]}
            width={130}
          />
          <Button
            size="sm"
            variant={hideDead ? "primary" : "ghost"}
            icon={<EyeOff size={14} />}
            tip={t("proxies.hideDeadTip")}
            onClick={() => {
              setHideDead(!hideDead);
              localStorage.setItem("proxies.hideDead", hideDead ? "0" : "1");
            }}
          >
            {t("proxies.hideDead")}
          </Button>
          <div className="spacer" />
          <Segmented
            value={layout}
            onChange={(v) => run(() => patchSettings({ ui: { proxyLayout: v } }), t("common.failed"))}
            options={[
              { value: "card", label: "", icon: <LayoutGrid size={14} /> },
              { value: "list", label: "", icon: <List size={14} /> },
            ]}
          />
          <Button size="sm" variant="ghost" icon={<RefreshCw size={14} />} onClick={reload} tip={t("common.refresh")} />
        </div>
        {!running ? (
          <CoreDown />
        ) : loading && !data ? (
          <div className="empty">
            <Spinner />
          </div>
        ) : error ? (
          <Banner tone="danger">{String(error instanceof Error ? error.message : error)}</Banner>
        ) : mode === "direct" ? (
          <Banner tone="info">{t("proxies.directMode")}</Banner>
        ) : groups.length === 0 ? (
          <Empty title={t("proxies.noGroups")}>{t("proxies.noGroupsHint")}</Empty>
        ) : (
          <div className="col" style={{ gap: 10 }}>
            {groups.map((g) => (
              <Group
                key={g.name}
                group={g}
                open={isOpen(g.name)}
                onToggle={() => toggle(g.name)}
                search={search}
                sort={sort}
                layout={layout}
                columns={settings?.ui.proxyColumns ?? 0}
                reload={reload}
                showIcon={settings?.ui.groupIcons ?? true}
                flash={flash === g.name}
                hideDead={hideDead}
              />
            ))}
          </div>
        )}
        {data && mode !== "direct" && <ProvidersPanel providers={data.providers} reload={reload} />}
      </div>
    </>
  );
}
