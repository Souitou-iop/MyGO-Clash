import { ArrowDownRight, ArrowUpRight, Download, FileUp, ListFilter, MoreHorizontal, Settings2, Trash2, X } from "lucide-react";
import { type ReactNode, useEffect, useMemo, useState } from "react";
import { Flag } from "../components/Flag";
import { PageHeader } from "../components/Page";
import { bytes, ruleType } from "../lib/format";
import { useAsync, useInterval } from "../lib/hooks";
import { useT } from "../lib/i18n";
import { patchSettings, run, toast, useApp } from "../lib/store";
import { Stats as API, type StatsItem, type StatsPoint, type StatsReport } from "../mygo";
import { Button, Card, confirm, Dialog, Empty, Field, Input, Menu, SearchInput, Segmented, Select, Switch } from "../ui";

type Range = "today" | "7d" | "30d" | "all" | "custom";
type Dim = "app" | "site" | "node" | "rule" | "policy" | "country";
type SortBy = "traffic" | "conns";

const OTHER = "\u0000other";

function today(): string {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

function shift(date: string, days: number): string {
  const d = new Date(`${date}T00:00:00`);
  d.setDate(d.getDate() + days);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

/** Bars is the traffic over time: the hours of a day, or the days of a range. */
function Bars({ points, hourly, byConns }: { points: StatsPoint[]; hourly: boolean; byConns: boolean }) {
  const t = useT();
  const value = (p: StatsPoint) => (byConns ? (p.conns ?? 0) : p.up + p.down);
  const max = Math.max(1, ...points.map(value));
  const every = hourly ? 3 : points.length > 62 ? 14 : points.length > 14 ? 5 : 1;
  return (
    <div className="stats-bars" style={{ gridTemplateColumns: `repeat(${points.length}, minmax(0, 1fr))` }}>
      {points.map((p, i) => {
        const label = hourly ? `${p.label}:00` : p.label.slice(5);
        const tip = `${hourly ? label : p.label}  ↓ ${bytes(p.down)}  ↑ ${bytes(p.up)}  ${t("stats.connsN", { n: p.conns ?? 0 })}`;
        return (
          <div key={p.label} className="stats-col" title={tip}>
            <div className="stats-stack" style={{ height: `${(value(p) / max) * 100}%` }}>
              {byConns ? (
                <i className="stats-conns" style={{ flexGrow: 1 }} />
              ) : (
                <>
                  <i className="stats-up" style={{ flexGrow: p.up }} />
                  <i className="stats-down" style={{ flexGrow: p.down }} />
                </>
              )}
            </div>
            <span className="stats-tick faint">{i % every === 0 ? label : ""}</span>
          </div>
        );
      })}
    </div>
  );
}

/** Ranking lists what carried the traffic, each row filled to its share of the busiest; a row narrows the page to it. */
function Ranking({
  title,
  items,
  note,
  empty,
  byConns,
  selected,
  onPick,
  name,
}: {
  title: string;
  items: StatsItem[];
  note?: ReactNode;
  empty?: string;
  byConns: boolean;
  selected?: string;
  onPick: (it: StatsItem) => void;
  name: (it: StatsItem) => ReactNode;
}) {
  const t = useT();
  const weight = (i: StatsItem) => (byConns ? (i.conns ?? 0) : i.up + i.down);
  const max = Math.max(1, ...items.map(weight));
  return (
    <Card title={title} bodyClass="stats-rank">
      {items.length === 0 ? (
        <div className="faint stats-none">{empty ?? t("stats.noData")}</div>
      ) : (
        items.map((it) => {
          // A sliver of a fill would read as an edge stripe.
          const share = weight(it) / max;
          const key = it.other ? OTHER : it.name;
          const on = selected === key;
          const tip = `↓ ${bytes(it.down)}  ↑ ${bytes(it.up)}  ${t("stats.connsN", { n: it.conns ?? 0 })}`;
          return (
            <button key={key} className={`stats-row${on ? " is-on" : ""}`} title={tip} onClick={() => onPick(it)} disabled={it.other && !on}>
              {share >= 0.04 && <span className="stats-fill" style={{ width: `${share * 100}%` }} />}
              <span className={`stats-name${it.other || it.name === "—" ? " muted" : ""}`}>{name(it)}</span>
              <span className="stats-val tnum">{byConns ? (it.conns ?? 0).toLocaleString() : bytes(it.up + it.down)}</span>
              {on && <X size={12} className="stats-unpick" />}
            </button>
          );
        })
      )}
      {note}
    </Card>
  );
}

function Delta({ cur, prev }: { cur: number; prev?: number }) {
  const t = useT();
  // Against next to nothing, a percentage says nothing.
  if (prev === undefined || prev < 1 << 20) return null;
  const pct = Math.round(((cur - prev) / prev) * 100);
  if (Math.abs(pct) > 999) return null;
  const up = pct >= 0;
  return (
    <span className={`stats-delta ${up ? "is-up" : "is-down"}`} title={t("stats.prevTip", { prev: bytes(prev) })}>
      {up ? <ArrowUpRight size={12} /> : <ArrowDownRight size={12} />}
      {Math.abs(pct)}%
    </span>
  );
}

function SettingsDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const t = useT();
  const s = useApp((st) => st.settings!);
  const [text, setText] = useState("");
  useEffect(() => {
    if (open) setText(s.stats.exclude.join("\n"));
  }, [open, s.stats.exclude]);
  const save = (patch: Parameters<typeof patchSettings>[0]) => void run(() => patchSettings(patch), t("settings.saveFailed"));
  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={t("stats.settings")}
      icon={<Settings2 size={16} />}
      size="narrow"
      footer={
        <Button
          variant="primary"
          onClick={() => {
            save({ stats: { exclude: text.split("\n").map((l) => l.trim()).filter(Boolean) } });
            onClose();
          }}
        >
          {t("common.done")}
        </Button>
      }
    >
      <div className="form">
        <Field label={t("stats.keep")}>
          <Select
            value={s.stats.keepDays}
            onChange={(v) => save({ stats: { keepDays: v } })}
            options={[30, 90, 180, 365, 0].map((d) => ({ value: d, label: d === 0 ? t("stats.keepForever") : t("stats.keepDays", { n: d }) }))}
            width={160}
          />
        </Field>
        <div className="setting" style={{ padding: 0 }}>
          <div className="setting-text">
            <div className="setting-label">{t("stats.excludeLocal")}</div>
          </div>
          <Switch checked={s.stats.excludeLocal} onChange={(v) => save({ stats: { excludeLocal: v } })} />
        </div>
        <Field label={t("stats.exclude")} hint={t("stats.excludeHint")}>
          <textarea className="input mono" rows={5} value={text} onChange={(e) => setText(e.target.value)} placeholder={"example.com\n10.0.0.0/8\nprocess:Steam"} />
        </Field>
      </div>
    </Dialog>
  );
}

