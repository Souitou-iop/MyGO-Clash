# Changelog

The section of each version is what the app's update window shows.

## 0.1.4-beta

修复编辑窗口闪烁，并让卡片随侧边栏平滑移动。/ Fixes flickering editors and lets cards glide as the sidebar folds.

### 🐛 修复 / Fixes

- 修复订阅菜单中的规则、节点、代理组、覆写与脚本编辑窗口不停闪烁、无法使用的问题：翻译函数每次渲染都会重建，编辑器读取内容的副作用因此陷入循环（1.5 秒内重复读取上百次）；现在按语言缓存，每个编辑器只读取一次 / Fixed the rule, proxy, group, override and script editors in the profile menu flickering and being unusable: the translation function was rebuilt on every render, so the effect that loads their content looped (hundreds of reloads in 1.5 seconds); it is now cached per language and each editor loads once
- 代理、订阅、连通性、规则与 Tailscale 页面的卡片在侧边栏伸缩导致列数变化时，从原位置平滑滑到新位置，下方内容同步移动，不再跳动 / Cards on the proxies, profiles, connectivity, rules and Tailscale pages now glide from their old position to the new one when the sidebar changes the column count, and the content below moves with them instead of jumping

## 0.1.3-beta

界面细节打磨。/ Interface polish.

### ✨ 改进 / Improvements

- 全局主按钮参照 Linear、Raycast 重排：实色底、顶部 1px 内高光与贴身投影，悬停加深、按下下沉；所有按钮按下时轻微缩小 / The primary button is redesigned after Linear and Raycast: a solid fill with a 1px inner highlight and a close shadow; hover darkens it, pressing sinks it, and every button shrinks slightly while pressed
- 侧边栏收起重做为一条动效：图标固定在同一竖线上只有宽度在动，文字淡出，流量数字原位交叉淡入为短格式，警告圆点平滑移到角落 / The sidebar collapse is now one motion: icons stay on a fixed vertical line so only the width animates, labels fade out, the traffic numbers cross-fade into their short form, and the warning dot glides to its corner
- 侧边栏悬停提示改为浮在页面一侧，不再被侧边栏裁剪 / Sidebar tooltips now float beside the page instead of being clipped by the sidebar
- 「同步与备份」未配置时的引导卡片改为朴素样式：去掉渐变光晕与卖点清单，换成图标、标题、一行说明与按钮 / The unconfigured sync card is plainer: the gradient glow and the checklist give way to an icon, a title, one line of text and the buttons

## 0.1.2-beta

界面动效与新连通性页。/ Motion across the app and a new connectivity page.

### ✨ 改进 / Improvements

- 解锁检测页升级为连通性页：出口 IP、国内外分流与流媒体、AI 服务逐项检测，配品牌图标、结果柱条与二维码 / The unlock page becomes a connectivity page: exit IP, domestic and proxy routing, streaming and AI services are checked item by item, with brand logos, result bars and a QR code
- 分段选择器与标签的选中滑块平滑移动，文字颜色同步淡入 / Segmented controls and tabs: the selection slides over smoothly while label colors cross-fade
- 页面切换淡入并轻微上移 / Switching pages fades the content in and up
- 首页主开关：开启时圆标弹跳并扩出波纹，关闭时波纹收回 / The home main switch bounces with an outward ripple when enabled and an inward one when disabled
- 每次测速结束后延迟数字亮一下（首页延迟卡、代理节点与连通性页）/ Delay numbers flash after every retest (the home latency card, proxy nodes and the connectivity page)
- 选中代理节点时卡片弹跳并扩出光环，仅对点选的节点播放 / Selecting a proxy node bounces the card with a halo, only for nodes you click
- 首页卡片仅在启动后首次进入时依次入场；骨架加载完成后内容淡入替换 / Home cards stagger in only on the first visit after launch; skeletons fade into their content
- 连通性结果柱条从底部长出 / Connectivity bars grow from the bottom
- 图标动效（如上传图标），遵循系统"减少动态效果"设置：开启时动效时长归零 / Animated icons (such as upload), honoring the system reduce-motion setting, which zeroes animation durations

## 0.1.1-beta

图标与侧边栏的打磨版本。/ A polish release for the compass icon and the sidebar.

### ✨ 改进 / Improvements

- 罗盘图标按几何结构整体重画：星芒、M、圆环与凸起全部为精确直线与平滑曲线，不再带有描摹原图时带入的噪点 / The compass icon is rebuilt from fitted geometry: every star ray, the M, the ring and the bumps are precise straight lines and smooth curves, with none of the noise that came from tracing the source image
- 收起侧边栏时流量以紧凑格式显示（最多 4 字符，如 512B、9.9K、1.0M），完整数值移到悬停提示 / With the sidebar collapsed, traffic shows in a compact format of at most four characters (512B, 9.9K, 1.0M), with the full value in the hover tooltip

### 🐛 修复 / Fixes

- 修复展开侧边栏时闪现横向滚动条的问题 / Fixed a horizontal scrollbar that flashed while the sidebar expanded

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
