import { useEffect, useState } from "react";
import { useT } from "../../lib/i18n";
import { toastError } from "../../lib/store";
import { Profiles, type Option, type Profile } from "../../mygo";
import { Button, Dialog, Field, Input, NumberInput, Segmented, Switch } from "../../ui";

/** ProfileDialog creates a profile, or edits one's properties. */
export function ProfileDialog({ open, onClose, profile }: { open: boolean; onClose: () => void; profile?: Profile | null }) {
  const t = useT();
  const editing = !!profile;
  const [type, setType] = useState<"remote" | "local">("remote");
  const [name, setName] = useState("");
  const [desc, setDesc] = useState("");
  const [url, setUrl] = useState("");
  const [opt, setOpt] = useState<Option>({});
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    if (!open) return;
    setType((profile?.type as "remote" | "local") ?? "remote");
    setName(profile?.name ?? "");
    setDesc(profile?.desc ?? "");
    setUrl(profile?.url ?? "");
    setOpt(profile?.option ?? {});
  }, [open, profile]);
  const set = <K extends keyof Option>(k: K, v: Option[K]) => setOpt((o) => ({ ...o, [k]: v }));
  const validURL = type === "local" || /^https?:\/\/\S+$/i.test(url.trim());
  const save = async () => {
    setBusy(true);
    try {
      if (editing && profile) {
        await Profiles.patch(profile.uid, { name, desc, url: type === "remote" ? url : undefined, option: opt });
      } else {
        await Profiles.create({ type, name, desc, url, content: "", option: opt });
      }
      onClose();
    } catch (e) {
      toastError(editing ? t("profiles.saveFailed") : t("profiles.importFailed"), e);
    }
    setBusy(false);
  };
  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={editing ? t("profiles.editInfo") : t("profiles.new")}
      footer={
        <>
          <Button onClick={onClose}>{t("common.cancel")}</Button>
          <Button variant="primary" loading={busy} disabled={!validURL} onClick={save}>
            {editing ? t("common.save") : type === "remote" ? t("profiles.import") : t("common.create")}
          </Button>
        </>
      }
    >
      <div className="form">
        {!editing && (
          <Segmented
            full
            value={type}
            onChange={setType}
            options={[
              { value: "remote", label: t("profiles.remote") },
              { value: "local", label: t("profiles.local") },
            ]}
          />
        )}
        <div className="form-row">
          <Field label={t("profiles.name")} hint={type === "remote" && !editing ? t("profiles.nameHint") : undefined}>
            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder={type === "remote" ? t("profiles.nameAuto") : ""} />
          </Field>
          <Field label={t("profiles.desc")}>
            <Input value={desc} onChange={(e) => setDesc(e.target.value)} />
          </Field>
        </div>
        {type === "remote" && (
          <>
            <Field label={t("profiles.url")} error={url && !validURL ? t("profiles.badUrl") : undefined}>
              <textarea className="textarea mono" rows={3} value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://" spellCheck={false} />
            </Field>
            <div className="form-row">
              <Field label={t("profiles.userAgent")} hint={t("profiles.userAgentHint")}>
                <Input value={opt.userAgent ?? ""} onChange={(e) => set("userAgent", e.target.value)} placeholder="mihomo/1.19 clash-verge/v2.4 MyGO-Clash" />
              </Field>
              <Field
                label={t("profiles.interval")}
                hint={!opt.updateInterval && profile?.suggestedInterval ? t("profiles.intervalFollow", { n: profile.suggestedInterval }) : t("profiles.intervalHint")}
              >
                <NumberInput value={opt.updateInterval ?? 0} onChange={(v) => set("updateInterval", v)} min={0} max={525600} width={140} disabled={!!opt.noAutoUpdate} />
              </Field>
            </div>
            <div className="form-row">
              <Field label={t("profiles.timeout")}>
                <NumberInput value={opt.timeoutSeconds ?? 60} onChange={(v) => set("timeoutSeconds", v)} min={5} max={600} width={140} />
              </Field>
              <div />
            </div>
            <div className="rows">
              <div className="setting">
                <div className="setting-text">
                  <div className="setting-label">{t("profiles.withProxy")}</div>
                  <div className="setting-desc">{t("profiles.withProxyHint")}</div>
                </div>
                <Switch checked={!!opt.withProxy} onChange={(v) => set("withProxy", v)} />
              </div>
              <div className="setting">
                <div className="setting-text">
                  <div className="setting-label">{t("profiles.autoUpdate")}</div>
                  <div className="setting-desc">{t("profiles.autoUpdateHint")}</div>
                </div>
                <Switch checked={!opt.noAutoUpdate} onChange={(v) => set("noAutoUpdate", !v)} />
              </div>
              <div className="setting">
                <div className="setting-text">
                  <div className="setting-label">{t("profiles.insecure")}</div>
                  <div className="setting-desc" style={{ color: "var(--warning)" }}>
                    {t("profiles.insecureHint")}
                  </div>
                </div>
                <Switch checked={!!opt.insecure} onChange={(v) => set("insecure", v)} />
              </div>
            </div>
          </>
        )}
      </div>
    </Dialog>
  );
}
