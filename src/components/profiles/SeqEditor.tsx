import { ArrowDown, ArrowUp, Code2, Plus, Trash2 } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useT } from "../../lib/i18n";
import { toast, toastError } from "../../lib/store";
import { Profiles, type NameLists, type SeqPatch } from "../../mygo";
import { Badge, Button, Dialog, Empty, Field, Input, SearchInput, Select, Spinner, Tabs } from "../../ui";

export type SeqKind = "rules" | "proxies" | "groups";

const RULE_TYPES = [
  "DOMAIN",
  "DOMAIN-SUFFIX",
  "DOMAIN-KEYWORD",
  "DOMAIN-REGEX",
  "DOMAIN-WILDCARD",
  "GEOSITE",
  "IP-CIDR",
  "IP-CIDR6",
  "IP-SUFFIX",
  "IP-ASN",
  "GEOIP",
  "SRC-GEOIP",
  "SRC-IP-CIDR",
  "SRC-PORT",
  "DST-PORT",
  "IN-PORT",
  "IN-TYPE",
  "NETWORK",
  "PROCESS-NAME",
  "PROCESS-PATH",
  "PROCESS-NAME-REGEX",
  "UID",
  "DSCP",
  "RULE-SET",
  "MATCH",
];
const NO_RESOLVE = new Set(["IP-CIDR", "IP-CIDR6", "IP-SUFFIX", "IP-ASN", "GEOIP", "RULE-SET"]);
const GROUP_TYPES = ["select", "url-test", "fallback", "load-balance", "relay"];

type Item = unknown;
type Tab = "prepend" | "append" | "delete";

function label(kind: SeqKind, it: Item): string {
  if (typeof it === "string") return it;
  if (it && typeof it === "object") {
    const o = it as Record<string, unknown>;
    return String(o.name ?? JSON.stringify(o));
  }
  return String(it);
}

function sub(it: Item): string {
  if (it && typeof it === "object") {
    const o = it as Record<string, unknown>;
    const parts = [o.type, o.server && `${o.server}:${o.port ?? ""}`, Array.isArray(o.proxies) && `${(o.proxies as unknown[]).length} members`].filter(Boolean);
    return parts.join(" · ");
  }
  return "";
}

function RuleForm({ names, onAdd }: { names: NameLists; onAdd: (rule: string) => void }) {
  const t = useT();
  const [type, setType] = useState("DOMAIN-SUFFIX");
  const [payload, setPayload] = useState("");
  const [target, setTarget] = useState("DIRECT");
  const [noResolve, setNoResolve] = useState(false);
  const targets = [...names.groups, ...names.builtin, ...names.proxies];
  const add = () => {
    const parts = type === "MATCH" ? [type, target] : [type, payload.trim(), target];
    if (noResolve && NO_RESOLVE.has(type)) parts.push("no-resolve");
    onAdd(parts.join(","));
    setPayload("");
  };
  return (
    <div className="row" style={{ flexWrap: "wrap", gap: 8, padding: 12, background: "var(--surface-2)", borderRadius: 10, border: "1px solid var(--border)" }}>
      <Select value={type} onChange={setType} options={RULE_TYPES.map((r) => ({ value: r, label: r }))} width={170} />
      {type !== "MATCH" && (
        <Input
          value={payload}
          onChange={(e) => setPayload(e.target.value)}
          placeholder={t("editor.payload")}
          style={{ flex: 1, minWidth: 160 }}
          onKeyDown={(e) => e.key === "Enter" && payload.trim() && add()}
        />
      )}
      <Select value={target} onChange={setTarget} options={targets.map((n) => ({ value: n, label: n }))} width={170} />
      {NO_RESOLVE.has(type) && (
        <label className="row" style={{ gap: 5, fontSize: 12 }}>
          <input type="checkbox" checked={noResolve} onChange={(e) => setNoResolve(e.target.checked)} /> no-resolve
        </label>
      )}
      <Button variant="primary" icon={<Plus size={14} />} onClick={add} disabled={type !== "MATCH" && !payload.trim()}>
        {t("common.add")}
      </Button>
    </div>
  );
}

