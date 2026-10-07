import { AlertTriangle, CheckCircle2, Info, Loader2, Search, X, XCircle } from "lucide-react";
import {
  type ButtonHTMLAttributes,
  type InputHTMLAttributes,
  type ReactNode,
  type RefObject,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { createPortal } from "react-dom";
import { create } from "zustand";
import { RainyCompass } from "../components/Art";
import { delayClass, delayText } from "../lib/format";
import { useT } from "../lib/i18n";
import { dismiss, useApp } from "../lib/store";

// ---------- Buttons ----------

type BtnProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "ghost" | "danger" | "default";
  size?: "sm" | "md" | "lg";
  icon?: ReactNode;
  loading?: boolean;
  tip?: string;
};

export function Button({ variant = "default", size = "md", icon, loading, tip, className = "", children, disabled, ...rest }: BtnProps) {
  const cls = ["btn", variant !== "default" && variant, size !== "md" && size, !children && "icon-btn", className]
    .filter(Boolean)
    .join(" ");
  return (
    <button type="button" className={cls} disabled={disabled || loading} data-tip={tip} aria-label={tip} {...rest}>
      {loading ? <Loader2 size={size === "sm" ? 13 : 15} className="spin" /> : icon}
      {children}
    </button>
  );
}

// ---------- Choices ----------

export function Switch({
  checked,
  onChange,
  disabled,
  busy,
  label,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
  disabled?: boolean;
  busy?: boolean;
  label?: string;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      className={`switch${checked ? " on" : ""}${busy ? " busy" : ""}`}
      disabled={disabled}
      onClick={() => !busy && onChange(!checked)}
    />
  );
}

/**
 * useIndicator places the sliding highlight of a segmented control or of
 * tabs on the active child, through the --ind-* properties of the box. The
 * highlight moves only once placed, so it never slides in from the start.
 */
function useIndicator<E extends HTMLElement>(value: unknown) {
  const ref = useRef<E>(null);
  useLayoutEffect(() => {
    const box = ref.current;
    if (!box) return;
    const place = () => {
      const a = box.querySelector<HTMLElement>(":scope > .active");
      box.classList.toggle("ind-on", !!a);
      if (!a) return;
      box.style.setProperty("--ind-x", `${a.offsetLeft}px`);
      box.style.setProperty("--ind-y", `${a.offsetTop}px`);
      box.style.setProperty("--ind-w", `${a.offsetWidth}px`);
      box.style.setProperty("--ind-h", `${a.offsetHeight}px`);
    };
    place();
    // Labels change width with the language and the font.
    const ro = new ResizeObserver(place);
    ro.observe(box);
    for (const c of box.children) ro.observe(c);
    const frame = requestAnimationFrame(() => box.classList.add("ind-ready"));
    return () => {
      ro.disconnect();
      cancelAnimationFrame(frame);
    };
  }, [value]);
  return ref;
}

export function Segmented<T extends string>({
  value,
  options,
  onChange,
  full,
}: {
  value: T;
  options: { value: T; label: ReactNode; icon?: ReactNode }[];
  onChange: (v: T) => void;
  full?: boolean;
}) {
  const ref = useIndicator<HTMLDivElement>(value);
  return (
    <div ref={ref} className={`segmented${full ? " full" : ""}`} role="radiogroup">
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          role="radio"
          aria-checked={o.value === value}
          className={o.value === value ? "active" : ""}
          onClick={() => o.value !== value && onChange(o.value)}
        >
          {o.icon}
          {o.label}
        </button>
      ))}
    </div>
  );
}

export function Select<T extends string | number>({
  value,
  options,
  onChange,
  width,
  disabled,
}: {
  value: T;
  options: { value: T; label: string }[];
  onChange: (v: T) => void;
  width?: number | string;
  disabled?: boolean;
}) {
  return (
    <select
      className="select"
      value={String(value)}
      disabled={disabled}
      style={{ width }}
      onChange={(e) => {
        const o = options.find((o) => String(o.value) === e.target.value);
        if (o) onChange(o.value);
      }}
    >
      {options.map((o) => (
        <option key={String(o.value)} value={String(o.value)}>
          {o.label}
        </option>
      ))}
    </select>
  );
}

// ---------- Inputs ----------

