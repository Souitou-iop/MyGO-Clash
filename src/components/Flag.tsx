import { flagUrl } from "../lib/flags";

/**
 * Flag shows a country's flag from the bundled set, on every platform
 * alike. While masked it is a neutral block: a blurred flag would still
 * tell its colors.
 */
export function Flag({ code, masked, className }: { code: string | undefined; masked?: boolean; className?: string }) {
  const cls = className ? `flag ${className}` : "flag";
  if (masked || !flagUrl(code)) return <span className={masked ? `${cls} flag-mask` : cls} />;
  return <img className={cls} src={flagUrl(code)} alt="" draggable={false} loading="lazy" />;
}
