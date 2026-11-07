import { Layers2, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { useT } from "../lib/i18n";
import { patchSettings, run, useApp } from "../lib/store";
import type { RuleSet } from "../mygo";
import { Badge, Button, Dialog, Input, Select, Switch } from "../ui";

/** A preset of the catalog: a rule set the maintainers vouched for, with
 * the policy most people want from it. */
type Preset = { id: string; key: string; url: string; behavior: string; format: string; policy: string };

const PRESETS: Preset[] = [
  { id: "ads", key: "rulesets.pAds", url: "https://fastly.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@meta/geo/geosite/category-ads-all.mrs", behavior: "domain", format: "mrs", policy: "REJECT" },
  { id: "openai", key: "rulesets.pOpenai", url: "https://fastly.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@meta/geo/geosite/openai.mrs", behavior: "domain", format: "mrs", policy: "PROXY" },
  { id: "telegram", key: "rulesets.pTelegram", url: "https://fastly.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@meta/geo/geosite/telegram.mrs", behavior: "domain", format: "mrs", policy: "PROXY" },
  { id: "cn", key: "rulesets.pCn", url: "https://fastly.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@meta/geo/geosite/cn.mrs", behavior: "domain", format: "mrs", policy: "DIRECT" },
  { id: "microsoft", key: "rulesets.pMicrosoft", url: "https://fastly.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@meta/geo/geosite/microsoft.mrs", behavior: "domain", format: "mrs", policy: "DIRECT" },
  { id: "apple", key: "rulesets.pApple", url: "https://fastly.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@meta/geo/geosite/apple.mrs", behavior: "domain", format: "mrs", policy: "DIRECT" },
];

const BEHAVIORS = ["domain", "ipcidr", "classical"];
const FORMATS = ["mrs", "yaml", "text"];
const newID = () => Date.now().toString(36) + Math.random().toString(36).slice(2, 6);

/** RuleSetsDialog manages the rule sets the user added: the catalog's
 * presets, a set of their own, and the ones in place. */
export function RuleSetsDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const t = useT();
  const s = useApp((st) => st.settings!);
  const sets = s.ruleSets ?? [];
  const save = (next: RuleSet[]) => run(() => patchSettings({ ruleSets: next }), t("common.failed"));
  const [custom, setCustom] = useState(false);
  const [name, setName] = useState("");
  const [url, setUrl] = useState("");
  const [behavior, setBehavior] = useState("domain");
  const [format, setFormat] = useState("mrs");
  const [policy, setPolicy] = useState("PROXY");

  const addPreset = (p: Preset) => {
    if (sets.some((x) => x.url === p.url)) return;
    void save([...sets, { id: newID(), name: t(p.key as never), url: p.url, behavior: p.behavior, format: p.format, policy: p.policy, enabled: true }]);
  };
  const addCustom = () => {
    const u = url.trim();
    if (!/^https?:\/\//.test(u)) return;
    void save([...sets, { id: newID(), name: name.trim() || u.split("/").pop() || u, url: u, behavior, format, policy, enabled: true }]);
    setCustom(false);
    setName("");
    setUrl("");
  };
  const update = (id: string, fn: (x: RuleSet) => RuleSet) => save(sets.map((x) => (x.id === id ? fn(x) : x)));
  const host = (u: string) => {
    try {
      return new URL(u).host;
    } catch {
      return u;
    }
  };
  const policyLabel = (p: string) => (p === "PROXY" ? t("rulesets.policyProxy") : p);

  return (
    <Dialog open={open} onClose={onClose} title={t("rulesets.title")} icon={<Layers2 size={16} />} size="wide">
      <p className="rules-intro muted">{t("rulesets.hint")}</p>
      {sets.length > 0 && (
        <div className="col" style={{ gap: 8, marginBottom: 14 }}>
          {sets.map((x) => (
            <div key={x.id} className="row" style={{ padding: "8px 10px", border: "1px solid var(--border)", borderRadius: 8, gap: 10 }}>
              <Switch checked={x.enabled} onChange={(v) => update(x.id, (y) => ({ ...y, enabled: v }))} />
              <div className="grow" style={{ minWidth: 0 }}>
                <div className="row" style={{ gap: 7 }}>
                  <span style={{ fontWeight: 600 }} className="ellipsis">{x.name}</span>
                  <Badge>{x.behavior}</Badge>
                  <span className="faint ellipsis">{host(x.url)}</span>
                </div>
              </div>
              <Select
                value={x.policy}
                onChange={(v) => update(x.id, (y) => ({ ...y, policy: v }))}
                options={[
                  { value: "DIRECT", label: "DIRECT" },
                  { value: "REJECT", label: "REJECT" },
                  { value: "PROXY", label: t("rulesets.policyProxy") },
                ]}
                width={150}
              />
              <Button size="sm" variant="ghost" icon={<Trash2 size={14} />} onClick={() => save(sets.filter((y) => y.id !== x.id))} tip={t("common.delete")} />
            </div>
          ))}
        </div>
      )}

      <div className="muted" style={{ fontSize: 12, marginBottom: 6 }}>{t("rulesets.preset")}</div>
      <div className="row" style={{ gap: 8, flexWrap: "wrap", marginBottom: 14 }}>
        {PRESETS.map((p) => (
          <Button key={p.id} size="sm" variant={sets.some((x) => x.url === p.url) ? "primary" : "ghost"} onClick={() => addPreset(p)}>
            {t(p.key as never)}
          </Button>
        ))}
      </div>

      {custom ? (
        <div className="col" style={{ gap: 8 }}>
          <div className="row" style={{ gap: 8 }}>
            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder={t("rulesets.name")} width={150} />
            <Input value={url} onChange={(e) => setUrl(e.target.value)} placeholder={t("rulesets.url")} style={{ flex: 1 }} spellCheck={false} />
          </div>
          <div className="row" style={{ gap: 8 }}>
            <Select value={behavior} onChange={setBehavior} options={BEHAVIORS.map((b) => ({ value: b, label: b }))} width={130} />
            <Select value={format} onChange={setFormat} options={FORMATS.map((f) => ({ value: f, label: f }))} width={110} />
            <Select
              value={policy}
              onChange={setPolicy}
              options={[
                { value: "PROXY", label: t("rulesets.policyProxy") },
                { value: "DIRECT", label: "DIRECT" },
                { value: "REJECT", label: "REJECT" },
              ]}
              width={180}
            />
            <Button variant="primary" icon={<Plus size={14} />} disabled={!/^https?:\/\//.test(url.trim())} onClick={addCustom}>
              {t("common.add")}
            </Button>
            <Button variant="ghost" onClick={() => setCustom(false)}>
              {t("common.cancel")}
            </Button>
          </div>
        </div>
      ) : (
        <Button size="sm" variant="ghost" icon={<Plus size={14} />} onClick={() => setCustom(true)}>
          {t("rulesets.custom")}
        </Button>
      )}
    </Dialog>
  );
}
