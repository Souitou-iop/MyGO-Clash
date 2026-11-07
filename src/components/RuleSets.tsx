import { Layers2, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { useT } from "../lib/i18n";
import { patchSettings, run, useApp } from "../lib/store";
import type { RuleSet } from "../mygo";
import { Badge, Button, Dialog, Input, Select, Switch } from "../ui";

/** A preset of the catalog: a rule set the maintainers vouched for — its
 * URL checked to answer — with the policy most people want from it. */
type Preset = { id: string; nameKey?: string; name?: string; url: string; behavior: string; format: string; policy: string };

/** A group of the catalog, shown under its own heading. */
type Group = { key: string; items: Preset[] };

const RULES_DAT = "https://fastly.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@meta/geo/geosite/";
const LOYAL = "https://fastly.jsdelivr.net/gh/Loyalsoldier/clash-rules@release/";
const BM7 = "https://fastly.jsdelivr.net/gh/blackmatrix7/ios_rule_script@master/rule/Clash/";

const CATALOG: Group[] = [
  {
    key: "rulesets.catAds",
    items: [
      { id: "ads", nameKey: "rulesets.pAds", url: RULES_DAT + "category-ads-all.mrs", behavior: "domain", format: "mrs", policy: "REJECT" },
      { id: "ads-loyal", nameKey: "rulesets.pAdsL", url: LOYAL + "reject.txt", behavior: "domain", format: "yaml", policy: "REJECT" },
      { id: "ads-bm7", nameKey: "rulesets.pAdsB", url: BM7 + "Advertising/Advertising.yaml", behavior: "classical", format: "yaml", policy: "REJECT" },
    ],
  },
  {
    key: "rulesets.catAI",
    items: [
      { id: "openai", nameKey: "rulesets.pOpenai", url: RULES_DAT + "openai.mrs", behavior: "domain", format: "mrs", policy: "PROXY" },
      { id: "anthropic", name: "Anthropic · Claude", url: RULES_DAT + "anthropic.mrs", behavior: "domain", format: "mrs", policy: "PROXY" },
    ],
  },
  {
    key: "rulesets.catStream",
    items: [
      { id: "netflix", name: "Netflix", url: RULES_DAT + "netflix.mrs", behavior: "domain", format: "mrs", policy: "PROXY" },
      { id: "disney", name: "Disney", url: RULES_DAT + "disney.mrs", behavior: "domain", format: "mrs", policy: "PROXY" },
      { id: "youtube", name: "YouTube", url: RULES_DAT + "youtube.mrs", behavior: "domain", format: "mrs", policy: "PROXY" },
      { id: "spotify", name: "Spotify", url: RULES_DAT + "spotify.mrs", behavior: "domain", format: "mrs", policy: "PROXY" },
    ],
  },
  {
    key: "rulesets.catCN",
    items: [
      { id: "cn", nameKey: "rulesets.pCn", url: RULES_DAT + "cn.mrs", behavior: "domain", format: "mrs", policy: "DIRECT" },
      { id: "outbound", nameKey: "rulesets.pOutbound", url: RULES_DAT + "geolocation-!cn.mrs", behavior: "domain", format: "mrs", policy: "PROXY" },
      { id: "proxy-loyal", nameKey: "rulesets.pProxyList", url: LOYAL + "proxy.txt", behavior: "domain", format: "yaml", policy: "PROXY" },
      { id: "gfw", nameKey: "rulesets.pGfw", url: LOYAL + "gfw.txt", behavior: "domain", format: "yaml", policy: "PROXY" },
      { id: "direct-loyal", nameKey: "rulesets.pDirectL", url: LOYAL + "direct.txt", behavior: "domain", format: "yaml", policy: "DIRECT" },
    ],
  },
  {
    key: "rulesets.catPlatform",
    items: [
      { id: "telegram", nameKey: "rulesets.pTelegram", url: RULES_DAT + "telegram.mrs", behavior: "domain", format: "mrs", policy: "PROXY" },
      { id: "telegram-ip", nameKey: "rulesets.pTelegramIP", url: LOYAL + "telegramcidr.txt", behavior: "ipcidr", format: "yaml", policy: "PROXY" },
      { id: "twitter", name: "Twitter / X", url: RULES_DAT + "twitter.mrs", behavior: "domain", format: "mrs", policy: "PROXY" },
      { id: "facebook", name: "Facebook / Meta", url: RULES_DAT + "facebook.mrs", behavior: "domain", format: "mrs", policy: "PROXY" },
      { id: "github", name: "GitHub", url: RULES_DAT + "github.mrs", behavior: "domain", format: "mrs", policy: "PROXY" },
      { id: "google", name: "Google", url: RULES_DAT + "google.mrs", behavior: "domain", format: "mrs", policy: "PROXY" },
      { id: "microsoft", nameKey: "rulesets.pMicrosoft", url: RULES_DAT + "microsoft.mrs", behavior: "domain", format: "mrs", policy: "DIRECT" },
      { id: "apple", nameKey: "rulesets.pApple", url: RULES_DAT + "apple.mrs", behavior: "domain", format: "mrs", policy: "DIRECT" },
    ],
  },
  {
    key: "rulesets.catMisc",
    items: [
      { id: "steam", name: "Steam", url: RULES_DAT + "steam.mrs", behavior: "domain", format: "mrs", policy: "PROXY" },
      { id: "bilibili", name: "Bilibili", url: RULES_DAT + "bilibili.mrs", behavior: "domain", format: "mrs", policy: "DIRECT" },
      { id: "apps", nameKey: "rulesets.pApps", url: LOYAL + "applications.txt", behavior: "classical", format: "yaml", policy: "DIRECT" },
    ],
  },
];

const BEHAVIORS = ["domain", "ipcidr", "classical"];
const FORMATS = ["mrs", "yaml", "text"];
const newID = () => Date.now().toString(36) + Math.random().toString(36).slice(2, 6);

/** RuleSetsDialog manages the rule sets the user added: the catalog's
 * presets by category, a set of their own, and the ones in place. */
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

  const add = (p: Preset) => {
    if (sets.some((x) => x.url === p.url)) return;
    void save([...sets, { id: newID(), name: p.nameKey ? t(p.nameKey as never) : p.name ?? p.id, url: p.url, behavior: p.behavior, format: p.format, policy: p.policy, enabled: true }]);
  };
  const added = (p: Preset) => sets.some((x) => x.url === p.url);
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
  const presetName = (p: Preset) => (p.nameKey ? t(p.nameKey as never) : p.name ?? p.id);

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

      <div style={{ maxHeight: 320, overflowY: "auto", display: "flex", flexDirection: "column", gap: 12, paddingRight: 4 }}>
        {CATALOG.map((g) => (
          <div key={g.key}>
            <div className="muted" style={{ fontSize: 12, margin: "4px 0 6px", fontWeight: 600 }}>{t(g.key as never)}</div>
            <div className="col" style={{ gap: 4 }}>
              {g.items.map((p) => {
                const inPlace = added(p);
                return (
                  <div key={p.id} className="row" style={{ gap: 8, padding: "5px 8px", borderRadius: 8, alignItems: "center" }}>
                    <span className="grow ellipsis" style={{ fontSize: 13 }}>{presetName(p)}</span>
                    <span className="faint" style={{ fontSize: 11 }}>{policyLabel(p.policy)}</span>
                    {inPlace ? (
                      <Badge>{t("rulesets.inPlace")}</Badge>
                    ) : (
                      <Button size="sm" variant="ghost" icon={<Plus size={13} />} onClick={() => add(p)}>
                        {t("common.add")}
                      </Button>
                    )}
                  </div>
                );
              })}
            </div>
          </div>
        ))}
      </div>

      {custom ? (
        <div className="col" style={{ gap: 8, marginTop: 12 }}>
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
        <Button size="sm" variant="ghost" icon={<Plus size={14} />} onClick={() => setCustom(true)} style={{ marginTop: 12 }}>
          {t("rulesets.custom")}
        </Button>
      )}
    </Dialog>
  );
}