export function Input({ invalid, className = "", ...rest }: InputHTMLAttributes<HTMLInputElement> & { invalid?: boolean }) {
  return <input className={`input${invalid ? " invalid" : ""} ${className}`} spellCheck={false} autoComplete="off" {...rest} />;
}

export function SearchInput({ value, onChange, placeholder, width = 240 }: { value: string; onChange: (v: string) => void; placeholder?: string; width?: number }) {
  return (
    <div className="input-group" style={{ width }}>
      <Search size={14} />
      <input value={value} onChange={(e) => onChange(e.target.value)} placeholder={placeholder} spellCheck={false} />
      {value && (
        <button className="btn ghost sm icon-btn" style={{ height: 22, width: 22 }} onClick={() => onChange("")} aria-label="clear">
          <X size={12} />
        </button>
      )}
    </div>
  );
}

export function NumberInput({ value, onChange, min, max, width = 100, disabled }: { value: number; onChange: (v: number) => void; min?: number; max?: number; width?: number; disabled?: boolean }) {
  const [text, setText] = useState(String(value));
  useEffect(() => setText(String(value)), [value]);
  const n = Number(text);
  const invalid = text === "" || !Number.isInteger(n) || (min !== undefined && n < min) || (max !== undefined && n > max);
  return (
    <Input
      value={text}
      invalid={invalid}
      style={{ width }}
      disabled={disabled}
      inputMode="numeric"
      onChange={(e) => setText(e.target.value.replace(/[^\d]/g, ""))}
      onBlur={() => (!invalid && n !== value ? onChange(n) : setText(String(value)))}
      onKeyDown={(e) => e.key === "Enter" && (e.target as HTMLInputElement).blur()}
    />
  );
}

// ---------- Display ----------

export function Card({ title, icon, actions, children, className = "", bodyClass = "card-body" }: { title?: ReactNode; icon?: ReactNode; actions?: ReactNode; children?: ReactNode; className?: string; bodyClass?: string }) {
  return (
    <section className={`card ${className}`}>
      {(title || actions) && (
        <div className="card-head">
          <div className="card-title">
            {icon}
            {title}
          </div>
          <div className="spacer" />
          {actions}
        </div>
      )}
      <div className={bodyClass}>{children}</div>
    </section>
  );
}

export function Badge({ tone, children, tip }: { tone?: "accent" | "success" | "warning" | "danger" | "info"; children: ReactNode; tip?: string }) {
  return (
    <span className={`badge${tone ? ` ${tone}` : ""}`} data-tip={tip} style={tip ? { position: "relative" } : undefined}>
      {children}
    </span>
  );
}

export function Delay({ value, loading }: { value: number | undefined; loading?: boolean }) {
  const t = useT();
  const fresh = useFresh(value, loading);
  if (loading) return <Loader2 size={13} className="spin muted" />;
  return (
    <span key={fresh} className={`delay ${delayClass(value)}${fresh ? " fresh" : ""}`}>
      {delayText(value, t("common.timeout"))}
    </span>
  );
}

/**
 * useFresh counts the results that arrived since mounting: a test that
 * ended, or a value that changed. Keyed on it, an element replays its
 * arrival, so a retest that gives the same figure still shows.
 */
export function useFresh(value: unknown, loading?: boolean): number {
  const [n, setN] = useState(0);
  const last = useRef({ value, loading });
  useEffect(() => {
    const was = last.current;
    last.current = { value, loading };
    if (loading) return;
    if (was.loading || (was.value !== value && was.value !== undefined && value !== undefined)) setN((c) => c + 1);
  }, [value, loading]);
  return n;
}

export function Progress({ value, tone }: { value: number; tone?: "warning" | "danger" }) {
  return (
    <div className={`progress${tone ? ` ${tone}` : ""}`}>
      <div style={{ width: `${Math.max(0, Math.min(100, value))}%` }} />
    </div>
  );
}

export function Spinner({ size = 16 }: { size?: number }) {
  return <Loader2 size={size} className="spin muted" />;
}

export function Empty({ icon, art, title, children }: { icon?: ReactNode; art?: boolean; title: string; children?: ReactNode }) {
  return (
    <div className="empty">
      {art ? <RainyCompass /> : icon && <div className="empty-icon">{icon}</div>}
      <h3>{title}</h3>
      {children}
    </div>
  );
}

