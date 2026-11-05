import { useVirtualizer } from "@tanstack/react-virtual";
import { ArrowDown, ArrowUp, ChevronsUpDown, Pause, Play, Trash2, X, XCircle } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { PageHeader } from "../components/Page";
import { bytes, duration, rate, ruleType } from "../lib/format";
import { useInterval, useStream } from "../lib/hooks";
import { useT } from "../lib/i18n";
import { run, useApp } from "../lib/store";
import { Connections as API, type Closed, type Connection, type Connections as Snapshot, Stats } from "../mygo";
import { Badge, Button, confirm, Dialog, Empty, SearchInput, Segmented } from "../ui";
import { CoreDown } from "../components/CoreDown";

type SortKey = "start" | "host" | "down" | "up" | "downTotal" | "upTotal";
type View = "active" | "closed";

function host(c: Connection): string {
  const m = c.metadata;
  const h = m.host || m.sniffHost || m.destinationIP;
  return `${h}:${m.destinationPort}`;
}

/** fromClosed shapes a closed connection of the history like an open one. */
function fromClosed(c: Closed): Connection & { end: string } {
  return {
    id: c.id,
    start: c.start,
    end: c.end,
    upload: c.up,
    download: c.down,
    chains: c.chains ?? [],
    rule: c.rule,
    rulePayload: c.rulePayload ?? "",
    metadata: {
      network: c.network,
      type: c.type,
      sourceIP: c.sourceIP ?? "",
      destinationIP: c.destinationIP ?? "",
      sourcePort: "",
      destinationPort: c.destinationPort ?? "",
      host: c.host,
      process: c.process,
      processPath: c.processPath,
      destinationGeoIP: c.country ? [c.country] : undefined,
    },
  } as Connection & { end: string };
}

function chain(c: Connection): string {
  return [...(c.chains ?? [])].reverse().join(" → ");
}

const COLS = "minmax(200px, 2.2fr) 74px minmax(110px, 1fr) minmax(130px, 1.2fr) minmax(140px, 1.4fr) 82px 82px 80px 80px 66px 30px";

function Detail({ c, active, onClose }: { c: Connection | null; active: boolean; onClose: () => void }) {
  const t = useT();
  if (!c) return null;
  const m = c.metadata;
  const rows: [string, string][] = [
    [t("conn.host"), host(c)],
    [t("conn.network"), `${m.network} · ${m.type}`],
    [t("conn.source"), `${m.sourceIP}:${m.sourcePort}`],
    [t("conn.destination"), `${m.destinationIP || "—"}:${m.destinationPort}`],
    ["SNI", m.sniffHost || "—"],
    [t("conn.process"), m.process || "—"],
    [t("conn.processPath"), m.processPath || "—"],
    [t("conn.rule"), `${ruleType(c.rule, c.rulePayload)}${c.rulePayload ? ` (${c.rulePayload})` : ""}`],
    [t("conn.chain"), chain(c)],
    [t("conn.inbound"), [m.inboundName, m.inboundIP && `${m.inboundIP}:${m.inboundPort}`].filter(Boolean).join(" · ") || "—"],
    [t("conn.dnsMode"), m.dnsMode || "—"],
    [t("conn.remote"), m.remoteDestination || "—"],
    ["GeoIP", (m.destinationGeoIP ?? []).join(", ") || "—"],
    ["ASN", m.destinationIPASN || "—"],
    [t("conn.traffic"), `↑ ${bytes(c.upload)}  ↓ ${bytes(c.download)}`],
    [t("conn.started"), new Date(c.start).toLocaleString()],
  ];
  return (
    <Dialog
      open
      onClose={onClose}
      title={
        <>
          {t("conn.details")} {!active && <Badge>{t("conn.closed")}</Badge>}
        </>
      }
      size="wide"
      footer={
        active ? (
          <Button variant="danger" icon={<XCircle size={14} />} onClick={() => run(() => API.close([c.id]), t("common.failed")).then(onClose)}>
            {t("conn.close")}
          </Button>
        ) : undefined
      }
    >
      <dl className="kv" style={{ gridTemplateColumns: "140px 1fr" }}>
        {rows.map(([k, v]) => (
          <div key={k} style={{ display: "contents" }}>
            <dt>{k}</dt>
            <dd className="selectable mono" style={{ textAlign: "start", whiteSpace: "normal", wordBreak: "break-all" }}>
              {v}
            </dd>
          </div>
        ))}
      </dl>
    </Dialog>
  );
}

