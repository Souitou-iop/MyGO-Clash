import { ChevronRight, Crosshair, Info } from "lucide-react";
import { Fragment, useState } from "react";
import { errorText, ruleType } from "../lib/format";
import { useT } from "../lib/i18n";
import { type MatchResult, Rules } from "../mygo";
import { Badge, Banner, Button, Dialog, Input, Segmented } from "../ui";

/**
 * RuleTest answers which rule a connection to a domain, an address or a
 * URL would hit, and the proxy it would go through.
 */
export function RuleTest({ open, onClose, onLocate }: { open: boolean; onClose: () => void; onLocate: (index: number) => void }) {
  const t = useT();
  const [target, setTarget] = useState("");
  const [network, setNetwork] = useState<"tcp" | "udp">("tcp");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState<MatchResult | null>(null);
  const go = async () => {
    if (!target.trim() || busy) return;
    setBusy(true);
    setError("");
    try {
      setResult(await Rules.test({ target, network }));
    } catch (e) {
      setResult(null);
      setError(errorText(e));
    }
    setBusy(false);
  };
  const r = result;
  const via = r?.chain.at(-1);
  return (
    <Dialog open={open} onClose={onClose} title={t("rules.test")} icon={<Crosshair size={16} />} size="wide">
      <p className="rules-intro muted">
        <Info size={13} />
        {t("rules.test.intro")}
      </p>
      <div className="rt-input">
        <Input
          autoFocus
          value={target}
          onChange={(e) => setTarget(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && go()}
          placeholder={t("rules.test.placeholder")}
          spellCheck={false}
          style={{ flex: 1 }}
        />
        <Segmented
          value={network}
          onChange={setNetwork}
          options={[
            { value: "tcp", label: "TCP" },
            { value: "udp", label: "UDP" },
          ]}
        />
        <Button variant="primary" loading={busy} disabled={!target.trim()} onClick={go}>
          {t("rules.test.run")}
        </Button>
      </div>
      {error && <div className="card-error">{error}</div>}
      {r && (
        <div className="rt-result reveal reveal-stack">
          {r.source === "mode" && <Banner tone="info">{t(r.mode === "global" ? "rules.test.modeGlobal" : "rules.test.modeDirect")}</Banner>}
          <dl className="rt-kv">
            <dt>{t("rules.test.rule")}</dt>
            <dd>
              {r.source === "rule" ? (
                <div className="rt-rule">
                  <Badge>{ruleType(r.ruleType ?? "", r.payload)}</Badge>
                  <span className="mono selectable ellipsis" title={r.payload}>
                    {r.payload || "—"}
                  </span>
                  <span className="faint tnum">#{r.index + 1}</span>
                  <Button size="sm" variant="ghost" onClick={() => onLocate(r.index)}>
                    {t("rules.test.locate")}
                  </Button>
                </div>
              ) : r.source === "none" ? (
                <span className="muted">{t("rules.test.none")}</span>
              ) : (
                <span className="muted">—</span>
              )}
            </dd>
            <dt>{t("rules.test.policy")}</dt>
            <dd style={{ fontWeight: 600 }}>{r.policy}</dd>
            <dt>{t("rules.test.chain")}</dt>
            <dd>
              <div className="rt-chain">
                {r.chain.map((h, i) => (
                  <Fragment key={`${i}-${h.name}`}>
                    {i > 0 && <ChevronRight size={13} className="faint flip-rtl" />}
                    <span className={`rt-hop${h === via ? " last" : ""}`} title={h.type}>
                      {h.name}
                      <span className="faint">{h.type}</span>
                    </span>
                  </Fragment>
                ))}
              </div>
            </dd>
            <dt>{t("rules.test.ips")}</dt>
            <dd className="mono selectable">{r.ips.length > 0 ? r.ips.join("  ") : <span className="faint">{t("rules.test.noIPs")}</span>}</dd>
          </dl>
          <div className="faint rt-foot">
            {r.host}:{r.port} · {r.network.toUpperCase()}
            {r.disabled > 0 && ` · ${t("rules.test.skipped", { n: r.disabled })}`}
          </div>
        </div>
      )}
    </Dialog>
  );
}