export function Banner({ tone, children, action }: { tone: "warning" | "danger" | "info"; children: ReactNode; action?: ReactNode }) {
  const Icon = tone === "info" ? Info : AlertTriangle;
  return (
    <div className={`banner ${tone}`}>
      <Icon size={16} />
      <div className="grow">{children}</div>
      {action}
    </div>
  );
}

// ---------- Settings rows ----------

export function Section({ title, children, extra }: { title: ReactNode; children: ReactNode; extra?: ReactNode }) {
  return (
    <div className="section">
      <div className="section-title">
        {title}
        <div className="spacer" />
        {extra}
      </div>
      <div className="rows">{children}</div>
    </div>
  );
}

export function Row({ label, desc, children, onClick, icon }: { label: ReactNode; desc?: ReactNode; children?: ReactNode; onClick?: () => void; icon?: ReactNode }) {
  return (
    <div className={`setting${onClick ? " clickable" : ""}`} onClick={onClick}>
      {icon && <div className="muted" style={{ display: "flex" }}>{icon}</div>}
      <div className="setting-text">
        <div className="setting-label">{label}</div>
        {desc && <div className="setting-desc">{desc}</div>}
      </div>
      <div className="setting-control" onClick={(e) => onClick && e.stopPropagation()}>
        {children}
      </div>
    </div>
  );
}

export function Field({ label, hint, error, children }: { label: ReactNode; hint?: ReactNode; error?: string; children: ReactNode }) {
  return (
    <div className="field">
      <label>{label}</label>
      {children}
      {error ? <div className="error">{error}</div> : hint ? <div className="hint">{hint}</div> : null}
    </div>
  );
}

export function Tabs<T extends string>({ value, tabs, onChange }: { value: T; tabs: { value: T; label: ReactNode; icon?: ReactNode }[]; onChange: (v: T) => void }) {
  const ref = useIndicator<HTMLDivElement>(value);
  return (
    <div ref={ref} className="tabs" role="tablist">
      {tabs.map((t) => (
        <button key={t.value} role="tab" aria-selected={t.value === value} className={`tab${t.value === value ? " active" : ""}`} onClick={() => onChange(t.value)}>
          {t.icon}
          {t.label}
        </button>
      ))}
    </div>
  );
}

/**
 * Collapse grows to show its children when it opens and shrinks away when
 * it closes, turning back midway if asked. Its children mount only while
 * they show.
 */
export function Collapse({ open, children, inline }: { open: boolean; children: ReactNode; /** grows sideways, inside a row */ inline?: boolean }) {
  const [shown, setShown] = useState(open);
  if (open && !shown) setShown(true);
  const ref = useRef<HTMLElement>(null);
  const was = useRef(open);
  const anim = useRef<Animation | null>(null);
  useLayoutEffect(() => {
    const el = ref.current;
    if (was.current === open || !el) return;
    was.current = open;
    if (matchMedia("(prefers-reduced-motion: reduce)").matches) {
      if (!open) setShown(false);
      return;
    }
    // From where it stands: a move under way, or closed, or open.
    const moving = anim.current?.playState === "running";
    const size = (e: HTMLElement) => (inline ? e.offsetWidth : e.offsetHeight);
    const from = moving ? el.getBoundingClientRect()[inline ? "width" : "height"] : open ? 0 : size(el);
    const fade = moving ? Number(getComputedStyle(el).opacity) : open ? 0 : 1;
    anim.current?.cancel();
    // What is not yet drawn takes a size it guesses; measure it drawn, and
    // keep it drawn while it moves: clipped to nothing as it starts, it
    // would otherwise skip painting and open blank for a frame or two.
    el.classList.add("measuring");
    const to = open ? size(el) : 0;
    el.style.overflow = "hidden";
    // In a flex or grid parent, the gap before it comes and goes with it.
    const parent = el.parentElement && getComputedStyle(el.parentElement);
    const gap = el.previousElementSibling && parent ? Number.parseFloat(inline ? parent.columnGap : parent.rowGap) || 0 : 0;
    const prop = inline ? "width" : "height";
    const margin = inline ? "marginInlineStart" : "marginBlockStart";
    const a = el.animate(
      [
        { [prop]: `${from}px`, opacity: fade, [margin]: `${from ? 0 : -gap}px` },
        { [prop]: `${to}px`, opacity: open ? 1 : 0, [margin]: `${to ? 0 : -gap}px` },
      ],
      // Closed, it holds still until it is gone, not back at full size
      // for the frame before.
      { duration: Math.min(380, 220 + Math.abs(to - from) / 8), easing: "cubic-bezier(0.3, 0.7, 0.2, 1)", fill: open ? "none" : "forwards" },
    );
    anim.current = a;
    a.onfinish = () => {
      el.style.overflow = "";
      el.classList.remove("measuring");
      if (!open) setShown(false);
    };
  }, [open, shown, inline]);
  if (!shown) return null;
  return inline ? (
    <span ref={ref as RefObject<HTMLSpanElement>} className="collapse-inline">
      {children}
    </span>
  ) : (
    <div ref={ref as RefObject<HTMLDivElement>}>{children}</div>
  );
}

