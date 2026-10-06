# Changelog

The section of each version is what the app's update window shows.

## 0.1.0-beta

MyGO-Clash 的首个公开测试版，基于 mihomo 内核与 MyGo 框架。测试版不会推送给已安装的应用自动更新，欢迎反馈问题。
The first public beta of MyGO-Clash, built on the mihomo core and the MyGo framework. Installed apps are not offered beta versions as updates.

### ✨ 新增功能 / New Features

- 内嵌 mihomo 1.19.32 内核，支持规则、全局、直连三种模式，配置仅在内存中传递 / Embedded mihomo 1.19.32 core with rule, global and direct modes; the configuration is passed in memory only
- 订阅与本地配置管理，支持 Merge / Script 扩展与可视化规则、代理组编辑器 / Subscription and local profile management with Merge / Script extensions and visual rule and proxy-group editors
- 代理节点延迟测试、连接管理、实时日志、流量监控与流媒体解锁检测 / Proxy delay tests, connection management, live logs, traffic monitoring and streaming-unlock checks
- 系统代理（含 PAC 与守护）、TUN 模式与特权服务 / System proxy (with PAC and a guard), TUN mode and a privileged helper service
- 内置 Tailscale 节点：浏览器、二维码或 Auth Key 登录，支持 Headscale，也可管理已安装的 Tailscale 应用 / A built-in Tailscale node: sign in by browser, QR code or auth key, Headscale supported; it can also drive an installed Tailscale app
- WebDAV 端到端加密同步配置与设置（Argon2id + XChaCha20-Poly1305，三路合并解决冲突）/ End-to-end encrypted WebDAV sync of profiles and settings (Argon2id + XChaCha20-Poly1305, three-way merge for conflicts)
- 配置静态加密存储（macOS 钥匙串 / Linux Secret Service / Windows DPAPI）/ Encryption at rest through the system keyring (macOS Keychain / Linux Secret Service / Windows DPAPI)
- 系统原生通知、托盘菜单、原生快捷面板、全局快捷键与轻量模式 / Native notifications, tray menu, a native quick panel, global shortcuts and lightweight mode
- 应用内自动更新（签名校验）/ In-app self-updates with signature verification
- 界面支持 15 种语言 / An interface in 15 languages

#### 🖥️ Windows

- 提供 x64 与 ARM64 的 NSIS 安装包 / NSIS installers for x64 and ARM64

#### 🍎 macOS

- 提供 Apple 芯片与 Intel 芯片分开的 DMG 安装包，最低支持 macOS 12 / Separate DMG installers for Apple silicon and Intel, requires macOS 12 or later

#### 🐧 Linux

- 提供 DEB、RPM、Arch 软件包与 AppImage，支持 x64 与 ARM64 / DEB, RPM and Arch packages plus AppImage, for x64 and ARM64

### ⚠️ 已知问题 / Known Issues

- 测试版尚未在全部平台实机验证 TUN、服务安装与系统代理切换，如遇问题请提交 Issue / TUN, service installation and the system-proxy switch are not yet verified on real hardware on every platform; please file an issue if anything breaks
- macOS 版本未经 Apple 公证，首次打开需在「系统设置 → 隐私与安全性」中允许 / The macOS build is not notarized; on first launch, allow it in System Settings → Privacy & Security

## 0.1.0

The first preview of MyGO-Clash.

- The mihomo core with rule, global and direct modes, the system proxy and TUN
- Subscriptions and local profiles, with merges and scripts
- A built-in Tailscale node
- End-to-end encrypted sync of profiles and settings through WebDAV
- Native system notifications, a tray menu, a quick panel and global shortcuts
- Updates of the app itself
- An interface in 15 languages

MyGO-Clash 的首个预览版：mihomo 内核与三种代理模式、系统代理与 TUN、订阅与配置扩展、内置 Tailscale、经 WebDAV 端到端加密同步、系统原生通知、托盘与快捷面板、应用自动更新，界面支持 15 种语言。
