import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, rmSync } from "node:fs";
import { resolve } from "node:path";
import { chromium } from "playwright-core";

const theme = process.argv.includes("--light") ? "light" : "dark";
const lang = process.argv.includes("--en") ? "en" : "zh";
const suffix = lang === "en" ? "-en" : "";
const outDir = resolve("out");
const videoDir = resolve("out/.video");
const gif = resolve(outDir, `hero-${theme}${suffix}.gif`);
const mp4 = resolve(outDir, `hero-${theme}${suffix}.mp4`);
const palette = resolve(outDir, ".palette.png");
const ffmpeg = process.env.FFMPEG || "/Volumes/SanDisk/Developer/Tools/ffmpeg/ffmpeg";

const edge = "/Volumes/SanDisk/Applications/AppPorts.noindex/Microsoft Edge.app/Contents/MacOS/Microsoft Edge";
const executablePath = process.env.CHROME_PATH || (existsSync(edge) ? edge : undefined);

mkdirSync(outDir, { recursive: true });
rmSync(videoDir, { recursive: true, force: true });
mkdirSync(videoDir, { recursive: true });

const browser = await chromium.launch(executablePath ? { executablePath } : { channel: "chrome" });
const context = await browser.newContext({
  viewport: { width: 1080, height: 576 },
  recordVideo: { dir: videoDir, size: { width: 1080, height: 576 } },
});
const page = await context.newPage();
await page.goto(`http://localhost:5174/?embed=1&theme=${theme}&lang=${lang}`);
await page.waitForSelector(".mcs-stage .ss-stage");
await page.waitForTimeout(13_500);

const video = page.video();
await page.close();
await context.close();
await browser.close();

const source = await video.path();
const trim = "0.55";
const length = "12.35";
const scale = "960:512:flags=lanczos";

execFileSync(ffmpeg, [
  "-y",
  "-ss",
  trim,
  "-t",
  length,
  "-i",
  source,
  "-vf",
  `fps=20,scale=${scale},palettegen=stats_mode=diff`,
  palette,
], { stdio: "inherit" });

execFileSync(ffmpeg, [
  "-y",
  "-ss",
  trim,
  "-t",
  length,
  "-i",
  source,
  "-i",
  palette,
  "-lavfi",
  `fps=20,scale=${scale}[x];[x][1:v]paletteuse=dither=bayer:bayer_scale=3`,
  "-loop",
  "0",
  gif,
], { stdio: "inherit" });

execFileSync(ffmpeg, [
  "-y",
  "-ss",
  trim,
  "-t",
  length,
  "-i",
  source,
  "-vf",
  "scale=1080:576:flags=lanczos",
  "-c:v",
  "libx264",
  "-pix_fmt",
  "yuv420p",
  "-movflags",
  "+faststart",
  mp4,
], { stdio: "inherit" });

rmSync(videoDir, { recursive: true, force: true });
rmSync(palette, { force: true });
console.log(`gif -> ${gif}`);
console.log(`mp4 -> ${mp4}`);