function ProxyForm({ onAdd }: { onAdd: (items: Item[]) => void }) {
  const t = useT();
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  return (
    <div className="col" style={{ padding: 12, background: "var(--surface-2)", borderRadius: 10, border: "1px solid var(--border)" }}>
      <textarea className="textarea mono" rows={4} value={text} onChange={(e) => setText(e.target.value)} placeholder={t("editor.linksHint")} spellCheck={false} />
      <Button
        variant="primary"
        icon={<Plus size={14} />}
        loading={busy}
        disabled={!text.trim()}
        style={{ alignSelf: "flex-end" }}
        onClick={async () => {
          setBusy(true);
          try {
            const items = await Profiles.parseLinks(text);
            onAdd(items);
            setText("");
            toast({ level: "success", message: t("editor.parsed", { n: items.length }) });
          } catch (e) {
            toastError(t("editor.parseFailed"), e);
          }
          setBusy(false);
        }}
      >
        {t("editor.addLinks")}
      </Button>
    </div>
  );
}

function GroupForm({ names, onAdd }: { names: NameLists; onAdd: (g: Item) => void }) {
  const t = useT();
  const [name, setName] = useState("");
  const [type, setType] = useState("select");
  const [members, setMembers] = useState<string[]>([]);
  const [url, setUrl] = useState("https://www.gstatic.com/generate_204");
  const [filter, setFilter] = useState("");
  const all = [...names.builtin.slice(0, 2), ...names.groups, ...names.proxies].filter((n) => n.toLowerCase().includes(filter.toLowerCase()));
  const tests = type === "url-test" || type === "fallback" || type === "load-balance";
  const add = () => {
    const g: Record<string, unknown> = { name: name.trim(), type, proxies: members };
    if (tests) {
      g.url = url;
      g.interval = 300;
    }
    onAdd(g);
    setName("");
    setMembers([]);
  };
  return (
    <div className="col" style={{ padding: 12, background: "var(--surface-2)", borderRadius: 10, border: "1px solid var(--border)", gap: 10 }}>
      <div className="row">
        <Input value={name} onChange={(e) => setName(e.target.value)} placeholder={t("editor.groupName")} style={{ flex: 1 }} />
        <Select value={type} onChange={setType} options={GROUP_TYPES.map((g) => ({ value: g, label: g }))} width={150} />
      </div>
      {tests && <Input value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://www.gstatic.com/generate_204" />}
      <SearchInput value={filter} onChange={setFilter} placeholder={t("editor.members")} width={260} />
      <div className="row" style={{ flexWrap: "wrap", gap: 6, maxHeight: 140, overflow: "auto" }}>
        {all.map((n) => (
          <button key={n} className={`chip${members.includes(n) ? " active" : ""}`} onClick={() => setMembers(members.includes(n) ? members.filter((m) => m !== n) : [...members, n])}>
            {n}
          </button>
        ))}
      </div>
      <Button variant="primary" icon={<Plus size={14} />} disabled={!name.trim() || members.length === 0} onClick={add} style={{ alignSelf: "flex-end" }}>
        {t("editor.addGroup", { n: members.length })}
      </Button>
    </div>
  );
}

/** SeqEditor edits a rules, proxies or groups extension visually. */
export function SeqEditor({
  uid,
  profileUid,
  kind,
  title,
  onClose,
  onRaw,
}: {
  uid: string | null;
  profileUid: string;
  kind: SeqKind;
  title: string;
  onClose: () => void;
  onRaw: () => void;
}) {
  const t = useT();
  const [patch, setPatch] = useState<SeqPatch | null>(null);
  const [names, setNames] = useState<NameLists>({ proxies: [], groups: [], builtin: [] });
  const [own, setOwn] = useState<string[]>([]);
  const [tab, setTab] = useState<Tab>("prepend");
  const [filter, setFilter] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    if (!uid) return;
    setPatch(null);
    setTab("prepend");
    Promise.all([Profiles.seq(uid), Profiles.names(profileUid), kind === "rules" ? Profiles.rulesOf(profileUid) : Promise.resolve([])])
      .then(([p, n, r]) => {
        setPatch(p);
        setNames(n);
        setOwn(kind === "rules" ? r : kind === "proxies" ? n.proxies : n.groups);
      })
      .catch((e) => toastError(t("profiles.readFailed"), e));
  }, [uid, profileUid, kind, t]);
  const list = patch ? (tab === "delete" ? patch.delete : patch[tab]) : [];
  const update = (fn: (p: SeqPatch) => SeqPatch) => setPatch((p) => (p ? fn(p) : p));
  const addItems = (items: Item[]) => update((p) => (tab === "append" ? { ...p, append: [...p.append, ...items] } : { ...p, prepend: [...p.prepend, ...items] }));
  const move = (i: number, d: number) =>
    update((p) => {
      if (tab === "delete") return p;
      const arr = [...p[tab]];
      const j = i + d;
      if (j < 0 || j >= arr.length) return p;
      [arr[i], arr[j]] = [arr[j], arr[i]];
      return { ...p, [tab]: arr };
    });
  const remove = (i: number) => update((p) => (tab === "delete" ? { ...p, delete: p.delete.filter((_, k) => k !== i) } : { ...p, [tab]: p[tab].filter((_, k) => k !== i) }));
  const deletable = useMemo(() => own.filter((n) => n.toLowerCase().includes(filter.toLowerCase())), [own, filter]);
  const save = async () => {
    if (!uid || !patch) return;
    setBusy(true);
    try {
      await Profiles.saveSeq(uid, patch);
      toast({ level: "success", message: t("profiles.saved") });
      onClose();
    } catch (e) {
      toastError(t("profiles.saveFailed"), e);
    }
    setBusy(false);
  };
  return (
    <Dialog
      open={!!uid}
      onClose={onClose}
      title={title}
      size="wide"
      footer={
        <>
          <Button variant="ghost" icon={<Code2 size={14} />} onClick={onRaw} style={{ marginInlineEnd: "auto" }}>
            {t("editor.editYaml")}
          </Button>
          <Button onClick={onClose}>{t("common.cancel")}</Button>
          <Button variant="primary" loading={busy} onClick={save} disabled={!patch}>
            {t("common.save")}
          </Button>
        </>
      }
    >
      {!patch ? (
        <div className="empty">
          <Spinner />
        </div>
      ) : (
        <div className="col" style={{ gap: 12 }}>
          <Tabs
            value={tab}
            onChange={setTab}
            tabs={[
              { value: "prepend", label: `${t("editor.prepend")} (${patch.prepend.length})` },
              { value: "append", label: `${t("editor.append")} (${patch.append.length})` },
              { value: "delete", label: `${t("editor.delete")} (${patch.delete.length})` },
            ]}
          />
          <div className="muted" style={{ fontSize: 12, marginTop: -6 }}>
            {t(`editor.${tab}Hint` as never)}
          </div>
          {tab !== "delete" &&
            (kind === "rules" ? <RuleForm names={names} onAdd={(r) => addItems([r])} /> : kind === "proxies" ? <ProxyForm onAdd={addItems} /> : <GroupForm names={names} onAdd={(g) => addItems([g])} />)}
          {tab === "delete" && (
            <div className="col" style={{ gap: 8 }}>
              <SearchInput value={filter} onChange={setFilter} placeholder={t("common.search")} />
              <div style={{ maxHeight: 220, overflow: "auto", border: "1px solid var(--border)", borderRadius: 10 }}>
                {deletable.length === 0 && <div className="muted" style={{ padding: 12 }}>{t("editor.nothing")}</div>}
                {deletable.map((n) => {
                  const on = patch.delete.includes(n);
                  return (
                    <label key={n} className="row table-row" style={{ gridTemplateColumns: "auto 1fr", display: "flex" }}>
                      <input type="checkbox" checked={on} onChange={() => update((p) => ({ ...p, delete: on ? p.delete.filter((x) => x !== n) : [...p.delete, n] }))} />
                      <span className="ellipsis mono" style={{ fontSize: 12 }}>
                        {n}
                      </span>
                    </label>
                  );
                })}
              </div>
            </div>
          )}
          <div>
            {list.length === 0 ? (
              <Empty title={t("editor.empty")} />
            ) : (
              <div style={{ border: "1px solid var(--border)", borderRadius: 10, overflow: "hidden" }}>
                {list.map((it, i) => (
                  <div key={i} className="row table-row" style={{ display: "flex", height: "auto", minHeight: 36, padding: "6px 10px" }}>
                    <Badge>{i + 1}</Badge>
                    <div className="grow">
                      <div className="ellipsis mono" style={{ fontSize: 12 }}>
                        {label(kind, it)}
                      </div>
                      {sub(it) && <div className="faint" style={{ fontSize: 11 }}>{sub(it)}</div>}
                    </div>
                    {tab !== "delete" && (
                      <>
                        <Button size="sm" variant="ghost" icon={<ArrowUp size={13} />} onClick={() => move(i, -1)} disabled={i === 0} />
                        <Button size="sm" variant="ghost" icon={<ArrowDown size={13} />} onClick={() => move(i, 1)} disabled={i === list.length - 1} />
                      </>
                    )}
                    <Button size="sm" variant="ghost" icon={<Trash2 size={13} />} onClick={() => remove(i)} />
                  </div>
                ))}
              </div>
            )}
          </div>
          {kind === "rules" && tab !== "delete" && (
            <Field label="" hint={t("editor.ruleOrder")}>
              <span />
            </Field>
          )}
        </div>
      )}
    </Dialog>
  );
}
