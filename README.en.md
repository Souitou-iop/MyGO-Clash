<p align="center">
  <img src="resources/icon.png" width="168" alt="MyGO-Clash">
</p>

<h1 align="center">MyGO-Clash</h1>

<p align="center">
  A cross-platform proxy client built on the <a href="https://github.com/MetaCubeX/mihomo">mihomo</a> core and the <a href="https://github.com/egoist/mygo">MyGo</a> framework<br>
  <i>迷子でもいい、前へ進め。</i>
</p>

<p align="center">
  <a href="README.md">简体中文</a> · <b>English</b> · <a href="README.ja.md">日本語</a>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/platform-macOS%20%7C%20Windows%20%7C%20Linux-4f8fc0" alt="Platforms">
  <img src="https://img.shields.io/badge/core-mihomo%201.19.32-e2617f" alt="Core">
  <img src="https://img.shields.io/badge/license-GPL--3.0-2a7ab0" alt="License">
  <img src="https://img.shields.io/badge/status-preview-e8b84a" alt="Status">
</p>

> [!NOTE]
> MyGO-Clash is an early preview, v0.1.0. The interface and the main flows have been tested on macOS (Apple Silicon). TUN and the system service, switching the system proxy, signing in to Tailscale, WebDAV sync, and running on Windows and Linux have not been fully verified in real use yet. Please report problems in [Issues](https://github.com/Souitou-iop/MyGO-Clash/issues).

## Features

### Proxy

- The mihomo core built in, running as its own process; install the system service to turn on TUN mode
- Rule, global and direct modes, switched from the home page, the tray, the quick panel or global shortcuts
- System proxy with a bypass list and PAC scripts, set again when another app changes it
- TUN with the Mixed, gVisor and System stacks, auto route, strict route, DNS hijacking and excluded ranges
- Delay tests and automatic checks of proxies; connections can close when the proxy or mode changes, to reconnect through the new route
- LAN sharing, IPv6, process matching, and an external controller for web dashboards

### Profiles

- Remote subscriptions, with the traffic used and the expiry date, updated on a schedule, and through the proxy when the provider can't be reached directly
- Local profiles, with a built-in YAML editor
- Extensions: YAML merges and JavaScript scripts, for every profile or one, and items to prepend, append or remove from rules, proxies and groups
- Proxies imported from share links (vmess://, ss://, trojan://, vless://, hysteria2://…)
- One-click import from `clash://install-config?url=…` links

### Tailscale

- A built-in Tailscale node in user space: nothing else to install, no admin rights; or drive the Tailscale app already on the computer
- Subnet routes and exit nodes through your rules, MagicDNS, and self-hosted control servers such as Headscale
- Your other devices can use this computer's proxy at its Tailscale address
- TUN mode and Tailscale kept out of each other's way

### Sync and backup

- Profiles and settings kept in step across computers through any WebDAV server: Nextcloud, Jianguoyun, Synology, InfiniCLOUD…
- End-to-end encryption: a key derived from your passphrase with Argon2id, XChaCha20-Poly1305 for each file, and file names hidden too
- Conflicts reported when a profile changed on two devices, while settings merge field by field; device-specific settings such as TUN and shortcuts never sync
- Backups on this computer or on WebDAV, and export to an encrypted file

### Tools

- Unlock tests: Netflix, Disney+, YouTube Premium, ChatGPT, Claude, Gemini, Spotify, TikTok and the Steam currency
- Exit IP lookup and reachability of common sites
- Connections, rules and logs

### Desktop

- A tray menu (modes, proxies, profiles, Tailscale), the speed next to the tray icon, a native quick panel and global shortcuts
- Home cards to turn on and off and put in order, laid out to fit the window's width
- Light and dark themes, several accent colors, your own font and CSS; the interface in Simplified Chinese and English
- Errors and warnings as native system notifications while the window isn't in front; clicking one opens the related page
- Lightweight mode: closes the web view to save memory while the core and the tray keep running
- Launch at login, silent start, and the terminal's proxy variables copied in one click
- Subscription URLs, passwords and other secrets encrypted, with the key in the system's secure storage (the macOS Keychain, Windows DPAPI, the Linux Secret Service)
- On Windows, one click lets Microsoft Store apps reach the proxy
- On macOS 26 and later, the app icon follows the system's light, dark, clear and tinted appearances

## Download

No release has been published yet; for now, build from source (below). Releases will appear on the [Releases](https://github.com/Souitou-iop/MyGO-Clash/releases) page.

## Building from source

You need:

- [Go](https://go.dev/dl/) 1.27.1 or later
- [Bun](https://bun.sh)
- On Linux, GTK 3 and WebKitGTK 4.1, e.g. `sudo apt install libwebkit2gtk-4.1-0` on Debian and Ubuntu
- On Windows 10, the [WebView2 Runtime](https://developer.microsoft.com/microsoft-edge/webview2/) (Windows 11 includes it)

```bash
git clone https://github.com/Souitou-iop/MyGO-Clash.git
cd MyGO-Clash
bun install
bun run dev     # development: rebuilds on changes
bun run build   # packages the app for this platform into build/
```

Build through the scripts of `package.json`: they add the `-tags=with_gvisor` mihomo needs. After changing the Go side's API, `bun run generate` updates the frontend bindings in `src/mygo.ts`.

The app icon comes from an Icon Composer document. After changing the design, run this on a Mac with Xcode 26 or later to make the macOS `Assets.car`, the `icon.png` of the other platforms and the interface's icons again:

```bash
go run ./cmd/genicon path/to/MyGo-Clash.icon
```

## Layout

```
main.go              entry: one executable runs the app, the core (core) or the privileged service (service)
internal/app         windows, tray, quick panel, notifications, settings and the frontend's API
internal/corehost    runs the mihomo core
internal/coremgr     runs the core as its own process or through the system service
internal/service     the privileged service TUN needs
internal/profiles    subscriptions and profiles
internal/enhance     merges, scripts and other extensions of profiles
internal/tailnet     Tailscale
internal/cloudsync   end-to-end encrypted sync
internal/sysproxy    the system proxy
src/                 the React frontend
cmd/genicon          makes every platform's icons from the Icon Composer document
```

## Thanks

- [mihomo](https://github.com/MetaCubeX/mihomo), the proxy core
- [MyGo](https://github.com/egoist/mygo), the cross-platform desktop framework
- [Tailscale](https://tailscale.com), for the networking

## Notice

- MyGO-Clash is an unofficial fan project. Its name, colors, compass and guitar pick pay homage to MyGO!!!!!, the band of BanG Dream!; it is not affiliated with Bushiroad or the band.
- This software is for learning about and researching network technology. Please follow the laws where you live.

## License

[GPL-3.0](LICENSE)
