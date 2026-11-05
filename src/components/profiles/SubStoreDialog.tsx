import { Boxes, Check, Layers, Plus } from "lucide-react";
import { useEffect, useState } from "react";
import { useT } from "../../lib/i18n";
import { run, toast, useApp } from "../../lib/store";
import { Profiles as API, type SubStoreItem } from "../../mygo";
import { Button, Dialog, Empty, Input } from "../../ui";

/** SubStoreDialog lists the subscriptions and collections of a Sub-Store backend, to add them as profiles. */
export function SubStoreDialog({ open, onClose, known }: { open: boolean; onClose: () => void; known: Set<string> }) {
  const t = useT();
  const saved = useApp((s) => s.settings?.subscriptions.subStore ?? "");
  const [base, setBase] = useState(saved);
  const [items, setItems] = useState<SubStoreItem[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [adding, setAdding] = useState("");
  const load = async (addr = base) => {
    if (!addr.trim()) return;
    setLoading(true);
    const list = await run(() => API.subStore(addr), t("substore.failed"));
    setLoading(false);
    if (list) setItems(list);
  };
  useEffect(() => {
    if (!open) return;
    setBase(saved);
    setItems(null);
    if (saved) void load(saved);
  }, [open]);
  const add = async (it: SubStoreItem) => {
    setAdding(it.url);
    const p = await run(() => API.create({ type: "remote", url: it.url, name: it.displayName || it.name, desc: "Sub-Store", content: "", option: {} }), t("profiles.importFailed"));
    setAdding("");
    if (p) toast({ level: "success", message: t("profiles.imported"), detail: p.name });
  };
  return (
    <Dialog open={open} onClose={onClose} title="Sub-Store" icon={<Boxes size={16} />} footer={<Button onClick={onClose}>{t("common.close")}</Button>}>
      <div className="form">
        <div className="row" style={{ gap: 8 }}>
          <Input
            className="mono grow"
            value={base}
            placeholder="http://127.0.0.1:3000"
            spellCheck={false}
            onChange={(e) => setBase(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && load()}
          />
          <Button variant="primary" loading={loading} disabled={!base.trim()} onClick={() => load()}>
            {t("substore.connect")}
          </Button>
        </div>
        {items === null ? (
          <div className="muted" style={{ fontSize: 12.5 }}>
            {t("substore.hint")}
          </div>
        ) : items.length === 0 ? (
          <Empty title={t("substore.empty")} icon={<Boxes size={22} />} />
        ) : (
          <div className="substore-list">
            {items.map((it) => {
              const have = known.has(it.url);
              return (
                <div key={it.url} className="setting">
                  {it.icon ? <img className="substore-icon" src={it.icon} alt="" /> : it.kind === "collection" ? <Layers size={16} className="muted" /> : <Boxes size={16} className="muted" />}
                  <div className="setting-text">
                    <div className="setting-label">{it.displayName || it.name}</div>
                    <div className="setting-desc">
                      {it.kind === "collection" ? t("substore.collection") : t("substore.sub")}
                      {it.remark && ` · ${it.remark}`}
                    </div>
                  </div>
                  <Button size="sm" icon={have ? <Check size={13} /> : <Plus size={13} />} disabled={have} loading={adding === it.url} onClick={() => add(it)}>
                    {have ? t("substore.added") : t("common.add")}
                  </Button>
                </div>
              );
            })}
          </div>
        )}
      </div>
    </Dialog>
  );
}
