// The service that TUN mode needs, offered as Clash Verge does: turning TUN
// on without it explains why, installs it, then turns TUN on; a service
// that an update of the app left behind is offered for updating.

import { System } from "../mygo";
import { confirm } from "../ui";
import { t } from "./i18n";
import { run, useApp } from "./store";

/** enableTunWithService installs the service, which asks for an administrator's authorization, then turns TUN mode on. */
export async function enableTunWithService(): Promise<void> {
  const svc = useApp.getState().state?.service;
  if (svc?.installed && svc.outdated) return updateService();
  if (await confirm({ title: t("service.tunTitle"), message: t("service.tunMessage"), confirm: t("service.installAndTun") })) {
    await run(() => System.installService(true), t("service.installFailed"), t("service.tunReady"));
  }
}

/** updateService offers to update a service of another version than the app's. */
export async function updateService(): Promise<void> {
  const { state, info } = useApp.getState();
  const message = t("service.outdatedMessage", { v: state?.service.version ?? "?", app: info?.version ?? "" });
  if (await confirm({ title: t("service.outdatedTitle"), message, confirm: t("service.update") })) {
    await run(() => System.installService(false), t("service.installFailed"), t("service.updated"));
  }
}

let offered = false;

/** offerServiceUpdate asks once a session to update an outdated service. */
export function offerServiceUpdate(): void {
  const svc = useApp.getState().state?.service;
  if (offered || !svc?.supported || !svc.installed || !svc.outdated) return;
  offered = true;
  void updateService();
}
