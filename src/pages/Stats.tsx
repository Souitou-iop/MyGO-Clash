import { Trash2 } from "lucide-react";
import { useState } from "react";
import { PageHeader } from "../components/Page";
import { bytes } from "../lib/format";
import { useAsync, useInterval } from "../lib/hooks";
import { useT } from "../lib/i18n";
import { run } from "../lib/store";
import { Stats as API, type StatsItem, type StatsPoint } from "../mygo";
import { Button, Card, confirm, Empty, Segmented } from "../ui";

type Range = "today" | "7d" | "30d";

/** Bars is the traffic over time: the hours of a day, or the days of a range. */
function Bars({ points, hourly }: { points: StatsPoint[]; hourly: boolean }) {
  const max = Math.max(1, ...points.map((p) => p.up + p.down));
  const every = hourly ? 3 : points.length > 14 ? 5 : 1;
  return (
    <div className="stats-bars" style={{ gridTemplateColumns: `repeat(${points.length}, minmax(0, 1fr))` }}>
      {points.map((p, i) => {
        const label = hourly ? `${p.label}:00` : p.label.slice(5);
        const tip = `${hourly ? label : p.label}  ↓ ${bytes(p.down)}  ↑ ${bytes(p.up)}`;
        return (
          <div key={p.label} className="stats-col" title={tip}>
            <div className="stats-stack" style={{ height: `${((p.up + p.down) / max) * 100}%` }}>
              <i className="stats-up" style={{ flexGrow: p.up }} />
              <i className="stats-down" style={{ flexGrow: p.down }} />
            </div>
            <span className="stats-tick faint">{i % every === 0 ? label : ""}</span>
          </div>
        );
      })}
    </div>
  );
}

/** Ranking lists what carried the traffic, each row filled to its share of the busiest. */
function Ranking({ title, items }: { title: string; items: StatsItem[] }) {
  const t = useT();
  const max = Math.max(1, ...items.map((i) => i.up + i.down));
  return (
    <Card title={title} bodyClass="stats-rank">
      {items.length === 0 ? (
        <div className="faint stats-none">{t("stats.noData")}</div>
      ) : (
        items.map((it) => {
          const name = it.other ? t("stats.other") : it.name;
          return (
            <div key={it.other ? "\0other" : it.name} className="stats-row" title={`${name}  ↓ ${bytes(it.down)}  ↑ ${bytes(it.up)}`}>
              <span className="stats-fill" style={{ width: `${((it.up + it.down) / max) * 100}%` }} />
              <span className={`stats-name${it.other ? " muted" : ""}`}>{name}</span>
              <span className="stats-val tnum">{bytes(it.up + it.down)}</span>
            </div>
          );
        })
      )}
    </Card>
  );
}

export default function Stats() {
  const t = useT();
  const [range, setRange] = useState<Range>("today");
  const { data, reload } = useAsync(() => API.query({ range, top: 10 }), [range]);
  useInterval(() => void reload(), 10000);
  const clear = async () => {
    if (!(await confirm({ title: t("stats.clearTitle"), message: t("stats.clearMsg"), confirm: t("stats.clear"), danger: true }))) return;
    await run(() => API.clear(), t("common.failed"));
    void reload();
  };
  return (
    <>
      <PageHeader title={t("nav.stats")}>
        <Segmented
          value={range}
          onChange={setRange}
          options={[
            { value: "today", label: t("stats.today") },
            { value: "7d", label: t("stats.days7") },
            { value: "30d", label: t("stats.days30") },
          ]}
        />
        <Button size="sm" variant="danger" icon={<Trash2 size={14} />} onClick={clear}>
          {t("stats.clear")}
        </Button>
      </PageHeader>
      <div className="page-body">
        {!data || data.up + data.down === 0 ? (
          <Empty title={t("stats.empty")} art>
            <p className="muted">{t("stats.emptyHint")}</p>
          </Empty>
        ) : (
          <div className="stats-page">
            <div className="stats-summary">
              <Card className="stats-tile">
                <div className="stats-label muted">{t("stats.total")}</div>
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
            </div>
            <Card title={range === "today" ? t("stats.byHour") : t("stats.byDay")}>
              <Bars points={range === "today" ? data.hours : data.days} hourly={range === "today"} />
            </Card>
            <div className="grid-cards">
              <Ranking title={t("stats.apps")} items={data.apps} />
              <Ranking title={t("stats.sites")} items={data.sites} />
              <Ranking title={t("stats.nodes")} items={data.nodes} />
            </div>
          </div>
        )}
      </div>
    </>
  );
}
