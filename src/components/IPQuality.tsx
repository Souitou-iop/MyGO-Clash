import { useEffect, useState } from "react";
import { type Key, useT } from "../lib/i18n";
import { type IPQuality, Tools } from "../mygo";
import { Badge } from "../ui";

// Answers by address, so that a refresh or a second card asking about the
// same address does not ask the services again. A failure is kept for a
// while too: the services are free, and hammering them helps nobody.
const TTL = 10 * 60_000;
const cache = new Map<string, { at: number; q: Promise<IPQuality | null> }>();

function lookup(ip: string): Promise<IPQuality | null> {
  const hit = cache.get(ip);
  if (hit && Date.now() - hit.at < TTL) return hit.q;
  const q = Tools.ipQuality(ip).catch(() => null);
  cache.set(ip, { at: Date.now(), q });
  return q;
}

/** useIPQuality loads the quality of an address once the address is known.
 * It never errors: without an answer there is simply nothing to show. */
export function useIPQuality(ip: string | undefined): IPQuality | null {
  const [q, setQ] = useState<{ ip: string; v: IPQuality | null } | null>(null);
  useEffect(() => {
    if (!ip) return;
    let live = true;
    void lookup(ip).then((v) => live && setQ({ ip, v }));
    return () => {
      live = false;
    };
  }, [ip]);
  return q && q.ip === ip ? q.v : null;
}

const KINDS: Record<string, Key> = {
  residential: "ipq.residential",
  datacenter: "ipq.datacenter",
  mobile: "ipq.mobile",
  business: "ipq.business",
};

/** riskTone colors a risk score: calm below 25, wary below 60. */
export const riskTone = (risk: number) => (risk < 25 ? "success" : risk < 60 ? "warning" : "danger");

/**
 * IPQualityRows are the rows of a <dl class="kv"> that say what kind of
 * line an address is and how risky it looks. They appear after the lookup
 * and stay away when it fails.
 */
export function IPQualityRows({ ip }: { ip: string | undefined }) {
  const t = useT();
  const q = useIPQuality(ip);
  if (!q) return null;
  const kind = KINDS[q.kind];
  const scored = q.risk >= 0;
  const flags = [q.vpn && "VPN", q.tor && "Tor", q.proxy && !q.vpn && !q.tor && t("ipq.proxy")].filter(Boolean);
  if (!kind && !scored && flags.length === 0) return null;
  const label = [kind && t(kind), scored && t("ipq.risk", { n: q.risk })].filter(Boolean).join(" · ");
  const tip = [q.org, q.source].filter(Boolean).join(" · ");
  return (
    <>
      <dt>{t("ipq.quality")}</dt>
      <dd className="ipq" title={tip}>
        {label && <Badge tone={scored ? riskTone(q.risk) : undefined}>{label}</Badge>}
        {flags.map((f) => (
          <Badge key={String(f)} tone="warning">
            {f}
          </Badge>
        ))}
      </dd>
    </>
  );
}
