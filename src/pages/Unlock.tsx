import { CheckCircle2, CircleDashed, HelpCircle, Loader2, MinusCircle, Play, RefreshCw, XCircle } from "lucide-react";
import { Channel } from "mygo-runtime";
import { useEffect, useState } from "react";
import { PageHeader } from "../components/Page";
import { flag, relative } from "../lib/format";
import { useT } from "../lib/i18n";
import { toastError, useApp } from "../lib/store";
import { Tools, type Unlock as Result } from "../mygo";
import { Badge, Banner, Button } from "../ui";

const look: Record<string, { icon: typeof CheckCircle2; tone: "success" | "warning" | "danger" | undefined; color: string }> = {
  yes: { icon: CheckCircle2, tone: "success", color: "var(--success)" },
  partial: { icon: MinusCircle, tone: "warning", color: "var(--warning)" },
  no: { icon: XCircle, tone: "danger", color: "var(--danger)" },
  failed: { icon: HelpCircle, tone: undefined, color: "var(--text-faint)" },
  pending: { icon: CircleDashed, tone: undefined, color: "var(--text-faint)" },
  checking: { icon: Loader2, tone: undefined, color: "var(--text-muted)" },
};

function load(): Record<string, Result> {
  try {
    return JSON.parse(localStorage.getItem("unlock.results") ?? "{}");
  } catch {
    return {};
  }
}

export default function Unlock() {
  const t = useT();
  const lang = useApp((s) => s.lang);
  const running = useApp((s) => s.state?.core.status === "running");
  const preview = useApp((s) => s.preview);
  const [items, setItems] = useState<Result[]>([]);
  const [results, setResults] = useState<Record<string, Result>>(load);
  const [checking, setChecking] = useState<Set<string>>(new Set());
  useEffect(() => {
    if (!preview) Tools.unlockList().then(setItems).catch(() => {});
  }, [preview]);
  const check = async (ids: string[]) => {
    setChecking((c) => new Set([...c, ...(ids.length ? ids : items.map((i) => i.id))]));
    const ch = new Channel<Result>((r) => {
      setResults((old) => {
        const next = { ...old, [r.id]: r };
        localStorage.setItem("unlock.results", JSON.stringify(next));
        return next;
      });
      setChecking((c) => {
        const n = new Set(c);
        n.delete(r.id);
        return n;
      });
    });
    try {
      await Tools.checkUnlock(ids, ch);
    } catch (e) {
      toastError(t("common.failed"), e);
    }
    setChecking(new Set());
  };
  const busy = checking.size > 0;
  return (
    <>
      <PageHeader title={t("nav.unlock")}>
        <Button variant="primary" size="sm" icon={busy ? <Loader2 size={14} className="spin" /> : <Play size={14} />} disabled={busy || !running} onClick={() => check([])}>
          {t("unlock.checkAll")}
        </Button>
      </PageHeader>
      <div className="page-body">
        {!running && <Banner tone="warning">{t("common.coreNotRunning")}</Banner>}
        <div className="muted" style={{ fontSize: 12.5, marginBottom: 12 }}>
          {t("unlock.hint")}
        </div>
        <div className="grid-cards" style={{ gridTemplateColumns: "repeat(auto-fill, minmax(250px, 1fr))" }}>
          {items.map((it) => {
            const r = results[it.id];
            const state = checking.has(it.id) ? "checking" : (r?.status ?? "pending");
            const l = look[state] ?? look.pending!;
            const Icon = l.icon;
            return (
              <div key={it.id} className="card card-pad col unlock-card" style={{ gap: 8 }}>
                <div className="row">
                  <Icon size={18} color={l.color} className={state === "checking" ? "spin" : ""} />
                  <span className="grow" style={{ fontWeight: 620 }}>
                    {it.name}
                  </span>
                  <Button size="sm" variant="ghost" icon={<RefreshCw size={13} />} disabled={state === "checking" || !running} onClick={() => check([it.id])} tip={t("common.retest")} />
                </div>
                <div className="row">
                  <Badge tone={l.tone}>{t(`unlock.${state}` as never)}</Badge>
                  {r?.region && (
                    <span style={{ fontSize: 13 }}>
                      {r.region.length === 2 ? flag(r.region) : ""} {r.region}
                    </span>
                  )}
                </div>
                <div className="faint" style={{ fontSize: 11.5, minHeight: 16 }}>
                  {r?.detail}
                  {r?.at && ` · ${relative(r.at, lang)}`}
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </>
  );
}