export default function Stats() {
  const t = useT();
  const lang = useApp((s) => s.lang);
  const navigate = useApp((s) => s.navigate);
  const [range, setRange] = useState<Range>("today");
  const [from, setFrom] = useState(() => shift(today(), -6));
  const [to, setTo] = useState(today);
  const [device, setDevice] = useState("");
  const [filter, setFilter] = useState("");
  const [applied, setApplied] = useState("");
  const [where, setWhere] = useState<Partial<Record<Dim, string>>>({});
  const [sortBy, setSortBy] = useState<SortBy>("traffic");
  const [settings, setSettings] = useState(false);
  const findProcess = useApp((s) => s.settings?.clash.findProcessMode);
  const { data: devices } = useAsync(() => API.devices(), []);
  const query = () =>
    API.query({ range: range === "custom" ? "custom" : range, from: range === "custom" ? from : undefined, to: range === "custom" ? to : undefined, top: 10, filter: applied || undefined, where, sortBy, device });
  const { data, reload } = useAsync(query, [range, from, to, applied, JSON.stringify(where), sortBy, device]);
  useInterval(() => void reload(), 10000);
  useEffect(() => {
    const id = setTimeout(() => setApplied(filter.trim()), 250);
    return () => clearTimeout(id);
  }, [filter]);
  const region = useMemo(() => {
    try {
      return new Intl.DisplayNames([lang], { type: "region" });
    } catch {
      return null;
    }
  }, [lang]);
  const countryName = (cc: string) => (cc === "LAN" ? t("stats.lan") : cc === "—" ? t("stats.unknownCountry") : (region?.of(cc) ?? cc));
  const pick = (dim: Dim) => (it: StatsItem) => {
    const key = it.other ? OTHER : it.name;
    setWhere((w) => {
      const next = { ...w };
      if (next[dim] === key) delete next[dim];
      else next[dim] = key;
      return next;
    });
  };
  const clear = async () => {
    if (!(await confirm({ title: t("stats.clearTitle"), message: t("stats.clearMsg"), confirm: t("stats.clear"), danger: true }))) return;
    await run(() => API.clear(), t("common.failed"));
    void reload();
  };
  const exportAs = async (format: "csv" | "json") => {
    const [f, tt] = range === "custom" ? [from, to] : data ? [data.from, data.to] : ["", ""];
    const path = await run(() => API.export(format, f, tt), t("common.failed"));
    if (path) toast({ level: "success", message: t("stats.exported"), detail: path });
  };
  const importFile = async () => {
    const n = await run(() => API.import(), t("common.failed"));
    if (n) {
      toast({ level: "success", message: t("stats.imported", { n }) });
      void reload();
    }
  };
  const label = (dim: Dim, name: string): string => {
    if (name === OTHER) return t("stats.other");
    if (dim === "country") return countryName(name);
    if (dim === "rule") {
      const [type, ...rest] = name.split(",");
      return rest.length ? `${ruleType(type!, rest.join(","))} ${rest.join(",")}` : ruleType(type!);
    }
    if (dim === "app" && name === "—") return t("stats.unknownApp");
    return name;
  };
  const itemName = (dim: Dim) => (it: StatsItem) => {
    if (it.other) return t("stats.other");
    if (dim === "country")
      return (
        <span className="stats-country">
          {/^[A-Z]{2}$/.test(it.name) && <Flag code={it.name} />}
          {countryName(it.name)}
        </span>
      );
    return label(dim, it.name);
  };
  const dims: { dim: Dim; title: string; items: (r: StatsReport) => StatsItem[] }[] = [
    { dim: "app", title: t("stats.apps"), items: (r) => r.apps },
    { dim: "site", title: t("stats.sites"), items: (r) => r.sites },
    { dim: "node", title: t("stats.nodes"), items: (r) => r.nodes },
    { dim: "policy", title: t("stats.policies"), items: (r) => r.policies },
    { dim: "rule", title: t("stats.rules"), items: (r) => r.rules },
    { dim: "country", title: t("stats.countries"), items: (r) => r.countries },
  ];
  const active = Object.entries(where) as [Dim, string][];
  const hourly = !!data && data.from === data.to;
  const narrowed = active.length > 0 || !!applied;
  const emptyMsg = narrowed ? t("stats.noMatches") : undefined;
  const findConnections = () => {
    const q = (where.site && where.site !== OTHER && where.site) || (where.app && where.app !== OTHER && where.app) || applied;
    useApp.setState({ connPreset: { view: "closed", search: q || "" } });
    navigate("connections");
  };
  return (
    <>
      <PageHeader title={t("nav.stats")}>
        {(devices?.length ?? 0) > 1 && (
          <Select
            value={device}
            onChange={setDevice}
            options={[
              { value: "", label: t("stats.thisDevice") },
              { value: "all", label: t("stats.allDevices") },
              ...devices!.filter((d) => !d.this).map((d) => ({ value: d.id, label: d.name })),
            ]}
            width={150}
          />
        )}
        <Segmented
          value={range}
          onChange={(v) => {
            if (v === "custom" && data) {
              setFrom(data.from);
              setTo(data.to);
            }
            setRange(v);
          }}
          options={[
            { value: "today", label: t("stats.today") },
            { value: "7d", label: t("stats.days7") },
            { value: "30d", label: t("stats.days30") },
            { value: "all", label: t("stats.all") },
            { value: "custom", label: t("stats.custom") },
          ]}
        />
        <Menu
          trigger={(open) => <Button size="sm" variant="ghost" icon={<MoreHorizontal size={15} />} onClick={open} tip={t("stats.more")} />}
          items={[
            { label: t("stats.exportCsv"), icon: <Download size={14} />, onClick: () => void exportAs("csv") },
            { label: t("stats.exportJson"), icon: <Download size={14} />, onClick: () => void exportAs("json") },
            { label: t("stats.import"), icon: <FileUp size={14} />, onClick: () => void importFile() },
            { separator: true },
            { label: t("stats.settings"), icon: <Settings2 size={14} />, onClick: () => setSettings(true) },
            { label: t("stats.clear"), icon: <Trash2 size={14} />, danger: true, onClick: () => void clear() },
          ]}
        />
      </PageHeader>
      <div className="page-body">
        {range === "custom" && (
          <div className="stats-dates">
            <Input type="date" value={from} min={data?.oldest || undefined} max={to} onChange={(e) => e.target.value && setFrom(e.target.value)} />
            <span className="faint">–</span>
            <Input type="date" value={to} min={from} max={today()} onChange={(e) => e.target.value && setTo(e.target.value)} />
            <div className="row" style={{ gap: 4 }}>
              {[7, 14, 30, 90].map((n) => (
                <button key={n} className="chip" onClick={() => (setFrom(shift(today(), -(n - 1))), setTo(today()))}>
                  {t("stats.lastDays", { n })}
                </button>
              ))}
            </div>
          </div>
        )}
        {!data || (data.up + data.down === 0 && !narrowed) ? (
          <Empty title={t("stats.empty")} art>
            <p className="muted">{t("stats.emptyHint")}</p>
          </Empty>
        ) : (
          <div className="stats-page">
            <div className="stats-summary">
              <Card className="stats-tile">
                <div className="stats-label muted">
                  {t("stats.total")}
                  <Delta cur={data.up + data.down} prev={data.prev ? data.prev.up + data.prev.down : undefined} />
                </div>
                <div className="stats-big tnum">{bytes(data.up + data.down)}</div>
              </Card>
              <Card className="stats-tile">
                <div className="stats-label muted">
                  <i className="stats-dot stats-down" />
                  {t("common.download")}
                </div>
                <div className="stats-big tnum">{bytes(data.down)}</div>
              </Card>
              <Card className="stats-tile">
                <div className="stats-label muted">
                  <i className="stats-dot stats-up" />
                  {t("common.upload")}
                </div>
                <div className="stats-big tnum">{bytes(data.up)}</div>
              </Card>
              <Card className="stats-tile">
                <div className="stats-label muted">
                  <i className="stats-dot stats-conns" />
                  {t("stats.conns")}
                </div>
                <div className="stats-big tnum">{data.conns.toLocaleString()}</div>
              </Card>
            </div>
            <Card
              title={hourly ? t("stats.byHour") : t("stats.byDay")}
              actions={
                <Segmented
                  value={sortBy}
                  onChange={setSortBy}
                  options={[
                    { value: "traffic", label: t("stats.byTraffic") },
                    { value: "conns", label: t("stats.byConns") },
                  ]}
                />
              }
            >
              <Bars points={hourly ? data.hours : data.days} hourly={hourly} byConns={sortBy === "conns"} />
              {data.partial && <div className="faint stats-partial">{t("stats.partial")}</div>}
            </Card>
            <div className="stats-toolbar">
              <SearchInput value={filter} onChange={setFilter} placeholder={t("stats.filter")} width={260} />
              {active.map(([dim, name]) => (
                <button key={dim} className="chip active stats-chip" onClick={() => pick(dim)({ name, up: 0, down: 0, other: name === OTHER })}>
                  <ListFilter size={12} />
                  <span className="faint">{t(`stats.dim.${dim}` as never)}</span>
                  {label(dim, name)}
                  <X size={12} />
                </button>
              ))}
              {active.length > 1 && (
                <button className="chip" onClick={() => setWhere({})}>
                  {t("stats.clearFilters")}
                </button>
              )}
              {narrowed && device === "" && (
                <Button size="sm" variant="ghost" onClick={findConnections} style={{ marginInlineStart: "auto" }}>
                  {t("stats.toConnections")}
                </Button>
              )}
            </div>
            <div className="grid-cards">
              {dims.map(({ dim, title, items }) => (
                <Ranking
                  key={dim}
                  title={title}
                  items={items(data)}
                  empty={emptyMsg}
                  byConns={sortBy === "conns"}
                  selected={where[dim]}
                  onPick={pick(dim)}
                  name={itemName(dim)}
                  note={
                    dim === "app" && findProcess !== "always" && data.apps.every((a) => a.other || a.name === "—") ? (
                      <div className="stats-note muted">
                        <span>{t("stats.appsHint")}</span>
                        <Button size="sm" onClick={() => void run(() => patchSettings({ clash: { findProcessMode: "always" } }), t("settings.saveFailed"))}>
                          {t("stats.appsEnable")}
                        </Button>
                      </div>
                    ) : undefined
                  }
                />
              ))}
            </div>
          </div>
        )}
      </div>
      <SettingsDialog open={settings} onClose={() => setSettings(false)} />
    </>
  );
}
