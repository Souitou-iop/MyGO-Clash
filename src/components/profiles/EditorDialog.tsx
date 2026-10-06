import { lazy, Suspense, useEffect, useState } from "react";
import { useT } from "../../lib/i18n";
import { toast, toastError } from "../../lib/store";
import { Profiles, type LogEntry } from "../../mygo";
import { Badge, Button, confirm, Dialog, Spinner } from "../../ui";

const CodeEditor = lazy(() => import("../CodeEditor"));

/** EditorDialog edits what an item holds as text: a profile's YAML, an
 * extension's YAML or JavaScript. */
export function EditorDialog({ uid, title, lang, onClose, readOnly }: { uid: string | null; title: string; lang: "yaml" | "javascript"; onClose: () => void; readOnly?: boolean }) {
  const t = useT();
  const [text, setText] = useState<string | null>(null);
  const [orig, setOrig] = useState("");
  const [busy, setBusy] = useState(false);
  const [logs, setLogs] = useState<LogEntry[]>([]);
  useEffect(() => {
    if (!uid) return;
    setText(null);
    Profiles.content(uid)
      .then((c) => {
        setText(c);
        setOrig(c);
      })
      .catch((e) => toastError(t("profiles.readFailed"), e));
    Profiles.logs()
      .then((l) => setLogs(l[uid] ?? []))
      .catch(() => setLogs([]));
  }, [uid, t]);
  const dirty = text !== null && text !== orig;
  const save = async () => {
    if (!uid || text === null || !dirty) return;
    setBusy(true);
    try {
      await Profiles.saveContent(uid, text);
      setOrig(text);
      toast({ level: "success", message: t("profiles.saved") });
      Profiles.logs()
        .then((l) => setLogs(l[uid] ?? []))
        .catch(() => {});
    } catch (e) {
      toastError(t("profiles.saveFailed"), e);
    }
    setBusy(false);
  };
  const close = async () => {
    if (dirty && !(await confirm({ title: t("profiles.discardTitle"), message: t("profiles.discard"), confirm: t("profiles.discardOk"), danger: true }))) return;
    onClose();
  };
  return (
    <Dialog
      open={!!uid}
      onClose={close}
      title={title}
      size="xwide"
      flush
      footer={
        <>
          <span className="muted" style={{ fontSize: 12, marginRight: "auto" }}>
            {readOnly ? t("profiles.readOnly") : dirty ? t("profiles.unsaved") : `${navigator.platform.includes("Mac") ? "⌘" : "Ctrl"}+S ${t("common.save")}`}
          </span>
          <Button onClick={close}>{t("common.close")}</Button>
          {!readOnly && (
            <Button variant="primary" loading={busy} disabled={!dirty} onClick={save}>
              {t("common.save")}
            </Button>
          )}
        </>
      }
    >
      {text === null ? (
        <div className="empty" style={{ flex: 1 }}>
          <Spinner />
        </div>
      ) : (
        <Suspense
          fallback={
            <div className="empty" style={{ flex: 1 }}>
              <Spinner />
            </div>
          }
        >
          <CodeEditor value={text} onChange={setText} lang={lang} onSave={save} readOnly={readOnly} />
        </Suspense>
      )}
      {logs.length > 0 && (
        <div style={{ maxHeight: 150, overflow: "auto", padding: "8px 14px", borderTop: "1px solid var(--border)" }} className="col">
          {logs.map((l, i) => (
            <div key={i} className="row" style={{ alignItems: "flex-start", fontSize: 12 }}>
              <Badge tone={l.level === "exception" || l.level === "error" ? "danger" : l.level === "warn" ? "warning" : "info"}>{l.level}</Badge>
              <pre className="mono selectable" style={{ margin: 0, whiteSpace: "pre-wrap" }}>
                {l.message}
              </pre>
            </div>
          ))}
        </div>
      )}
    </Dialog>
  );
}
