import {
  CheckCircle2,
  Code2,
  Download,
  ExternalLink,
  FileCode2,
  FilePlus2,
  FileStack,
  FileUp,
  Globe,
  GripVertical,
  Info,
  ListOrdered,
  Merge,
  MoreHorizontal,
  Network,
  Pencil,
  RefreshCw,
  ScrollText,
  Server,
  Trash2,
  Waypoints,
} from "lucide-react";
import { lazy, Suspense, useEffect, useState } from "react";
import { EditorDialog } from "../components/profiles/EditorDialog";
import { ProfileDialog } from "../components/profiles/ProfileDialog";
import { type SeqKind, SeqEditor } from "../components/profiles/SeqEditor";
import { PageHeader } from "../components/Page";
import { bytes, dateOnly, percent, relative } from "../lib/format";
import { useNow } from "../lib/hooks";
import { useT } from "../lib/i18n";
import { run, toast, toastError, useApp } from "../lib/store";
import { App, Profiles as API, type LogEntry, type Profile } from "../mygo";
import { Badge, Button, confirm, Dialog, Empty, Menu, Progress, reflow, Spinner } from "../ui";

const CodeEditor = lazy(() => import("../components/CodeEditor"));

type Editing = { uid: string; title: string; lang: "yaml" | "javascript"; readOnly?: boolean } | null;
type SeqEditing = { uid: string; profile: string; kind: SeqKind; title: string } | null;

