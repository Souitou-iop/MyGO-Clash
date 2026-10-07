import { useVirtualizer } from "@tanstack/react-virtual";
import { ArrowDown, ArrowUp, ChevronsUpDown, Copy, CornerLeftUp, Info, MoreHorizontal, Pencil, RefreshCw, SquareArrowOutUpRight } from "lucide-react";
import { useMemo, useRef, useState } from "react";
import { EditorDialog } from "../components/profiles/EditorDialog";
import { RULE_TYPES, SeqEditor, type RuleDraft } from "../components/profiles/SeqEditor";
import { PageHeader } from "../components/Page";
import { relative } from "../lib/format";
import { useAsync } from "../lib/hooks";
import { useT } from "../lib/i18n";
import { run, toast, useApp } from "../lib/store";
import { App, Profiles, Proxies, Rules as API, type Rule } from "../mygo";
import { Badge, Banner, Button, Empty, Menu, reflow, SearchInput, Select, Spinner, Switch, Tabs } from "../ui";

const COLS = "52px 150px minmax(220px, 2fr) minmax(140px, 1fr) 90px 52px 32px";

/** SYNTAX spells the core's rule types as configurations write them. */
const SYNTAX: Record<string, string> = {
  Domain: "DOMAIN",
  DomainSuffix: "DOMAIN-SUFFIX",
  DomainKeyword: "DOMAIN-KEYWORD",
  DomainRegex: "DOMAIN-REGEX",
  DomainWildcard: "DOMAIN-WILDCARD",
  GeoSite: "GEOSITE",
  GeoIP: "GEOIP",
  SrcGeoIP: "SRC-GEOIP",
  IPASN: "IP-ASN",
  SrcIPASN: "SRC-IP-ASN",
  IPCIDR: "IP-CIDR",
  SrcIPCIDR: "SRC-IP-CIDR",
  IPSuffix: "IP-SUFFIX",
  SrcIPSuffix: "SRC-IP-SUFFIX",
  SrcPort: "SRC-PORT",
  DstPort: "DST-PORT",
  InPort: "IN-PORT",
  InUser: "IN-USER",
  InName: "IN-NAME",
  InType: "IN-TYPE",
  ProcessName: "PROCESS-NAME",
  ProcessPath: "PROCESS-PATH",
  ProcessNameRegex: "PROCESS-NAME-REGEX",
  ProcessPathRegex: "PROCESS-PATH-REGEX",
  ProcessNameWildcard: "PROCESS-NAME-WILDCARD",
  ProcessPathWildcard: "PROCESS-PATH-WILDCARD",
  Match: "MATCH",
  RuleSet: "RULE-SET",
  Network: "NETWORK",
  DSCP: "DSCP",
  Uid: "UID",
  SubRules: "SUB-RULE",
};

function syntax(r: Rule): string {
  const s = SYNTAX[r.type] ?? r.type.toUpperCase();
  return s === "IP-CIDR" && r.payload.includes(":") ? "IP-CIDR6" : s;
}

/** line writes a rule as a configuration does. */
function line(r: Rule): string {
  const type = syntax(r);
  return [type, ...(type === "MATCH" ? [] : [r.payload]), r.proxy].join(",");
}

type Status = "" | "hit" | "unhit" | "off";
type Sort = { key: "index" | "hits"; desc: boolean };

