import { useVirtualizer } from "@tanstack/react-virtual";
import { ArrowDownToLine, Copy, Pause, Play, Trash2 } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { PageHeader } from "../components/Page";
import { useStream } from "../lib/hooks";
import { useT } from "../lib/i18n";
import { toast, useApp } from "../lib/store";
import { App, Logs as API, type LogEvent } from "../mygo";
import { Badge, Button, Empty, SearchInput, Segmented } from "../ui";

const LEVELS = ["debug", "info", "warning", "error"] as const;
type Level = (typeof LEVELS)[number] | "all";
const rank: Record<string, number> = { debug: 0, info: 1, warning: 2, error: 3 };
const tone: Record<string, "info" | "warning" | "danger" | undefined> = { info: "info", warning: "warning", error: "danger" };
const MAX = 5000;

export default function Logs() {
  const t = useT();
  const preview = useApp((s) => s.preview);
  const [logs, setLogs] = useState<LogEvent[]>([]);
  const [level, setLevel] = useState<Level>("all");
  const [search, setSearch] = useState("");
  const [paused, setPaused] = useState(false);
  const [follow, setFollow] = useState(true);
  const buffer = useRef<LogEvent[]>([]);
  useEffect(() => {
    if (!preview) API.recent().then((l) => setLogs(l.slice(-MAX))).catch(() => {});
  }, [preview]);
  useStream<LogEvent>(
    (ch) => API.stream(ch),
    (e) => {
      buffer.current.push(e);
    },
    [],
  );
  // Logs come in bursts: add them a few times a second.
  useEffect(() => {
    const id = setInterval(() => {
      if (paused || buffer.current.length === 0) return;
      const add = buffer.current;
      buffer.current = [];
      setLogs((l) => [...l, ...add].slice(-MAX));
    }, 250);
    return () => clearInterval(id);
  }, [paused]);
  const shown = useMemo(() => {
    const q = search.trim().toLowerCase();
    return logs.filter((l) => (level === "all" || (rank[l.type] ?? 1) >= rank[level]!) && (!q || l.payload.toLowerCase().includes(q)));
  }, [logs, level, search]);
  const scroller = useRef<HTMLDivElement>(null);
  const virt = useVirtualizer({ count: shown.length, getScrollElement: () => scroller.current, estimateSize: () => 30, overscan: 20 });
  useEffect(() => {
    if (follow && shown.length) virt.scrollToIndex(shown.length - 1, { align: "end" });
  }, [shown.length, follow, virt]);
  return (
    <>
      <PageHeader title={t("nav.logs")}>
        <Button size="sm" variant="ghost" icon={paused ? <Play size={14} /> : <Pause size={14} />} onClick={() => setPaused(!paused)}>
          {paused ? t("common.resume") : t("common.pause")}
        </Button>
        <Button
          size="sm"
          variant="ghost"
          icon={<Copy size={14} />}
          onClick={() =>
            App.copyText(shown.map((l) => `${new Date(l.time).toISOString()} [${l.type}] ${l.payload}`).join("\n")).then(() => toast({ level: "success", message: t("common.copied") }))
          }
        >
          {t("common.copy")}
        </Button>
        <Button
          size="sm"
          variant="ghost"
          icon={<Trash2 size={14} />}
          onClick={() => {
            setLogs([]);
            void API.clear();
          }}
        >
          {t("common.clear")}
        </Button>
      </PageHeader>
      <div className="page-body flush">
        <div className="list-toolbar">
          <Segmented
            value={level}
            onChange={setLevel}
            options={[{ value: "all" as Level, label: t("common.all") }, ...LEVELS.map((l) => ({ value: l as Level, label: t(`logs.${l}`) }))]}
          />
          <SearchInput value={search} onChange={setSearch} placeholder={t("logs.search")} width={280} />
          <div className="spacer" />
          <Button size="sm" variant={follow ? "primary" : "ghost"} icon={<ArrowDownToLine size={14} />} onClick={() => setFollow(!follow)} tip={t("logs.follow")} />
        </div>
        {shown.length === 0 ? (
          <Empty title={t("logs.empty")} art />
        ) : (
          <div
            className="vlist"
            ref={scroller}
            onWheel={(e) => {
              if (e.deltaY < 0 && follow) setFollow(false);
            }}
          >
            <div style={{ height: virt.getTotalSize(), position: "relative" }}>
              {virt.getVirtualItems().map((row) => {
                const l = shown[row.index]!;
                return (
                  <div
                    key={row.key}
                    data-index={row.index}
                    ref={virt.measureElement}
                    className="log-row"
                    style={{ position: "absolute", top: 0, left: 0, right: 0, transform: `translateY(${row.start}px)` }}
                  >
                    <span className="faint tnum mono" style={{ fontSize: 11.5 }}>
                      {new Date(l.time).toLocaleTimeString()}
                    </span>
                    <Badge tone={tone[l.type]}>{l.type}</Badge>
                    <span className="mono selectable log-text">{l.payload}</span>
                  </div>
                );
              })}
            </div>
          </div>
        )}
      </div>
    </>
  );
}