export default function Connections() {
  const t = useT();
  const running = useApp((s) => s.state?.core.status === "running");
  const preset = useApp((s) => s.connPreset);
  const [snap, setSnap] = useState<Snapshot | null>(null);
  const [closed, setClosed] = useState<(Connection & { end: string })[]>([]);
  const [closedTotal, setClosedTotal] = useState(0);
  const [paused, setPaused] = useState(false);
  const [search, setSearch] = useState(preset?.search ?? "");
  const [net, setNet] = useState<"all" | "tcp" | "udp">("all");
  const [view, setView] = useState<View>(preset?.view ?? "active");
  useEffect(() => {
    if (preset) useApp.setState({ connPreset: null });
  }, [preset]);
  // The closed connections come from the history the statistics keep,
  // which goes on while the page is closed.
  const loadClosed = async () => {
    const page = await Stats.closed({ offset: 0, limit: 500 }).catch(() => null);
    if (!page) return;
    setClosed(page.items.map(fromClosed));
    setClosedTotal(page.total);
  };
  useEffect(() => void loadClosed(), []);
  useInterval(() => {
    if (!paused) void loadClosed();
  }, view === "closed" ? 3000 : 15000);
  const [sort, setSort] = useState<{ key: SortKey; desc: boolean }>({ key: "start", desc: true });
  // The connection shown in detail, kept up to date while it lasts.
  const [detail, setDetail] = useState<Connection | null>(null);
  const live = detail && snap?.connections.find((c) => c.id === detail.id);
  const prev = useRef<Map<string, Connection>>(new Map());
  useStream<Snapshot>(
    (ch) => API.stream(ch),
    (s) => {
      if (paused) return;
      prev.current = new Map(s.connections.map((c) => [c.id, c] as const));
      setSnap(s);
    },
    [],
    running,
  );
  const list = useMemo(() => {
    const src = view === "active" ? (snap?.connections ?? []) : closed;
    const q = search.trim().toLowerCase();
    let out = src.filter((c) => (net === "all" || c.metadata.network === net) && (!q || [host(c), c.metadata.process, ruleType(c.rule, c.rulePayload), c.rulePayload, chain(c), c.metadata.sourceIP].some((v) => v?.toLowerCase().includes(q))));
    const key = (c: Connection): number | string => {
      switch (sort.key) {
        case "host":
          return host(c);
        case "down":
          return c.downloadSpeed ?? 0;
        case "up":
          return c.uploadSpeed ?? 0;
        case "downTotal":
          return c.download;
        case "upTotal":
          return c.upload;
        default:
          return new Date(c.start).getTime();
      }
    };
    out = [...out].sort((a, b) => {
      const x = key(a);
      const y = key(b);
      const r = typeof x === "string" ? x.localeCompare(y as string) : x - (y as number);
      return sort.desc ? -r : r;
    });
    return out;
  }, [snap, closed, view, search, net, sort]);
  const scroller = useRef<HTMLDivElement>(null);
  const virt = useVirtualizer({ count: list.length, getScrollElement: () => scroller.current, estimateSize: () => 36, overscan: 12 });
  const head = (key: SortKey, label: string) => (
    <button onClick={() => setSort((s) => ({ key, desc: s.key === key ? !s.desc : true }))}>
      {label}
      {sort.key === key ? sort.desc ? <ArrowDown size={11} /> : <ArrowUp size={11} /> : <ChevronsUpDown size={11} opacity={0.4} />}
    </button>
  );
  const closeFiltered = async () => {
    const all = !search && net === "all";
    if (!(await confirm({ title: t("conn.closeAllTitle"), message: all ? t("conn.closeAllMsg") : t("conn.closeSomeMsg", { n: list.length }), confirm: t("conn.close"), danger: true }))) return;
    void run(() => (all ? API.closeAll() : API.close(list.map((c) => c.id))), t("common.failed"));
  };
  return (
    <>
      <PageHeader
        title={t("nav.connections")}
        sub={
          snap && (
            <span className="muted tnum" style={{ fontSize: 12 }}>
              ↓ {bytes(snap.downloadTotal)} · ↑ {bytes(snap.uploadTotal)}
            </span>
          )
        }
      >
        <Button size="sm" variant="ghost" icon={paused ? <Play size={14} /> : <Pause size={14} />} onClick={() => setPaused(!paused)}>
          {paused ? t("common.resume") : t("common.pause")}
        </Button>
        {view === "closed" ? (
          <Button
            size="sm"
            variant="danger"
            icon={<Trash2 size={14} />}
            disabled={closedTotal === 0}
            onClick={async () => {
              if (await confirm({ title: t("conn.clearClosedTitle"), message: t("conn.clearClosedMsg"), confirm: t("common.clear"), danger: true })) {
                await run(() => Stats.clearClosed(), t("common.failed"));
                void loadClosed();
              }
            }}
          >
            {t("conn.clearClosed")}
          </Button>
        ) : (
          <Button size="sm" variant="danger" icon={<XCircle size={14} />} onClick={closeFiltered} disabled={list.length === 0}>
            {search || net !== "all" ? t("conn.closeFiltered") : t("conn.closeAll")}
          </Button>
        )}
      </PageHeader>
      <div className="page-body flush">
        <div className="list-toolbar">
          <Segmented
            value={view}
            onChange={setView}
            options={[
              { value: "active", label: `${t("conn.active")} ${snap?.connections.length ?? 0}` },
              { value: "closed", label: `${t("conn.closed")} ${closedTotal}` },
            ]}
          />
          <Segmented
            value={net}
            onChange={setNet}
            options={[
              { value: "all", label: t("common.all") },
              { value: "tcp", label: "TCP" },
              { value: "udp", label: "UDP" },
            ]}
          />
          <SearchInput value={search} onChange={setSearch} placeholder={t("conn.search")} width={280} />
        </div>
        {!running ? (
          <CoreDown />
        ) : list.length === 0 ? (
          <Empty title={t("conn.empty")} art />
        ) : (
          <div className="vlist" ref={scroller}>
            <div style={{ minWidth: 1100 }}>
              <div className="table-head" style={{ gridTemplateColumns: COLS }}>
                {head("host", t("conn.host"))}
                <span>{t("conn.network")}</span>
                <span>{t("conn.process")}</span>
                <span>{t("conn.rule")}</span>
                <span>{t("conn.chain")}</span>
                {head("down", t("conn.dlSpeed"))}
                {head("up", t("conn.ulSpeed"))}
                {head("downTotal", t("conn.dl"))}
                {head("upTotal", t("conn.ul"))}
                {head("start", t("conn.time"))}
                <span />
              </div>
              <div style={{ height: virt.getTotalSize(), position: "relative" }}>
                {virt.getVirtualItems().map((row) => {
                  const c = list[row.index]!;
                  return (
                    <div
                      key={c.id}
                      className="table-row"
                      style={{ gridTemplateColumns: COLS, position: "absolute", top: 0, left: 0, right: 0, transform: `translateY(${row.start}px)` }}
                      onClick={() => setDetail(c)}
                    >
                      <span className="cell mono" title={host(c)}>
                        {host(c)}
                      </span>
                      <span className="cell">
                        <Badge tone={c.metadata.network === "udp" ? "warning" : undefined}>{c.metadata.network.toUpperCase()}</Badge>
                      </span>
                      <span className="cell muted" title={c.metadata.processPath}>
                        {c.metadata.process || "—"}
                      </span>
                      <span className="cell" title={c.rulePayload}>
                        {ruleType(c.rule, c.rulePayload)}
                        {c.rulePayload && <span className="faint"> {c.rulePayload}</span>}
                      </span>
                      <span className="cell" title={chain(c)}>
                        {chain(c)}
                      </span>
                      <span className="cell tnum">{view === "active" ? rate(c.downloadSpeed ?? 0) : "—"}</span>
                      <span className="cell tnum">{view === "active" ? rate(c.uploadSpeed ?? 0) : "—"}</span>
                      <span className="cell tnum muted">{bytes(c.download)}</span>
                      <span className="cell tnum muted">{bytes(c.upload)}</span>
                      <span className="cell tnum faint" title={view === "closed" ? new Date((c as Connection & { end?: string }).end ?? c.start).toLocaleString() : undefined}>
                        {duration((view === "closed" ? new Date((c as Connection & { end?: string }).end ?? Date.now()).getTime() : Date.now()) - new Date(c.start).getTime())}
                      </span>
                      <span className="cell" onClick={(e) => e.stopPropagation()}>
                        {view === "active" && <Button size="sm" variant="ghost" icon={<X size={13} />} tip={t("conn.close")} onClick={() => run(() => API.close([c.id]), t("common.failed"))} />}
                      </span>
                    </div>
                  );
                })}
              </div>
            </div>
          </div>
        )}
      </div>
      <Detail c={live ?? closed.find((c) => c.id === detail?.id) ?? detail} active={!!live} onClose={() => setDetail(null)} />
    </>
  );
}