export default function Rules() {
  const t = useT();
  const lang = useApp((s) => s.lang);
  const runtime = useApp((s) => s.runtime);
  const running = useApp((s) => s.state?.core.status === "running");
  const profile = useApp((s) => s.state?.profileUid ?? "");
  const profileName = useApp((s) => s.state?.profileName ?? "");
  const navigate = useApp((s) => s.navigate);
  const { data, loading, reload, setData } = useAsync(() => API.list(), [runtime, running]);
  const { data: proxies } = useAsync(() => (running ? Proxies.view() : Promise.resolve(undefined)), [runtime, running]);
  const groups = useMemo(() => new Set((proxies?.groups ?? []).map((g) => g.name)), [proxies]);
  const [tab, setTab] = useState<"rules" | "providers">("rules");
  const [search, setSearch] = useState("");
  const [type, setType] = useState("");
  const [status, setStatus] = useState<Status>("");
  const [sort, setSort] = useState<Sort>({ key: "index", desc: false });
  const [busy, setBusy] = useState<Record<string, boolean>>({});
  const [restoring, setRestoring] = useState(false);
  const [seq, setSeq] = useState<{ uid: string; title: string; draft?: RuleDraft } | null>(null);
  const [raw, setRaw] = useState<{ uid: string; title: string } | null>(null);
  const all = data?.rules ?? [];
  const types = useMemo(() => [...new Set(all.map(syntax))].sort(), [all]);
  const off = useMemo(() => all.filter((r) => r.extra?.disabled).length, [all]);
  const rules = useMemo(() => {
    const q = search.trim().toLowerCase();
    const hits = (r: Rule) => r.extra?.hitCount ?? 0;
    const out = all.filter(
      (r) =>
        (!type || syntax(r) === type) &&
        (!status || (status === "off" ? !!r.extra?.disabled : status === "hit" ? hits(r) > 0 : hits(r) === 0)) &&
        (!q || r.payload.toLowerCase().includes(q) || r.proxy.toLowerCase().includes(q) || syntax(r).toLowerCase().includes(q)),
    );
    if (sort.key === "hits") out.sort((a, b) => hits(a) - hits(b) || b.index - a.index);
    if (sort.desc) out.reverse();
    return out;
  }, [all, search, type, status, sort]);
  const filtered = !!(search || type || status);
  const scroller = useRef<HTMLDivElement>(null);
  const virt = useVirtualizer({ count: rules.length, getScrollElement: () => scroller.current, estimateSize: () => 36, overscan: 15 });
  const toggle = async (r: Rule, disabled: boolean) => {
    setData((d) => d && { ...d, rules: d.rules.map((x) => (x.index === r.index ? { ...x, extra: { ...(x.extra ?? { hitCount: 0, hitAt: "", missCount: 0, missAt: "" }), disabled } } : x)) });
    await run(() => API.setDisabled(r.index, disabled), t("common.failed"));
  };
  const restore = async () => {
    setRestoring(true);
    const n = await run(() => API.enableAll(), t("common.failed"));
    setRestoring(false);
    if (n !== undefined) toast({ level: "success", message: t("rules.enabledAll", { n }) });
    void reload();
  };
  const update = async (name: string) => {
    setBusy((b) => ({ ...b, [name]: true }));
    await run(() => API.updateProvider(name), t("rules.updateFailed"));
    setBusy((b) => ({ ...b, [name]: false }));
    void reload();
  };
  // The rules extension of the current profile, with a rule to start from.
  const edit = async (draft?: RuleDraft) => {
    const uid = await run(() => Profiles.extension(profile, "rules"), t("common.failed"));
    if (uid) setSeq({ uid, title: `${t("profiles.ext.rules")} · ${profileName}`, draft });
  };
  const goGroup = (name: string) => {
    useApp.setState({ focusGroup: name });
    navigate("proxies");
  };
  const head = (key: Sort["key"], label: string) => (
    <button onClick={() => setSort((s) => (s.key === key ? { key, desc: !s.desc } : { key, desc: key === "hits" }))}>
      {label}
      {sort.key === key ? sort.desc ? <ArrowDown size={11} /> : <ArrowUp size={11} /> : <ChevronsUpDown size={11} opacity={0.4} />}
    </button>
  );
  return (
    <>
      <PageHeader title={t("nav.rules")} sub={data && <span className="muted" style={{ fontSize: 12 }}>{t("rules.count", { n: all.length })}</span>}>
        {running && profile && (
          <Button size="sm" variant="ghost" icon={<Pencil size={14} />} onClick={() => edit()} tip={t("rules.editTip")}>
            {t("rules.edit")}
          </Button>
        )}
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
          <Empty title={t("common.coreNotRunning")}>
            <p className="muted">{t("rules.coreHint")}</p>
            <Button onClick={() => navigate("home")}>{t("rules.goHome")}</Button>
          </Empty>
        ) : loading && !data ? (
          <div className="empty">
            <Spinner />
          </div>
        ) : tab === "rules" ? (
          all.length === 0 ? (
            <Empty title={t("rules.empty")} art>
              <p className="muted">{t("rules.emptyHint")}</p>
              <Button onClick={() => navigate("profiles")}>{t("rules.goProfiles")}</Button>
            </Empty>
          ) : (
            <>
              <p className="rules-intro muted">
                <Info size={13} />
                {t("rules.intro")}
              </p>
              {off > 0 && (
                <Banner
                  tone="info"
                  action={
                    <Button size="sm" loading={restoring} onClick={restore}>
                      {t("rules.enableAll")}
                    </Button>
                  }
                >
                  {t("rules.offCount", { n: off })}
                </Banner>
              )}
              <div className="list-toolbar">
                <SearchInput value={search} onChange={setSearch} placeholder={t("rules.search")} width={260} />
                <Select value={type} onChange={setType} options={[{ value: "", label: t("rules.allTypes") }, ...types.map((x) => ({ value: x, label: x }))]} width={170} />
                <Select<Status>
                  value={status}
                  onChange={setStatus}
                  options={[
                    { value: "", label: t("rules.status.all") },
                    { value: "hit", label: t("rules.status.hit") },
                    { value: "unhit", label: t("rules.status.unhit") },
                    { value: "off", label: t("rules.status.off") },
                  ]}
                  width={130}
                />
                <span className="muted" style={{ fontSize: 12 }}>
                  {t("rules.shown", { n: rules.length })}
                </span>
              </div>
              {rules.length === 0 ? (
                <Empty title={t("rules.noMatch")}>
                  {filtered && (
                    <Button
                      onClick={() => {
                        setSearch("");
                        setType("");
                        setStatus("");
                      }}
                    >
                      {t("rules.clearFilters")}
                    </Button>
                  )}
                </Empty>
              ) : (
                <div className="vlist" ref={scroller}>
                  <div className="table-head" style={{ gridTemplateColumns: COLS }}>
                    {head("index", "#")}
                    <span>{t("rules.type")}</span>
                    <span>{t("rules.payload")}</span>
                    <span>{t("rules.target")}</span>
                    {head("hits", t("rules.hits"))}
                    <span />
                    <span />
                  </div>
                  <div style={{ height: virt.getTotalSize(), position: "relative" }}>
                    {virt.getVirtualItems().map((row) => {
                      const r = rules[row.index]!;
                      const disabled = !!r.extra?.disabled;
                      const group = groups.has(r.proxy);
                      const draftable = RULE_TYPES.includes(syntax(r)) && syntax(r) !== "MATCH" && !!profile;
                      return (
                        <div key={r.index} className={`table-row${disabled ? " off" : ""}`} style={{ gridTemplateColumns: COLS, position: "absolute", top: 0, left: 0, right: 0, transform: `translateY(${row.start}px)` }}>
                          <span className="cell faint tnum">{r.index + 1}</span>
                          <span className="cell">
                            <Badge>{syntax(r)}</Badge>
                          </span>
                          <span className="cell mono selectable" title={r.payload}>
                            {r.payload || "—"}
                            {r.size > 0 && <span className="faint"> ({r.size})</span>}
                          </span>
                          <span className="cell" style={{ fontWeight: 560 }}>
                            {group ? (
                              <button className="rule-target" onClick={() => goGroup(r.proxy)} title={t("rules.goGroup")}>
                                {r.proxy}
                              </button>
                            ) : (
                              r.proxy
                            )}
                          </span>
                          <span className="cell tnum muted" title={r.extra?.hitAt ? t("rules.lastHit", { when: relative(r.extra.hitAt, lang) }) : ""}>
                            {r.extra?.hitCount ?? 0}
                          </span>
                          <span className="cell" title={t("rules.offTip")}>
                            {r.extra && <Switch checked={!disabled} onChange={(v) => toggle(r, !v)} label={t("rules.enabled")} />}
                          </span>
                          <span className="cell">
                            <Menu
                              trigger={(open) => <Button size="sm" variant="ghost" icon={<MoreHorizontal size={14} />} onClick={open} aria-label={t("rules.actions")} />}
                              items={[
                                { label: t("rules.copy"), icon: <Copy size={14} />, onClick: () => App.copyText(line(r)).then(() => toast({ level: "success", message: t("common.copied") })) },
                                ...(group ? [{ label: t("rules.goGroup"), icon: <SquareArrowOutUpRight size={14} />, onClick: () => goGroup(r.proxy) }] : []),
                                ...(draftable ? [{ label: t("rules.override"), icon: <CornerLeftUp size={14} />, onClick: () => edit({ type: syntax(r), payload: r.payload }) }] : []),
                              ]}
                            />
                          </span>
                        </div>
                      );
                    })}
                  </div>
                </div>
              )}
            </>
          )
        ) : (data?.providers.length ?? 0) === 0 ? (
          <Empty title={t("rules.noProviders")} />
        ) : (
          <div style={{ overflow: "auto" }}>
            <div className="list-toolbar">
              <p className="rules-intro muted grow" style={{ margin: 0 }}>
                <Info size={13} />
                {t("rules.providersHint")}
              </p>
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
      <SeqEditor
        uid={seq?.uid ?? null}
        profileUid={profile}
        kind="rules"
        title={seq?.title ?? ""}
        draft={seq?.draft}
        onClose={() => setSeq(null)}
        onRaw={() => {
          if (seq) setRaw({ uid: seq.uid, title: seq.title });
          setSeq(null);
        }}
      />
      <EditorDialog uid={raw?.uid ?? null} title={raw?.title ?? ""} lang="yaml" onClose={() => setRaw(null)} />
    </>
  );
}
