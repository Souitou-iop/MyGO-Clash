// Every country's flag of the bundled set, by its code. The system's emoji
// flags are not used: Windows has no flag glyphs, and Linux only has them
// when its emoji font does.
const files = import.meta.glob("/node_modules/flag-icons/flags/4x3/*.svg", {
  eager: true,
  query: "?url",
  import: "default",
}) as Record<string, string>;

const FLAGS: Record<string, string> = {};
for (const [path, url] of Object.entries(files)) {
  const cc = /\/([a-z]{2})\.svg$/.exec(path)?.[1];
  if (cc) FLAGS[cc.toUpperCase()] = url;
}

/** flagUrl returns the bundled flag of a country code, if it has one. */
export function flagUrl(code: string | undefined): string | undefined {
  const cc = code?.toUpperCase();
  return cc && cc.length === 2 ? FLAGS[cc] : undefined;
}
