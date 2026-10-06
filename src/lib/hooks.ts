import { Channel } from "mygo-runtime";
import { useCallback, useEffect, useRef, useState } from "react";
import { useApp } from "./store";

/** useAsync loads data, again when deps change, and offers a reload. */
export function useAsync<T>(fn: () => Promise<T>, deps: unknown[]) {
  const [data, setData] = useState<T | undefined>(undefined);
  const [error, setError] = useState<unknown>(undefined);
  const [loading, setLoading] = useState(true);
  const seq = useRef(0);
  const load = useCallback(async () => {
    const n = ++seq.current;
    setLoading(true);
    try {
      const v = await fn();
      if (n === seq.current) {
        setData(v);
        setError(undefined);
      }
    } catch (e) {
      if (n === seq.current) setError(e);
    } finally {
      if (n === seq.current) setLoading(false);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);
  useEffect(() => {
    void load();
  }, [load]);
  return { data, error, loading, reload: load, setData };
}

/** useStream calls a streaming method with a channel while mounted. */
export function useStream<T>(start: (ch: Channel<T>) => Promise<void>, onValue: (v: T) => void, deps: unknown[], enabled = true) {
  const cb = useRef(onValue);
  cb.current = onValue;
  useEffect(() => {
    if (!enabled || useApp.getState().preview) return;
    let stopped = false;
    let ch: Channel<T> | null = null;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const loop = () => {
      if (stopped) return;
      ch = new Channel<T>((v) => cb.current(v));
      start(ch)
        .catch(() => {})
        .finally(() => {
          if (!stopped) timer = setTimeout(loop, 1500);
        });
    };
    loop();
    return () => {
      stopped = true;
      clearTimeout(timer);
      ch?.close();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...deps, enabled]);
}

export function useInterval(fn: () => void, ms: number | null) {
  const cb = useRef(fn);
  cb.current = fn;
  useEffect(() => {
    if (ms === null) return;
    const id = setInterval(() => cb.current(), ms);
    return () => clearInterval(id);
  }, [ms]);
}

export function useDebounced<T>(value: T, ms: number): T {
  const [v, setV] = useState(value);
  useEffect(() => {
    const id = setTimeout(() => setV(value), ms);
    return () => clearTimeout(id);
  }, [value, ms]);
  return v;
}

/** useNow re-renders every interval, for relative times. */
export function useNow(ms = 30000) {
  const [, set] = useState(0);
  useInterval(() => set((n) => n + 1), ms);
}

/** useBusy tracks actions in flight by key. */
export function useBusy() {
  const [busy, setBusy] = useState<Record<string, boolean>>({});
  const wrap = useCallback(async <T,>(key: string, fn: () => Promise<T>): Promise<T | undefined> => {
    setBusy((b) => ({ ...b, [key]: true }));
    try {
      return await fn();
    } finally {
      setBusy((b) => {
        const { [key]: _, ...rest } = b;
        return rest;
      });
    }
  }, []);
  return [busy, wrap] as const;
}

/**
 * useWidth follows the width of an element, also one that renders later:
 * pass ref as the element's ref; el holds the element.
 */
export function useWidth<T extends HTMLElement>(): [(node: T | null) => void, number, React.RefObject<T | null>] {
  const el = useRef<T | null>(null);
  const ro = useRef<ResizeObserver | null>(null);
  const [width, setWidth] = useState(0);
  const ref = useCallback((node: T | null) => {
    ro.current?.disconnect();
    ro.current = null;
    el.current = node;
    if (!node) return;
    setWidth(node.clientWidth);
    ro.current = new ResizeObserver(() => setWidth(node.clientWidth));
    ro.current.observe(node);
  }, []);
  return [ref, width, el];
}