function ProfileCard({
  p,
  current,
  next,
  dns,
  onEdit,
  onEditFile,
  onExt,
  onDragStart,
  onDrop,
}: {
  p: Profile;
  current: boolean;
  next?: string;
  dns: boolean;
  onEdit: () => void;
  onEditFile: () => void;
  onExt: (kind: SeqKind | "merge" | "script") => void;
  onDragStart: () => void;
  onDrop: () => void;
}) {
  const t = useT();
  const lang = useApp((s) => s.lang);
  const [busy, setBusy] = useState("");
  const [over, setOver] = useState(false);
  const used = (p.usage?.upload ?? 0) + (p.usage?.download ?? 0);
  const total = p.usage?.total ?? 0;
  const pct = percent(used, total);
  const expired = !!p.usage?.expire && p.usage.expire * 1000 < Date.now();
  const act = async (key: string, fn: () => Promise<unknown>, fail: string, ok?: string) => {
    setBusy(key);
    await run(fn, fail, ok);
    setBusy("");
  };
  const activate = () => !current && act("activate", () => API.activate(p.uid), t("profiles.activateFailed"));
  return (
    <div
      className={`card profile-card${current ? " current" : ""}${over ? " drop" : ""}`}
      draggable
      onDragStart={(e) => {
        e.dataTransfer.effectAllowed = "move";
        onDragStart();
      }}
      onDragOver={(e) => {
        e.preventDefault();
        setOver(true);
      }}
      onDragLeave={() => setOver(false)}
      onDrop={(e) => {
        e.preventDefault();
        setOver(false);
        onDrop();
      }}
      onDoubleClick={activate}
    >
      <div className="row" style={{ alignItems: "flex-start" }}>
        <GripVertical size={14} className="faint drag-grip" />
        <div className="grow" style={{ minWidth: 0 }}>
          <div className="row" style={{ gap: 6 }}>
            <span className="profile-name ellipsis">{p.name}</span>
            {current && <CheckCircle2 size={15} color="var(--accent-fg)" style={{ flex: "none" }} />}
          </div>
          <div className="row" style={{ gap: 5, marginTop: 3, flexWrap: "wrap" }}>
            <Badge tone={p.type === "remote" ? "info" : undefined}>{p.type === "remote" ? t("profiles.remote") : t("profiles.local")}</Badge>
            {p.converted && <Badge tip={t("profiles.convertedTip")}>{t("profiles.converted")}</Badge>}
            {dns && <Badge tone="accent">DNS</Badge>}
            {p.proxies > 0 && <span className="faint" style={{ fontSize: 11.5 }}>{t("profiles.nodes", { n: p.proxies })}</span>}
          </div>
        </div>
        <Menu
          trigger={(open) => <Button size="sm" variant="ghost" icon={<MoreHorizontal size={15} />} onClick={open} />}
          items={[
            { label: t("profiles.editInfo"), icon: <Pencil size={14} />, onClick: onEdit },
            { label: t("profiles.editFile"), icon: <FileCode2 size={14} />, onClick: onEditFile },
            { separator: true },
            { label: t("profiles.ext.rules"), icon: <ListOrdered size={14} />, onClick: () => onExt("rules") },
            { label: t("profiles.ext.proxies"), icon: <Server size={14} />, onClick: () => onExt("proxies") },
            { label: t("profiles.ext.groups"), icon: <Network size={14} />, onClick: () => onExt("groups") },
            { label: t("profiles.ext.merge"), icon: <Merge size={14} />, onClick: () => onExt("merge") },
            { label: t("profiles.ext.script"), icon: <Code2 size={14} />, onClick: () => onExt("script") },
            { separator: true },
            ...(p.type === "remote"
              ? [
                  { label: t("profiles.update"), icon: <RefreshCw size={14} />, onClick: () => act("update", () => API.update(p.uid, null), t("profiles.updateFailed"), t("profiles.updated")) },
                  { label: t("profiles.updateViaProxy"), icon: <Waypoints size={14} />, onClick: () => act("update", () => API.update(p.uid, true), t("profiles.updateFailed"), t("profiles.updated")) },
                  { label: t("profiles.copyUrl"), icon: <Globe size={14} />, onClick: () => App.copyText(p.url ?? "").then(() => toast({ level: "success", message: t("common.copied") })) },
                ]
              : []),
            ...(p.home ? [{ label: t("profiles.home"), icon: <ExternalLink size={14} />, onClick: () => App.openURL(p.home!) }] : []),
            {
              label: dns ? t("profiles.dnsOff") : t("profiles.dnsOn"),
              icon: <Network size={14} />,
              onClick: async () => {
                if (!dns && (await API.hasDNS(p.uid)) && !(await confirm({ title: t("profiles.dnsWarnTitle"), message: t("profiles.dnsWarn"), confirm: t("profiles.dnsOn") }))) return;
                void run(() => API.setDNSOverride(p.uid, !dns), t("common.failed"));
              },
            },
            { label: t("profiles.export"), icon: <Download size={14} />, onClick: () => run(() => API.export(p.uid), t("common.failed")) },
            { separator: true },
            {
              label: t("common.delete"),
              icon: <Trash2 size={14} />,
              danger: true,
              onClick: async () => {
                if (await confirm({ title: t("profiles.deleteTitle"), message: t("profiles.deleteMsg", { name: p.name }), confirm: t("common.delete"), danger: true }))
                  void run(() => API.delete(p.uid), t("common.failed"));
              },
            },
          ]}
        />
      </div>
      {p.desc && (
        <div className="muted ellipsis" style={{ fontSize: 12 }}>
          {p.desc}
        </div>
      )}
      {total > 0 && (
        <div className="col" style={{ gap: 4 }}>
          <Progress value={pct} tone={pct > 90 || expired ? "danger" : pct > 75 ? "warning" : undefined} />
          <div className="row" style={{ fontSize: 11.5 }}>
            <span className="tnum muted">
              {bytes(used)} / {bytes(total)}
            </span>
            <span className="spacer" />
            {p.usage?.expire ? (
              <span style={{ color: expired ? "var(--danger)" : "var(--text-muted)" }}>{expired ? t("profiles.expired") : t("profiles.expires", { date: dateOnly(p.usage.expire) })}</span>
            ) : null}
          </div>
        </div>
      )}
      {p.lastError && (
        <div style={{ color: "var(--danger)", fontSize: 11.5 }} className="ellipsis" title={p.lastError}>
          {p.lastError}
        </div>
      )}
      <div className="row profile-foot">
        <span className="faint" style={{ fontSize: 11.5 }}>
          {p.type === "remote" ? relative(p.updated * 1000, lang) : t("profiles.editedAgo", { when: relative(p.updated * 1000, lang) })}
          {next && ` · ${t("profiles.nextShort", { when: relative(next, lang) })}`}
        </span>
        <span className="spacer" />
        {p.type === "remote" && (
          <Button size="sm" variant="ghost" icon={<RefreshCw size={13} />} loading={busy === "update"} tip={t("profiles.update")} onClick={() => act("update", () => API.update(p.uid, null), t("profiles.updateFailed"), t("profiles.updated"))} />
        )}
        {current ? (
          <Badge tone="accent">{t("profiles.inUse")}</Badge>
        ) : (
          <Button size="sm" loading={busy === "activate"} onClick={activate}>
            {t("profiles.use")}
          </Button>
        )}
      </div>
    </div>
  );
}

function RuntimeDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const t = useT();
  const [text, setText] = useState<string | null>(null);
  useEffect(() => {
    if (!open) return;
    API.runtimeYAML()
      .then(setText)
      .catch((e) => {
        toastError(t("profiles.readFailed"), e);
        onClose();
      });
    // Load once per opening.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);
  return (
    <Dialog
      open={open}
      onClose={() => {
        setText(null);
        onClose();
      }}
      title={t("profiles.runtime")}
      size="xwide"
      flush
      footer={
        <Button onClick={() => text && App.copyText(text).then(() => toast({ level: "success", message: t("common.copied") }))}>{t("common.copy")}</Button>
      }
    >
      {text === null ? (
        <div className="empty" style={{ flex: 1 }}>
          <Spinner />
        </div>
      ) : (
        <Suspense fallback={<Spinner />}>
          <CodeEditor value={text} lang="yaml" readOnly />
        </Suspense>
      )}
    </Dialog>
  );
}

function LogsDialog({ open, onClose, names }: { open: boolean; onClose: () => void; names: Record<string, string> }) {
  const t = useT();
  const [logs, setLogs] = useState<Record<string, LogEntry[]> | null>(null);
  useEffect(() => {
    if (open) API.logs().then(setLogs).catch(() => setLogs({}));
  }, [open]);
  const entries = Object.entries(logs ?? {}).filter(([, l]) => l.length > 0);
  return (
    <Dialog
      open={open}
      onClose={() => {
        setLogs(null);
        onClose();
      }}
      title={t("profiles.enhanceLogs")}
      size="wide"
    >
      {entries.length === 0 ? (
        <Empty title={t("profiles.noLogs")} icon={<ScrollText size={22} />} />
      ) : (
        entries.map(([uid, l]) => (
          <div key={uid} className="section">
            <div className="section-title">{names[uid] ?? uid}</div>
            <div className="rows">
              {l.map((e, i) => (
                <div key={i} className="setting" style={{ alignItems: "flex-start", minHeight: 0 }}>
                  <Badge tone={e.level === "exception" || e.level === "error" ? "danger" : e.level === "warn" ? "warning" : "info"}>{e.level}</Badge>
                  <pre className="mono selectable grow" style={{ margin: 0, whiteSpace: "pre-wrap", fontSize: 12 }}>
                    {e.message}
                  </pre>
                </div>
              ))}
            </div>
          </div>
        ))
      )}
    </Dialog>
  );
}

export default function Profiles() {
  const t = useT();
  const view = useApp((s) => s.profiles);
  const configError = useApp((s) => s.state?.configError);
  const [url, setUrl] = useState("");
  const [importing, setImporting] = useState(false);
  const [dialog, setDialog] = useState<{ open: boolean; profile: Profile | null }>({ open: false, profile: null });
  const [editing, setEditing] = useState<Editing>(null);
  const [seq, setSeq] = useState<SeqEditing>(null);
  const [runtime, setRuntime] = useState(false);
  const [logs, setLogs] = useState(false);
  const [updatingAll, setUpdatingAll] = useState(false);
  const [dragged, setDragged] = useState<string | null>(null);
  useNow();
  const items = view?.items ?? [];
  const names = Object.fromEntries(items.map((p) => [p.uid, p.name] as const));
  names.Merge = t("profiles.globalMerge");
  names.Script = t("profiles.globalScript");

  const importURL = async () => {
    const u = url.trim();
    if (!/^https?:\/\//i.test(u)) {
      toast({ level: "warning", message: t("profiles.badUrl") });
      return;
    }
    setImporting(true);
    const p = await run(() => API.create({ type: "remote", url: u, name: "", desc: "", content: "", option: {} }), t("profiles.importFailed"));
    if (p) {
      setUrl("");
      toast({ level: "success", message: t("profiles.imported"), detail: p.name });
    }
    setImporting(false);
  };
  const openExt = async (p: Profile, kind: SeqKind | "merge" | "script") => {
    const uid = await run(() => API.extension(p.uid, kind), t("common.failed"));
    if (!uid) return;
    const title = `${t(`profiles.ext.${kind}`)} · ${p.name}`;
    if (kind === "merge" || kind === "script") setEditing({ uid, title, lang: kind === "script" ? "javascript" : "yaml" });
    else setSeq({ uid, profile: p.uid, kind, title });
  };
  const drop = (target: string) => {
    if (!dragged || dragged === target) return;
    const order = items.map((p) => p.uid).filter((u) => u !== dragged);
    order.splice(order.indexOf(target), 0, dragged);
    void run(() => API.reorder(order), t("common.failed"));
    setDragged(null);
  };
  return (
    <>
      <PageHeader title={t("nav.profiles")}>
        <Button size="sm" variant="ghost" icon={<ScrollText size={14} />} onClick={() => setLogs(true)}>
          {t("profiles.enhanceLogs")}
        </Button>
        <Button size="sm" variant="ghost" icon={<FileStack size={14} />} onClick={() => setRuntime(true)}>
          {t("profiles.runtime")}
        </Button>
        <Button
          size="sm"
          icon={<RefreshCw size={14} />}
          loading={updatingAll}
          onClick={async () => {
            setUpdatingAll(true);
            const failed = await run(() => API.updateAll(), t("profiles.updateFailed"));
            setUpdatingAll(false);
            const n = Object.keys(failed ?? {}).length;
            if (failed) toast(n ? { level: "warning", message: t("profiles.someFailed", { n }) } : { level: "success", message: t("profiles.allUpdated") });
          }}
        >
          {t("profiles.updateAll")}
        </Button>
      </PageHeader>
      <div className="page-body">
        {configError && (
          <div className="banner danger">
            <Info size={16} />
            <span className="grow selectable">{configError}</span>
          </div>
        )}
        <div className="import-bar card">
          <div className="input-group grow" style={{ height: 34 }}>
            <Globe size={15} />
            <input value={url} onChange={(e) => setUrl(e.target.value)} placeholder={t("profiles.urlPlaceholder")} onKeyDown={(e) => e.key === "Enter" && importURL()} spellCheck={false} />
          </div>
          <Button variant="primary" size="lg" loading={importing} onClick={importURL} disabled={!url.trim()}>
            {t("profiles.import")}
          </Button>
          <Button size="lg" icon={<FileUp size={15} />} onClick={() => run(() => API.importFile(), t("profiles.importFailed"))} tip={t("profiles.fromFile")} />
          <Button size="lg" icon={<FilePlus2 size={15} />} onClick={() => setDialog({ open: true, profile: null })} tip={t("profiles.new")} />
        </div>
        {items.length === 0 ? (
          <Empty title={t("profiles.empty")} art>
            {t("profiles.emptyHint")}
          </Empty>
        ) : (
          <div className="grid-cards" ref={reflow} style={{ gridTemplateColumns: "repeat(auto-fill, minmax(290px, 1fr))" }}>
            {items.map((p) => (
              <ProfileCard
                key={p.uid}
                p={p}
                current={p.uid === view?.current}
                next={view?.nextUpdates[p.uid]}
                dns={!!view?.dns[p.uid]}
                onEdit={() => setDialog({ open: true, profile: p })}
                onEditFile={() => setEditing({ uid: p.uid, title: p.name, lang: "yaml" })}
                onExt={(k) => openExt(p, k)}
                onDragStart={() => setDragged(p.uid)}
                onDrop={() => drop(p.uid)}
              />
            ))}
          </div>
        )}
        <div className="section" style={{ marginTop: 22 }}>
          <div className="section-title">{t("profiles.globalExt")}</div>
          <div className="grid-cards" ref={reflow} style={{ gridTemplateColumns: "repeat(auto-fill, minmax(290px, 1fr))" }}>
            {[
              { uid: "Merge", icon: <Merge size={16} />, title: t("profiles.globalMerge"), desc: t("profiles.globalMergeDesc"), lang: "yaml" as const },
              { uid: "Script", icon: <Code2 size={16} />, title: t("profiles.globalScript"), desc: t("profiles.globalScriptDesc"), lang: "javascript" as const },
            ].map((g) => (
              <div key={g.uid} className="card card-pad row" style={{ alignItems: "flex-start" }}>
                <div className="empty-icon" style={{ width: 34, height: 34, borderRadius: 10, display: "grid", placeItems: "center", background: "var(--accent-soft)", color: "var(--accent)" }}>
                  {g.icon}
                </div>
                <div className="grow">
                  <div style={{ fontWeight: 620 }}>{g.title}</div>
                  <div className="muted" style={{ fontSize: 12 }}>
                    {g.desc}
                  </div>
                </div>
                <Button size="sm" icon={<Pencil size={13} />} onClick={() => setEditing({ uid: g.uid, title: g.title, lang: g.lang })}>
                  {t("common.edit")}
                </Button>
              </div>
            ))}
          </div>
        </div>
      </div>
      <ProfileDialog open={dialog.open} profile={dialog.profile} onClose={() => setDialog({ open: false, profile: null })} />
      <EditorDialog uid={editing?.uid ?? null} title={editing?.title ?? ""} lang={editing?.lang ?? "yaml"} readOnly={editing?.readOnly} onClose={() => setEditing(null)} />
      <SeqEditor
        uid={seq?.uid ?? null}
        profileUid={seq?.profile ?? ""}
        kind={seq?.kind ?? "rules"}
        title={seq?.title ?? ""}
        onClose={() => setSeq(null)}
        onRaw={() => {
          if (seq) setEditing({ uid: seq.uid, title: seq.title, lang: "yaml" });
          setSeq(null);
        }}
      />
      <RuntimeDialog open={runtime} onClose={() => setRuntime(false)} />
      <LogsDialog open={logs} onClose={() => setLogs(false)} names={names} />
    </>
  );
}

