import { CircleCheck, CirclePlus, Plus, X } from "lucide-react";
import { useEffect, useState } from "react";
import { Logo } from "./Logo";
import { useT } from "../lib/i18n";
import { toastError } from "../lib/store";
import { type Site, type SitePreset, Tools } from "../mygo";
import { Button, Dialog, Field, Input, Segmented, confirm } from "../ui";

/** The host a site tests, the identity imports dedupe on. */
const hostOf = (url: string) => {
  try {
    return new URL(url).host.toLowerCase();
  } catch {
    return url;
  }
};

type Group = "global" | "domestic";

/**
 * SitesDialog adds sites to the connectivity page: curated preset groups
 * after MyIP's, single custom sites, and the list the page holds now.
 */
export function SitesDialog({
  open,
  onClose,
  sites,
  onSave,
  running,
  onTest,
}: {
  open: boolean;
  onClose: () => void;
  sites: Site[];
  /** saves a list and answers with what the page keeps */
  onSave: (sites: Site[]) => Promise<Site[]>;
  running: boolean;
  /** retests, so that just-added sites answer at once */
  onTest: () => void;
}) {
  const t = useT();
  const [tab, setTab] = useState<"import" | "custom" | "list">("import");
  const [presets, setPresets] = useState<SitePreset[]>();
  const [name, setName] = useState("");
  const [url, setUrl] = useState("");
  const [group, setGroup] = useState<Group>("global");
  const [error, setError] = useState("");
  useEffect(() => {
    if (open && !presets) Tools.sitePresets().then(setPresets).catch(() => setPresets([]));
  }, [open, presets]);
  useEffect(() => {
    if (!open) {
      setTab("import");
      setName("");
      setUrl("");
      setGroup("global");
      setError("");
    }
  }, [open]);

  const hosts = new Set(sites.map((s) => hostOf(s.url)));
  const present = (p: SitePreset) => p.sites.every((s) => hosts.has(hostOf(s.url)));

  const save = async (next: Site[], retest = false) => {
    try {
      await onSave(next);
      if (retest && running) onTest();
      return true;
    } catch (e) {
      toastError(t("common.failed"), e);
      return false;
    }
  };

  const importPreset = async (p: SitePreset) => {
    const next = [...sites];
    for (const s of p.sites) if (!hosts.has(hostOf(s.url))) next.push(s);
    await save(next, true);
  };

  const addCustom = async () => {
    let host: string;
    try {
      const u = new URL(url.trim());
      if ((u.protocol !== "http:" && u.protocol !== "https:") || !u.host) throw new Error("bad url");
      host = u.host.toLowerCase();
    } catch {
      setError(t("connectivity.custom.badUrl"));
      return;
    }
    if (hosts.has(host)) {
      setError(t("connectivity.custom.known"));
      return;
    }
    if (await save([...sites, { id: host, name: name.trim() || host, url: url.trim(), group }], true)) {
      setName("");
      setUrl("");
      setError("");
    }
  };

  const reset = async () => {
    if (await confirm({ title: t("connectivity.list.reset"), message: t("connectivity.list.resetMsg"), danger: true, confirm: t("connectivity.list.reset") })) {
      await save([]);
    }
  };

  return (
    <Dialog open={open} onClose={onClose} title={t("connectivity.manage")} icon={<CirclePlus size={16} />} size="wide">
      <div className="sites-dialog">
        <Segmented
          full
          value={tab}
          onChange={setTab}
          options={[
            { value: "import", label: t("connectivity.tab.import") },
            { value: "custom", label: t("connectivity.tab.custom") },
            { value: "list", label: t("connectivity.tab.list") },
          ]}
        />
        {tab === "import" && (
          <div className="preset-grid">
            {(presets ?? []).map((p) => {
              const full = present(p);
              return (
                <div key={p.id} className="preset">
                  <span className="preset-emoji" aria-hidden>
                    {p.emoji}
                  </span>
                  <span className="col preset-body">
                    <span className="preset-name ellipsis">{t(`connectivity.preset.${p.id}` as never)}</span>
                    <span className="preset-meta">
                      <span className="preset-favi">
                        {p.sites.slice(0, 4).map((s) => (
                          <Logo key={s.id} id={s.id} name={s.name} size={14} />
                        ))}
                      </span>
                      <span>{t("connectivity.importCount", { n: p.sites.length })}</span>
                    </span>
                  </span>
                  <Button
                    size="sm"
                    variant="ghost"
                    icon={full ? <CircleCheck size={15} /> : <Plus size={15} />}
                    disabled={full}
                    onClick={() => importPreset(p)}
                    tip={full ? t("connectivity.imported") : t("common.add")}
                  />
                </div>
              );
            })}
          </div>
        )}
        {tab === "custom" && (
          <div className="form">
            <Field label={t("connectivity.custom.name")}>
              <Input value={name} placeholder={t("connectivity.custom.namePh")} maxLength={20} onChange={(e) => setName(e.target.value)} />
            </Field>
            <Field label={t("connectivity.custom.url")} error={error}>
              <Input
                value={url}
                placeholder={t("connectivity.custom.urlPh")}
                onChange={(e) => setUrl(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && name && url && addCustom()}
              />
            </Field>
            <div className="muted" style={{ fontSize: 12 }}>
              {t("connectivity.custom.hint")}
            </div>
            <div className="row">
              <Segmented
                value={group}
                onChange={setGroup}
                options={[
                  { value: "global", label: t("connectivity.group.global") },
                  { value: "domestic", label: t("connectivity.group.domestic") },
                ]}
              />
              <span className="grow" />
              <Button variant="primary" disabled={!name.trim() || !url.trim()} onClick={addCustom}>
                {t("common.add")}
              </Button>
            </div>
          </div>
        )}
        {tab === "list" && (
          <div>
            {(["global", "domestic"] as const).map((g) => {
              const of = sites.filter((s) => (s.group ?? "global") === g);
              if (!of.length) return null;
              return (
                <div key={g} className="cty-group">
                  <div className="cty-group-name faint">{t(`connectivity.group.${g}`)}</div>
                  <div className="sites-rows">
                    {of.map((s) => (
                      <div key={s.id} className="row sites-row">
                        <Logo id={s.id} name={s.name} size={20} />
                        <span className="grow ellipsis">{s.name}</span>
                        <span className="faint ellipsis sites-host">{hostOf(s.url)}</span>
                        <Button
                          size="sm"
                          variant="ghost"
                          icon={<X size={13} />}
                          onClick={async () => {
                            const next = sites.filter((x) => x.id !== s.id);
                            await save(next);
                          }}
                          tip={t("common.delete")}
                        />
                      </div>
                    ))}
                  </div>
                </div>
              );
            })}
            <div className="sites-reset">
              <Button variant="ghost" onClick={reset}>
                {t("connectivity.list.reset")}
              </Button>
            </div>
          </div>
        )}
      </div>
    </Dialog>
  );
}
