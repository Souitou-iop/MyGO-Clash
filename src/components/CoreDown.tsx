import { RotateCw } from "lucide-react";
import { useState } from "react";
import { useT } from "../lib/i18n";
import { run, useApp } from "../lib/store";
import { Core } from "../mygo";
import { Banner, Button, Empty } from "../ui";

/**
 * CoreDown stands in for what a page shows while the core is not running:
 * why, and a button to start it again.
 */
export function CoreDown({ banner }: { banner?: boolean }) {
  const t = useT();
  const core = useApp((s) => s.state?.core);
  const [busy, setBusy] = useState(false);
  const starting = core?.status === "starting";
  const failed = core?.status === "error";
  const title = starting ? t("home.state.starting") : failed ? t("home.state.error") : t("common.coreNotRunning");
  const hint = starting ? t("core.startingHint") : failed ? core?.error || t("core.errorHint") : t("core.stoppedHint");
  const start = async () => {
    setBusy(true);
    await run(() => Core.restart(), t("common.failed"));
    setBusy(false);
  };
  const action = (
    <Button size={banner ? "sm" : "md"} variant={banner ? "default" : "primary"} icon={<RotateCw size={14} />} loading={busy || starting} onClick={start}>
      {failed ? t("core.restart") : t("core.start")}
    </Button>
  );
  if (banner) {
    return (
      <Banner tone="warning" action={action}>
        <b>{title}</b> · {hint}
      </Banner>
    );
  }
  return (
    <Empty title={title}>
      <p className="muted" style={{ maxWidth: 420, textAlign: "center" }}>
        {hint}
      </p>
      {action}
    </Empty>
  );
}