/**
 * reflow, as the ref of a grid of cards, moves its cards to their new places
 * instead of having them jump there: when its columns change, as the page
 * narrows or widens with the sidebar, and when cards come, go or reorder.
 * New cards fade in.
 */
export function reflow(grid: HTMLElement | null) {
  if (!grid) return;
  const cols = () => getComputedStyle(grid).gridTemplateColumns.split(" ").length;
  // Places, unmoved by transforms, from the grid's top left.
  const place = (el: HTMLElement) => {
    const own = el.offsetParent === grid;
    return { x: el.offsetLeft - (own ? 0 : grid.offsetLeft), y: el.offsetTop - (own ? 0 : grid.offsetTop) };
  };
  const cards = () => [...grid.children] as HTMLElement[];
  const places = () => new Map(cards().map((c) => [c, place(c)]));
  const motion = { id: "reflow", duration: 320, easing: "cubic-bezier(0.3, 0.7, 0.2, 1)" };
  let n = cols();
  let last = places();
  let height = grid.offsetHeight;
  const move = (now: typeof last, arrived: boolean) => {
    if (reducedMotion()) return;
    // Rows come and go: what follows the grid moves with its height
    // instead of jumping.
    for (const a of grid.getAnimations()) if (a.id === "reflow") a.cancel();
    const h = grid.offsetHeight;
    if (h !== height) grid.animate([{ height: `${height}px`, alignContent: "start" }, { height: `${h}px`, alignContent: "start" }], motion);
    const top = grid.getBoundingClientRect().top;
    for (const [c, to] of now) {
      const from = last.get(c);
      // Only what is in sight, of grids of hundreds of nodes.
      const fromY = from?.y ?? to.y;
      if (Math.min(fromY, to.y) + top > innerHeight || Math.max(fromY, to.y) + top + c.offsetHeight < 0) continue;
      if (!from) {
        if (arrived) c.animate([{ opacity: 0, transform: "scale(0.97)" }, { opacity: 1, transform: "none" }], { ...motion, id: "arrive" });
        continue;
      }
      if (from.x === to.x && from.y === to.y) continue;
      // From where it shows now, a move already under way included.
      const tf = getComputedStyle(c).transform;
      const shown = new DOMMatrix(tf === "none" ? undefined : tf);
      for (const a of c.getAnimations()) if (a.id === "reflow") a.cancel();
      const dx = from.x + shown.m41 - to.x;
      const dy = from.y + shown.m42 - to.y;
      c.animate([{ transform: `translate(${dx}px, ${dy}px)` }, { transform: "none" }], motion);
    }
  };
  const settle = (now: typeof last) => {
    n = cols();
    last = now;
    height = grid.offsetHeight;
  };
  // Sizes: the grid's, which can change its columns, and each card's, which
  // moves those after it and only needs noting.
  const ro = new ResizeObserver(() => {
    const now = places();
    if (cols() !== n) move(now, false);
    settle(now);
  });
  // Cards that come, go or change places.
  const mo = new MutationObserver((records) => {
    const now = places();
    move(now, true);
    settle(now);
    for (const r of records) for (const el of r.addedNodes) if (el instanceof HTMLElement) ro.observe(el);
  });
  ro.observe(grid);
  for (const c of cards()) ro.observe(c);
  mo.observe(grid, { childList: true });
  return () => {
    ro.disconnect();
    mo.disconnect();
  };
}

