import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import { boot } from "./lib/store";
import "./styles/tokens.css";
import "./styles/base.css";
import "./styles/components.css";
import "./styles/pages.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);

boot().catch((e) => {
  console.error("boot", e);
});
