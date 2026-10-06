// The layout of the home page.

/**
 * Cards are laid out in rows of twelve columns. Each card has the span it
 * wants and the least it can live with; rows take cards while their least
 * spans fit, then share the row out, so every row is full and the cards in
 * it are of one height, whatever is shown and in whatever order.
 */
const SPANS: Record<string, { span: number; min: number }> = {
  control: { span: 12, min: 12 },
  traffic: { span: 8, min: 8 },
  proxy: { span: 4, min: 4 },
  profile: { span: 4, min: 4 },
  ip: { span: 4, min: 4 },
  tailscale: { span: 4, min: 4 },
  test: { span: 8, min: 4 },
  core: { span: 4, min: 4 },
  system: { span: 4, min: 4 },
};

export type Density = "wide" | "medium" | "narrow";

function spanOf(id: string, d: Density) {
  const s = SPANS[id] ?? { span: 4, min: 4 };
  if (d === "narrow") return { span: 12, min: 12 };
  if (d === "medium") return { span: s.span >= 8 ? 12 : 6, min: s.min >= 8 ? 12 : 6 };
  return s;
}

export function pack(ids: string[], d: Density): { id: string; span: number }[] {
  const rows: string[][] = [];
  let row: string[] = [];
  let used = 0;
  for (const id of ids) {
    const { min } = spanOf(id, d);
    if (row.length > 0 && used + min > 12) {
      rows.push(row);
      row = [];
      used = 0;
    }
    row.push(id);
    used += min;
  }
  if (row.length > 0) rows.push(row);
  // A card alone at the end, which would rather share its row, takes the
  // card before it along.
  const last = rows[rows.length - 1];
  const prev = rows[rows.length - 2];
  if (last && prev && last.length === 1 && prev.length >= 2 && spanOf(last[0]!, d).span < 12) {
    const moved = prev[prev.length - 1]!;
    if (spanOf(moved, d).min + spanOf(last[0]!, d).min <= 12) last.unshift(prev.pop()!);
  }
  return rows.flatMap((r) => {
    const specs = r.map((id) => spanOf(id, d));
    const spans = specs.map((s) => s.span);
    let total = spans.reduce((a, b) => a + b, 0);
    // Too wide: the most flexible card gives way first.
    while (total > 12) {
      let best = -1;
      for (let i = 0; i < spans.length; i++) {
        const slack = spans[i]! - specs[i]!.min;
        if (slack > 0 && (best < 0 || slack > spans[best]! - specs[best]!.min)) best = i;
      }
      if (best < 0) break;
      const cut = Math.min(total - 12, spans[best]! - specs[best]!.min);
      spans[best]! -= cut;
      total -= cut;
    }
    // Too narrow: the row is shared out evenly, the rest to the widest.
    if (total < 12) {
      const each = Math.floor((12 - total) / r.length);
      for (let i = 0; i < spans.length; i++) spans[i]! += each;
      total += each * r.length;
      spans[spans.indexOf(Math.max(...spans))]! += 12 - total;
    }
    return r.map((id, i) => ({ id, span: spans[i]! }));
  });
}

export function density(width: number): Density {
  if (width >= 820) return "wide";
  if (width >= 540) return "medium";
  return "narrow";
}
