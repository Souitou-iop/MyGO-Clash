import { useEffect, useRef } from "react";

export type Lang = "yaml" | "javascript" | "css";

/** CodeEditor edits text with CodeMirror, loaded when first shown. */
export default function CodeEditor({
  value,
  onChange,
  lang,
  readOnly,
  onSave,
}: {
  value: string;
  onChange?: (v: string) => void;
  lang: Lang;
  readOnly?: boolean;
  onSave?: () => void;
}) {
  const host = useRef<HTMLDivElement>(null);
  const view = useRef<import("@codemirror/view").EditorView | null>(null);
  const cbs = useRef({ onChange, onSave });
  cbs.current = { onChange, onSave };

  useEffect(() => {
    let disposed = false;
    (async () => {
      const [{ EditorView, keymap, lineNumbers, highlightActiveLine, highlightActiveLineGutter, drawSelection }, { EditorState }, cmds, lang_, search, language, autocomplete] =
        await Promise.all([
          import("@codemirror/view"),
          import("@codemirror/state"),
          import("@codemirror/commands"),
          lang === "yaml" ? import("@codemirror/lang-yaml").then((m) => m.yaml()) : lang === "css" ? import("@codemirror/lang-css").then((m) => m.css()) : import("@codemirror/lang-javascript").then((m) => m.javascript()),
          import("@codemirror/search"),
          import("@codemirror/language"),
          import("@codemirror/autocomplete"),
        ]);
      if (disposed || !host.current) return;
      const dark = document.documentElement.dataset.theme === "dark" || (document.documentElement.dataset.theme !== "light" && matchMedia("(prefers-color-scheme: dark)").matches);
      const theme = EditorView.theme(
        {
          "&": { backgroundColor: "var(--surface)", color: "var(--text)", height: "100%" },
          ".cm-content": { caretColor: "var(--accent)", padding: "10px 0" },
          ".cm-gutters": { backgroundColor: "var(--surface-2)", color: "var(--text-faint)", border: "none", borderRight: "1px solid var(--border)" },
          ".cm-activeLineGutter": { backgroundColor: "var(--surface-3)", color: "var(--text-2)" },
          ".cm-activeLine": { backgroundColor: "color-mix(in oklab, var(--accent) 5%, transparent)" },
          "&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection": { backgroundColor: "var(--accent-soft) !important" },
          ".cm-cursor": { borderLeftColor: "var(--accent)" },
          ".cm-panels": { backgroundColor: "var(--surface-2)", color: "var(--text)" },
          ".cm-searchMatch": { backgroundColor: "var(--warning-soft)" },
          ".cm-tooltip": { backgroundColor: "var(--surface)", border: "1px solid var(--border)" },
        },
        { dark },
      );
      const save = keymap.of([
        {
          key: "Mod-s",
          preventDefault: true,
          run: () => {
            cbs.current.onSave?.();
            return true;
          },
        },
      ]);
      const state = EditorState.create({
        doc: value,
        extensions: [
          lineNumbers(),
          highlightActiveLine(),
          highlightActiveLineGutter(),
          drawSelection(),
          cmds.history(),
          language.indentOnInput(),
          language.bracketMatching(),
          language.foldGutter(),
          language.syntaxHighlighting(language.defaultHighlightStyle, { fallback: true }),
          autocomplete.closeBrackets(),
          search.highlightSelectionMatches(),
          save,
          keymap.of([...cmds.defaultKeymap, ...cmds.historyKeymap, ...search.searchKeymap, cmds.indentWithTab]),
          lang_,
          theme,
          EditorState.tabSize.of(2),
          EditorState.readOnly.of(!!readOnly),
          EditorView.lineWrapping,
          EditorView.updateListener.of((u) => {
            if (u.docChanged) cbs.current.onChange?.(u.state.doc.toString());
          }),
        ],
      });
      view.current = new EditorView({ state, parent: host.current });
      if (!readOnly) view.current.focus();
    })();
    return () => {
      disposed = true;
      view.current?.destroy();
      view.current = null;
    };
    // The editor owns its text once created; value changes from outside
    // replace it below.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [lang, readOnly]);

  useEffect(() => {
    const v = view.current;
    if (v && v.state.doc.toString() !== value) {
      v.dispatch({ changes: { from: 0, to: v.state.doc.length, insert: value } });
    }
  }, [value]);

  return <div ref={host} className="editor-host" />;
}