/**
 * SideTips shows the tips of the sidebar beside it. The sidebar clips what
 * overflows it, so the usual tip above an element would be cut off there;
 * these float over the page instead, at the element's inline end.
 */
export function SideTips() {
  const [tip, setTip] = useState<{ text: string; x: number; y: number; rtl: boolean } | null>(null);
  useEffect(() => {
    const root = document.querySelector<HTMLElement>(".sidebar");
    if (!root) return;
    const over = (e: MouseEvent) => {
      const el = (e.target as Element).closest<HTMLElement>("[data-tip]");
      if (!el || !root.contains(el)) return setTip(null);
      const r = el.getBoundingClientRect();
      const rtl = getComputedStyle(el).direction === "rtl";
      setTip({ text: el.dataset.tip ?? "", x: rtl ? r.left - 8 : r.right + 8, y: r.top + r.height / 2, rtl });
    };
    const out = (e: MouseEvent) => {
      if (!root.contains(e.relatedTarget as Node | null)) setTip(null);
    };
    const hide = () => setTip(null);
    root.addEventListener("mouseover", over);
    root.addEventListener("mouseout", out);
    root.addEventListener("mousedown", hide);
    return () => {
      root.removeEventListener("mouseover", over);
      root.removeEventListener("mouseout", out);
      root.removeEventListener("mousedown", hide);
    };
  }, []);
  if (!tip?.text) return null;
  return createPortal(
    <div key={`${tip.text}${tip.y}`} className={`side-tip${tip.rtl ? " rtl" : ""}`} style={{ left: tip.x, top: tip.y }}>
      {tip.text}
    </div>,
    document.body,
  );
}

// ---------- Dialog ----------

const reducedMotion = () => matchMedia("(prefers-reduced-motion: reduce)").matches;

/**
 * usePresence keeps something on screen for ms after it closes, so it can
 * play its way out: leaving is true for that time.
 */
export function usePresence(open: boolean, ms: number): { mounted: boolean; leaving: boolean } {
  const [mounted, setMounted] = useState(open);
  if (open && !mounted) setMounted(true);
  useEffect(() => {
    if (open || !mounted) return;
    const id = setTimeout(() => setMounted(false), reducedMotion() ? 0 : ms);
    return () => clearTimeout(id);
  }, [open, mounted, ms]);
  return { mounted, leaving: mounted && !open };
}

/** dialogs stacks the open dialogs, the last on top. */
const dialogs: object[] = [];

export function Dialog({
  open,
  onClose,
  title,
  children,
  footer,
  size,
  flush,
  icon,
}: {
  open: boolean;
  onClose: () => void;
  title: ReactNode;
  children: ReactNode;
  footer?: ReactNode;
  size?: "narrow" | "wide" | "xwide";
  flush?: boolean;
  icon?: ReactNode;
}) {
  const latest = useRef(onClose);
  latest.current = onClose;
  useEffect(() => {
    if (!open) return;
    // Esc closes only the dialog on top, such as a confirmation over an editor.
    const me = {};
    dialogs.push(me);
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && dialogs.at(-1) === me && latest.current();
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("keydown", onKey);
      dialogs.splice(dialogs.indexOf(me), 1);
    };
  }, [open]);
  // On the way out it shows what it last showed, though its owner may
  // already have cleared what fed it.
  const { mounted, leaving } = usePresence(open, 140);
  const shown = useRef({ title, children, footer, icon });
  if (open) shown.current = { title, children, footer, icon };
  if (!mounted) return null;
  const v = shown.current;
  return createPortal(
    <div className={`overlay${leaving ? " leaving" : ""}`} onMouseDown={(e) => !leaving && e.target === e.currentTarget && onClose()}>
      <div className={`dialog${size ? ` ${size}` : ""}`} role="dialog" aria-modal="true">
        <div className="dialog-head">
          {v.icon}
          <h2>{v.title}</h2>
          <Button variant="ghost" size="sm" icon={<X size={15} />} onClick={onClose} tip="Esc" />
        </div>
        <div className={`dialog-body${flush ? " flush" : ""}`}>{v.children}</div>
        {v.footer && <div className="dialog-foot">{v.footer}</div>}
      </div>
    </div>,
    document.body,
  );
}

