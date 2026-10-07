import { useVirtualizer } from "@tanstack/react-virtual";
import { RefreshCw } from "lucide-react";
import { useMemo, useRef, useState } from "react";
import { PageHeader } from "../components/Page";
import { relative } from "../lib/format";
import { useAsync } from "../lib/hooks";
import { useT } from "../lib/i18n";
import { run, useApp } from "../lib/store";
import { Rules as API, type Rule } from "../mygo";
import { Badge, Button, Empty, reflow, SearchInput, Select, Spinner, Switch, Tabs } from "../ui";

const COLS = "52px 150px minmax(220px, 2fr) minmax(140px, 1fr) 90px 52px";

export default function Rules() {
  const t = useT();
  const lang = useApp((s) => s.lang);
  const runtime = useApp((s) => s.runtime);
  const running = useApp((s) => s.state?.core.status === "running");
  const { data, loading, reload, setData } = useAsync(() => API.list(), [runtime, running]);
  const [tab, setTab] = useState<"rules" | "providers">("rules");
  const [search, setSearch] = useState("");
  const [type, setType] = useState("");
  const [busy, setBusy] = useState<Record<string, boolean>>({});
  const types = useMemo(() => [...new Set((data?.rules ?? []).map((r) => r.type))].sort(), [data]);
  const rules = useMemo(() => {
    const q = search.trim().toLowerCase();
    return (data?.rules ?? []).filter((r) => (!type || r.type === type) && (!q || r.payload.toLowerCase().includes(q) || r.proxy.toLowerCase().includes(q) || r.type.toLowerCase().includes(q)));
  }, [data, search, type]);
  const scroller = useRef<HTMLDivElement>(null);
  const virt = useVirtualizer({ count: rules.length, getScrollElement: () => scroller.current, estimateSize: () => 36, overscan: 15 });
  const toggle = async (r: Rule, disabled: boolean) => {
    setData((d) => d && { ...d, rules: d.rules.map((x) => (x.index === r.index ? { ...x, extra: { ...(x.extra ?? { hitCount: 0, hitAt: "", missCount: 0, missAt: "" }), disabled } } : x)) });
    await run(() => API.setDisabled(r.index, disabled), t("common.failed"));
  };
  const update = async (name: string) => {
    setBusy((b) => ({ ...b, [name]: true }));
    await run(() => API.updateProvider(name), t("rules.updateFailed"));
    setBusy((b) => ({ ...b, [name]: false }));
    void reload();
  };
  return (
    <>
      <PageHeader title={t("nav.rules")} sub={data && <span className="muted" style={{ fontSize: 12 }}>{t("rules.count", { n: data.rules.length })}</span>}>
        <Button size="sm" variant="ghost" icon={<RefreshCw size={14} />} onClick={reload} tip={t("common.refresh")} />
      </PageHeader>
      <div className="page-body flush">
        <Tabs
          value={tab}
          onChange={setTab}
          tabs={[
            { value: "rules", label: t("rules.rules") },
            { value: "providers", label: `${t("rules.providers")} (${data?.providers.length ?? 0})` },
          ]}
        />
        {!running ? (
          <Empty title={t("common.coreNotRunning")} />
        ) : loading && !data ? (
          <div className="empty">
            <Spinner />
          </div>
        ) : tab === "rules" ? (
          <>
            <div className="list-toolbar">
              <SearchInput value={search} onChange={setSearch} placeholder={t("rules.search")} width={300} />
              <Select value={type} onChange={setType} options={[{ value: "", label: t("rules.allTypes") }, ...types.map((x) => ({ value: x, label: x }))]} width={170} />
              <span className="muted" style={{ fontSize: 12 }}>
                {t("rules.shown", { n: rules.length })}
              </span>
            </div>
            {rules.length === 0 ? (
              <Empty title={t("rules.empty")} art />
            ) : (
              <div className="vlist" ref={scroller}>
                <div className="table-head" style={{ gridTemplateColumns: COLS }}>
                  <span>#</span>
                  <span>{t("rules.type")}</span>
                  <span>{t("rules.payload")}</span>
                  <span>{t("rules.target")}</span>
                  <span>{t("rules.hits")}</span>
                  <span />
                </div>
                <div style={{ height: virt.getTotalSize(), position: "relative" }}>
                  {virt.getVirtualItems().map((row) => {
                    const r = rules[row.index]!;
                    const off = !!r.extra?.disabled;
                    return (
                      <div key={r.index} className="table-row" style={{ gridTemplateColumns: COLS, position: "absolute", top: 0, left: 0, right: 0, transform: `translateY(${row.start}px)`, opacity: off ? 0.45 : 1 }}>
                        <span className="cell faint tnum">{r.index + 1}</span>
                        <span className="cell">
                          <Badge>{r.type}</Badge>
                        </span>
                        <span className="cell mono selectable" title={r.payload}>
                          {r.payload || "—"}
                          {r.size > 0 && <span className="faint"> ({r.size})</span>}
                        </span>
                        <span className="cell" style={{ fontWeight: 560 }}>
                          {r.proxy}
                        </span>
                        <span className="cell tnum muted" title={r.extra?.hitAt ? relative(r.extra.hitAt, lang) : ""}>
                          {r.extra?.hitCount ?? 0}
                        </span>
                        <span className="cell">{r.extra && <Switch checked={!off} onChange={(v) => toggle(r, !v)} label={t("rules.enabled")} />}</span>
                      </div>
                    );
                  })}
                </div>
              </div>
            )}
          </>
        ) : (data?.providers.length ?? 0) === 0 ? (
          <Empty title={t("rules.noProviders")} />
        ) : (
          <div style={{ overflow: "auto" }}>
            <div className="list-toolbar">
              <Button
                size="sm"
                icon={<RefreshCw size={13} />}
                onClick={async () => {
                  for (const p of data!.providers) await update(p.name);
                }}
              >
                {t("proxies.updateAll")}
              </Button>
            </div>
            <div className="grid-cards" ref={reflow}>
              {data!.providers.map((p) => (
                <div key={p.name} className="card card-pad row">
                  <div className="grow">
                    <div style={{ fontWeight: 620 }} className="ellipsis">
                      {p.name}
                    </div>
                    <div className="muted" style={{ fontSize: 12 }}>
                      {p.behavior} · {p.format || "yaml"} · {p.vehicleType} · {t("rules.ruleCount", { n: p.ruleCount })}
                    </div>
                    <div className="faint" style={{ fontSize: 11.5 }}>
                      {t("profiles.updatedAgo", { when: relative(p.updatedAt, lang) })}
                    </div>
                  </div>
                  <Button size="sm" variant="ghost" icon={<RefreshCw size={14} />} loading={busy[p.name]} onClick={() => update(p.name)} tip={t("proxies.updateProvider")} />
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </>
  );
}
