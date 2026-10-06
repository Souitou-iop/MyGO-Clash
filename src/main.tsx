import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import { boot, useApp } from "./lib/store";
import "./styles/tokens.css";
import "./styles/base.css";
import "./styles/components.css";
import "./styles/pages.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);

// In development, the store is reachable from the debug API's eval.
if (import.meta.env.DEV) (window as unknown as { __app: typeof useApp }).__app = useApp;

boot().catch((e) => {
  console.error("boot", e);
});
