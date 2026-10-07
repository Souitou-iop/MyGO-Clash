// Brand logos of the tested sites and services, from Logos by lndev
// (logos.lndev.me, MIT; see assets/logos/LICENSE). They are drawn inline so
// that the monochrome ones take the text color in both themes.
const svgs = import.meta.glob<string>("../assets/logos/*.svg", { query: "?raw", import: "default", eager: true });

const logos: Record<string, string> = {};
for (const [path, svg] of Object.entries(svgs)) {
  logos[path.slice(path.lastIndexOf("/") + 1, -4)] = svg;
}

/** Logo draws the logo of id, or the first letter of name without one. */
export function Logo({ id, name, size = 20 }: { id: string; name: string; size?: number }) {
  const svg = logos[id];
  if (!svg) {
    return (
      <span className="logo logo-letter" style={{ width: size, height: size, fontSize: size * 0.55 }} aria-hidden>
        {name.charAt(0).toUpperCase()}
      </span>
    );
  }
  return <span className="logo" style={{ width: size, height: size }} aria-hidden dangerouslySetInnerHTML={{ __html: svg }} />;
}
