# MyGO-Clash showcase

The README hero stage: the Connections page behind, the native quick panel in
front. The panel opens from the tray, turns on the system proxy, selects HK 01,
runs a latency test, and closes. One loop is about 12 seconds.

The stage is a faux UI built with [Stageshow](https://github.com/benjitaylor/skills)
and the product's real tokens, labels and quick-panel geometry. It is an
illustration for the README, not a runtime screenshot.

## Run

```bash
bun install
bun run dev
```

Open `http://localhost:5174/`. Add `?theme=light` for the light theme and
`?lang=en` for the English labels.

## Capture

```bash
bun run capture            # dark, Chinese
bun run capture --light    # light, Chinese
bun run capture --en       # dark, English
```

`scripts/capture.mjs` records the stage with Playwright and writes `out/hero-*.gif`
and `out/hero-*.mp4` through ffmpeg. It uses the local Edge/Chrome build; set
`CHROME_PATH` and `FFMPEG` when the defaults do not apply.

Copy the final files to `docs/showcase/`; keep `resources/` for assets that ship
inside the app.

Stills are filmed with the Stageshow `film.mjs` script:

```bash
CHROME_PATH="..." node "$STAGESHOW/scripts/film.mjs" \
  "http://localhost:5174/" --select ".mcs-stage .ss-stage" \
  --width 1440 --pad 0 --reduced --still --out out/hero-dark.png
```

The film script imports `playwright-core` from the Stageshow skill folder. Run
`npm install` there, or link its `node_modules/playwright-core` to this
project's copy. `bun run capture` does not need the Stageshow script.

## Attribution

`kit/` is the Stageshow React kit by the Stageshow authors, MIT licensed; see
`kit/LICENSE`. The rest of this folder is part of MyGO-Clash.