// ---------- Confirm & prompt ----------

interface Ask {
  kind: "confirm" | "prompt";
  title: string;
  message?: ReactNode;
  confirm?: string;
  danger?: boolean;
  label?: string;
  value?: string;
  password?: boolean;
  placeholder?: string;
  resolve: (v: string | boolean | null) => void;
}

const useAsk = create<{ ask: Ask | null }>(() => ({ ask: null }));

export function confirm(opts: Omit<Ask, "kind" | "resolve">): Promise<boolean> {
  return new Promise((resolve) => useAsk.setState({ ask: { ...opts, kind: "confirm", resolve: (v) => resolve(v === true) } }));
}

export function prompt(opts: Omit<Ask, "kind" | "resolve">): Promise<string | null> {
  return new Promise((resolve) =>
    useAsk.setState({ ask: { ...opts, kind: "prompt", resolve: (v) => resolve(typeof v === "string" ? v : null) } }),
  );
}

export function AskHost() {
  const ask = useAsk((s) => s.ask);
  const t = useT();
  const [value, setValue] = useState("");
  useEffect(() => setValue(ask?.value ?? ""), [ask]);
  if (!ask) return null;
  const close = (v: string | boolean | null) => {
    useAsk.setState({ ask: null });
    ask.resolve(v);
  };
  const ok = () => close(ask.kind === "prompt" ? value : true);
  return (
    <Dialog
      open
      size="narrow"
      onClose={() => close(null)}
      title={ask.title}
      footer={
        <>
          <Button onClick={() => close(null)}>{t("common.cancel")}</Button>
          <Button variant={ask.danger ? "danger" : "primary"} className={ask.danger ? "solid" : ""} onClick={ok} disabled={ask.kind === "prompt" && !value}>
            {ask.confirm ?? t("common.ok")}
          </Button>
        </>
      }
    >
      <div className="form">
        {ask.message && <div className="muted">{ask.message}</div>}
        {ask.kind === "prompt" && (
          <Field label={ask.label ?? ""}>
            <Input
              autoFocus
              type={ask.password ? "password" : "text"}
              value={value}
              placeholder={ask.placeholder}
              onChange={(e) => setValue(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && value && ok()}
            />
          </Field>
        )}
      </div>
    </Dialog>
  );
}

// ---------- Menu ----------

export interface MenuItem {
  label?: string;
  icon?: ReactNode;
  onClick?: () => void;
  danger?: boolean;
  disabled?: boolean;
  separator?: boolean;
}

/** Menu shows items under its trigger when the trigger is clicked. */
export function Menu({ trigger, items, align = "right" }: { trigger: (open: () => void) => ReactNode; items: MenuItem[]; align?: "left" | "right" }) {
  const [pos, setPos] = useState<{ x: number; y: number } | null>(null);
  const anchor = useRef<HTMLSpanElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  const open = () => {
    const r = anchor.current?.getBoundingClientRect();
    if (r) setPos({ x: align === "right" ? r.right : r.left, y: r.bottom + 4 });
  };
  useLayoutEffect(() => {
    if (!pos || !menu.current) return;
    const m = menu.current.getBoundingClientRect();
    let x = align === "right" ? pos.x - m.width : pos.x;
    let y = pos.y;
    x = Math.max(8, Math.min(x, window.innerWidth - m.width - 8));
    if (y + m.height > window.innerHeight - 8) y = Math.max(8, pos.y - m.height - 40);
    menu.current.style.left = `${x}px`;
    menu.current.style.top = `${y}px`;
  }, [pos, align]);
  useEffect(() => {
    if (!pos) return;
    const close = (e: MouseEvent | KeyboardEvent) => {
      if (e instanceof KeyboardEvent && e.key !== "Escape") return;
      if (e instanceof MouseEvent && menu.current?.contains(e.target as Node)) return;
      setPos(null);
    };
    const blur = () => setPos(null);
    window.addEventListener("mousedown", close);
    window.addEventListener("keydown", close);
    window.addEventListener("blur", blur);
    return () => {
      window.removeEventListener("mousedown", close);
      window.removeEventListener("keydown", close);
      window.removeEventListener("blur", blur);
    };
  }, [pos]);
  const { mounted: shown, leaving } = usePresence(!!pos, 100);
  return (
    <>
      <span ref={anchor} style={{ display: "inline-flex" }}>
        {trigger(open)}
      </span>
      {shown &&
        createPortal(
          <div ref={menu} className={`menu${leaving ? " leaving" : ""}`} style={{ left: -9999, top: -9999 }} role="menu">
            {items.map((it, i) =>
              it.separator ? (
                <div key={i} className="menu-sep" />
              ) : (
                <button
                  key={i}
                  role="menuitem"
                  className={`menu-item${it.danger ? " danger" : ""}`}
                  disabled={it.disabled}
                  onClick={() => {
                    setPos(null);
                    it.onClick?.();
                  }}
                >
                  {it.icon}
                  {it.label}
                </button>
              ),
            )}
          </div>,
          document.body,
        )}
    </>
  );
}

// ---------- Toasts ----------

const toastIcons = { success: CheckCircle2, error: XCircle, warning: AlertTriangle, info: Info };

/** leave slides a toast out, then closes the room it took, so the others
 * move up rather than jump. */
function leave(el: HTMLElement | null) {
  if (!el || el.dataset.leaving) return;
  el.dataset.leaving = "1";
  el.style.overflow = "hidden";
  const h = el.offsetHeight;
  // The borders stay; the margin takes them back along with the gap.
  const cs = getComputedStyle(el);
  const rest = 8 + Number.parseFloat(cs.borderTopWidth) + Number.parseFloat(cs.borderBottomWidth);
  el.animate(
    [
      { opacity: 1, transform: "none", height: `${h}px`, marginBlockEnd: "0px" },
      { opacity: 0, transform: "translateX(16px)", height: `${h}px`, marginBlockEnd: "0px", offset: 0.45 },
      { opacity: 0, transform: "translateX(16px)", height: "0px", paddingBlock: "0px", marginBlockEnd: `-${rest}px` },
    ],
    { duration: 260, easing: "cubic-bezier(0.3, 0.7, 0.2, 1)", fill: "forwards" },
  );
}

export function Toasts({ onAction }: { onAction: (action: string) => void }) {
  const toasts = useApp((s) => s.toasts);
  const position = useApp((s) => s.settings?.ui.toastPosition ?? "top-right");
  const t = useT();
  // Toasts gone from the store stay a moment to play their way out. They
  // are kept in the same render that drops them, never a frame later, or
  // they would blink out and the rest jump up before it began.
  const [leaving, setLeaving] = useState<typeof toasts>([]);
  const [prev, setPrev] = useState(toasts);
  if (prev !== toasts) {
    setPrev(toasts);
    const gone = prev.filter((p) => !toasts.some((n) => n.id === p.id));
    if (gone.length > 0 && !reducedMotion()) setLeaving((l) => [...l, ...gone]);
  }
  useEffect(() => {
    if (leaving.length === 0) return;
    const id = setTimeout(() => setLeaving((l) => l.filter((x) => !leaving.includes(x))), 280);
    return () => clearTimeout(id);
  }, [leaving]);
  const all = [...toasts, ...leaving].sort((a, b) => a.id - b.id);
  return (
    <div className={`toasts ${position}`} aria-live="polite">
      {all.map((n) => {
        const Icon = toastIcons[n.level as keyof typeof toastIcons] ?? Info;
        const out = leaving.includes(n);
        return (
          <div key={n.id} className={`toast ${n.level}`} ref={out ? leave : undefined} aria-hidden={out || undefined}>
            <Icon size={17} className="toast-icon" />
            <div className="grow">
              <div className="toast-title">{n.message}</div>
              {n.detail && <div className="toast-detail">{n.detail}</div>}
              {n.action && (
                <Button
                  size="sm"
                  variant="primary"
                  style={{ marginTop: 8 }}
                  onClick={() => {
                    dismiss(n.id);
                    onAction(n.action!);
                  }}
                >
                  {t(`action.${n.action}` as never) || n.action}
                </Button>
              )}
            </div>
            <Button variant="ghost" size="sm" icon={<X size={13} />} onClick={() => dismiss(n.id)} />
          </div>
        );
      })}
    </div>
  );
}
