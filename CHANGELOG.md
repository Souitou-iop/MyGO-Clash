# Changelog

The section of each version is what the app's update window shows.

## 0.2.0-beta

流量都去了哪、这个网站会走哪条规则、出口 IP 是不是机房，现在都能直接看到；另有托盘测速、局域网代理密码与 Windows 防泄露。/ See where your traffic went, which rule a site will hit, and whether your exit IP is a data center; plus latency tests in the tray, a password for the LAN proxy, and leak protection on Windows.

### ✨ 新增功能 / New Features

- 新增「流量统计」页：按今天（逐小时）、7 天、30 天查看上传下载总量，以及流量最多的应用、网站（按主域名合并）与节点；记录保存在本机 90 天，不参与同步，可一键清除。已有用户的侧边栏会自动出现这一页 / A new *Statistics* page shows upload and download for today (by hour), 7 or 30 days, with the apps, sites (by registrable domain) and nodes that carried the most; the history stays on this device for 90 days, is not synced, and can be cleared. It appears in existing sidebars by itself
- 总量取自内核自身的计数，一秒内开合的短连接也算在内；来不及归到具体项的部分记为「其他」。按应用统计需要把「进程匹配」设为「总是查找」，页面上可一键开启 / Totals come from the core's own counters, so connections that open and close within a second count too; what can't be attributed shows as *Other*. Splitting by app needs process matching set to *Always*, which the page can turn on
- 规则页新增「规则测试」：输入域名、IP 或网址，用内核正在使用的规则算出会命中哪一条、走哪个策略与完整代理链，以及解析到的地址，并可在列表中定位到该规则；测试不计入命中次数 / The Rules page has a *Rule test*: enter a domain, IP or URL to see, with the rules the core runs, which rule it hits, the policy and the full proxy chain, and the addresses it resolved to, then jump to the rule in the list; tests don't add to hit counts
- 首页与连通性页的出口 IP 多了「IP 质量」：家宽 / 机房 / 移动网络等类型与 0–100 的风险分，并标出 VPN、代理、Tor，数据来自 ipapi.is 与 proxycheck.io / The exit IP on the home and connectivity pages shows its *IP quality*: residential, data center, mobile and so on, a 0–100 risk score, and VPN, proxy or Tor flags, from ipapi.is and proxycheck.io
- 托盘的代理组菜单顶部新增「测试延迟」，延迟按快 / 中 / 慢标出颜色（macOS、Linux）；与快捷面板共用同一测试，不会重复运行 / Each proxy group in the tray menu starts with *Test latency*, and delays are marked fast, medium or slow by color (macOS, Linux); it shares one test with the quick panel, so they never run twice
- 开启「允许局域网连接」后可设置「需要密码」：局域网里的其他设备须用用户名和密码才能使用代理，本机应用、系统代理与 TUN 不受影响 / With *Allow LAN* on, *Require password* makes other devices on the network sign in to use the proxy, while apps on this computer, the system proxy and TUN work as before
- Windows：「代理与 TUN」新增「防泄露」分区。「DNS 防泄露」（严格路由）阻止 TUN 之外的网卡发出 DNS 查询，新安装默认开启，已有用户请手动打开；「防 WebRTC 泄露」为 Chrome、Edge、Brave 写入浏览器策略，网页拿不到真实 IP，关闭代理或退出应用时自动撤销，默认关闭（开启后浏览器会显示「由你的组织管理」） / Windows: *Proxy & TUN* has a *Leak protection* section. *DNS leak protection* (strict route) stops DNS queries from leaving through adapters other than TUN; it is on for new installs, and existing users should turn it on. *WebRTC leak protection* sets a browser policy for Chrome, Edge and Brave so pages can't learn your real IP, and is undone when the proxy is off or the app quits; it is off by default, as browsers then say they are *managed by your organization*

## 0.1.9-beta

开关怎么变的、是谁变的，都写进应用日志。/ The app's log now says how each switch changed and what changed it.

### ✨ 新增功能 / New Features

- 应用自身的日志文件（不是日志页的内核日志）记下系统代理、TUN 与模式的每一次开关，并写明是谁改的：托盘 `tray`、快捷键 `hotkey`、快捷面板 `quick panel`、主窗口 `window`、WebDAV 同步 `sync`、装完服务后自动开启 TUN 的 `service install`；应用自己撤掉 TUN 的两种情况也各自写明：服务不可用、服务被卸载 / The app's own log file, not the core's Logs page, records every change of the system proxy, TUN and mode with what made it: `tray`, `hotkey`, `quick panel`, `window`, WebDAV `sync`, or `service install` when installing the helper turns TUN on; the two times the app takes TUN back are written out as well, for want of the service and after it is uninstalled
- 每次启动先记一行当时的状态，例如 `switch: at start, system proxy on, TUN off, mode rule`；之后的变化形如 `switch: TUN off (tray)`、`switch: mode rule → global (quick panel)` / Each start logs the state it came up in, such as `switch: at start, system proxy on, TUN off, mode rule`, and later changes read like `switch: TUN off (tray)` or `switch: mode rule → global (quick panel)`
- 排查用得上：TUN 自己不见了，在应用数据目录的 `logs/app.log` 里搜 `switch:`，就能看出它是被哪里关掉的；关闭前若没有一行 `switch: TUN off`，那就不是应用关的，得当 bug 查 / This is what to search for when TUN disappears on its own: `switch:` in `logs/app.log` under the app's data directory shows where it was turned off, and with no `switch: TUN off` before it the app did not turn it off, which is a bug worth reporting

## 0.1.8-beta

新电脑首次启动不再卡在 GeoIP 下载，托盘如实显示开关状态，UWP 回环可以逐个选择应用，另有纯黑主题与几处动效、布局修正。/ A first start on a new computer no longer stalls on the GeoIP download, the tray shows the switches as they are, UWP loopback can be set app by app, and there is a pure black theme with a few motion and layout fixes.

### ✨ 新增功能 / New Features

- 安装包内置 GeoIP（geoip.metadb）与 GeoSite（geosite.dat）数据库，像 Clash Verge 一样：新电脑（例如刚通过 WebDAV 同步完）第一次启动就能应用含 GEOIP / GEOSITE 规则的订阅，不必先从 GitHub 下载 / The installers carry the GeoIP (geoip.metadb) and GeoSite (geosite.dat) databases, as Clash Verge does: on a new computer, such as one just synced over WebDAV, a profile with GEOIP or GEOSITE rules applies on the first start without a download from GitHub
- 缺少的 Geo 数据库改为依次尝试：订阅自己的 geox-url → jsDelivr 镜像 → GitHub，下载到临时文件校验通过才替换；未指定 geox-url 时「更新 GeoData」也走镜像 / A missing geo database is now fetched from the profile's own geox-url, then jsDelivr mirrors, then GitHub, through a temporary file that has to check out; without a geox-url, *Update GeoData* uses the mirror too
- 检查与下载应用更新时，内核在运行就先经内核代理访问 GitHub，再直连，下载最后可经 gh-proxy.org 镜像；更新包有签名，镜像无法篡改。此前更新检查只会直连，代理开着也连不上 GitHub / Update checks and downloads now reach GitHub through the core when it runs, then directly, then for downloads through the gh-proxy.org mirror; updates are signed, so a mirror cannot alter one. Checks used to go direct only and failed where GitHub is blocked, even with the proxy on
- UWP 回环改为应用列表：显示应用名与包名、勾出已放行的应用，可搜索、全选 / 全不选（作用于当前搜索结果）或逐个勾选，保存时一次授权；列表在你自己的账户里读取，其他账户的放行设置保持不变 / UWP loopback is now a list of the Store apps, with names, package names and the current exemptions checked: search, select all or none of what the search shows, or pick apps, then authorize once to save; the list is read in your own account, and other accounts' exemptions are left as they are
- 新增「纯黑（OLED）」开关：深色主题下页面、快捷面板与窗口底色改为纯黑 / A *Pure black (OLED)* switch turns the dark theme's pages, quick panel and window background pure black
- Windows：双击托盘图标打开主窗口 / Windows: double-clicking the tray icon opens the main window
- 配置未能应用时，首页横幅多了「重试」按钮 / The home page's *configuration did not apply* banner has a *Retry* button

### 🐛 修复 / Fixes

- 托盘菜单的系统代理、TUN 勾选与代理模式不再停在旧状态（Windows 与 macOS 都有此问题）：开关、模式、内核状态、订阅或 Tailscale 状态变化时菜单都会刷新；Tailscale 子菜单在标签上显示连接状态 / The tray menu's system proxy and TUN checkmarks and the mode no longer stay as they were (on Windows and macOS alike): the menu follows the switches, the mode, the core, the profile and Tailscale; Tailscale's submenu shows its state in its label
- 订阅页随侧边栏展开 / 收起改变列数时，卡片平滑缩放到新列宽，不再闪一下跳变 / On the profiles page, cards now grow or shrink smoothly into their columns when the sidebar changes how many fit, instead of snapping
- macOS：侧边栏收起后红绿灯不再压在侧边栏边缘上 / macOS: the window buttons no longer hang over the edge of the collapsed sidebar

## 0.1.7-beta

同步可以选择不加密，由你决定。/ Sync can be set up without encryption, if you choose.

### ✨ 新增功能 / New Features

- 设置同步的第二步现在可以在「加密（推荐）」与「不加密」之间选：不加密的保险库只压缩不加密，之后每台设备加入都不需要密码；默认仍是加密 / Sync setup's second step now chooses between *Encrypt (recommended)* and *Don't encrypt*: an unencrypted vault only compresses its data, and every device joins it without a passphrase; encrypting remains the default
- 选「不加密」时会把代价说清楚：WebDAV 服务商或服务器管理员能读到你同步的订阅链接（含机场令牌）与节点密码，以后要改回加密，得在新的文件夹重新设置一次 / Choosing it spells out the cost: your WebDAV provider or the server's admin can read the subscription links you sync (provider tokens included) and your node passwords, and encrypting later means setting sync up again in a new folder
- 加密与否属于保险库本身，不属于这台设备：文件夹里已有同步数据时沿用它当初的选择，加入时不会被本机改成另一种，界面也只在该文件夹确实未加密时才免掉密码 / The choice belongs to the vault, not to this device: a folder that already holds data keeps what it was created with, joining cannot switch it from this computer, and the passphrase is skipped only when that folder really is unencrypted
- 同步页的安全区在未加密时显示「未加密」警告标记；没有密码可改，「修改密码」改为说明加密的保险库才可以在这里换密码 / The sync page's security section badges an unencrypted vault, and *Change passphrase* is replaced by a note that only an encrypted vault has a passphrase to change here

## 0.1.6-beta

修掉上一版动效里各闪一帧的两处。/ Fixes the two single-frame blinks the last version's motion had.

### 🐛 修复 / Fixes

- 代理组展开时不再空白一帧：组里的节点在收起状态下不算待在屏幕内，因此跳过绘制，展开开始的那一帧便什么也没有；现在整个动画期间都保持绘制 / A proxy group no longer opens blank for a frame: its nodes counted as off screen while it was closed, so they skipped drawing and the first frame showed nothing; they are now kept drawn while it moves
- 代理组收起结束时不再跳回满尺寸那一帧 / A group closing no longer springs back to full size for the frame before it goes
- 通知消失时不再闪一下：原先它先从画面里去掉、下一帧才放回来播放退场，其余通知会先跳一下才开始平滑补位；现在在同一次渲染里留下 / A toast that left no longer blinks: it was dropped from the screen and put back a frame later to play its way out, so the others jumped before sliding up; it is now kept in the same render
- 菜单关闭后不再留下一个多余的窗口失焦监听器 / The menu no longer leaves a window blur listener behind when it closes

## 0.1.5-beta

规则页重做，内核未运行时说明原因并给出下一步，修掉五个名不副实的行为，弹窗与列表改为一气呵成的动效。/ The rules page is rebuilt, pages say why the core is down and how to start it, five actions that did not match their names are fixed, and dialogs, toasts and lists now move in one motion.

### ✨ 新增功能 / New Features

- 规则页说明规则的生效方式（从上到下匹配，第一条命中的决定连接去向），页头「编辑规则」直接打开当前订阅的规则编辑器 / The rules page explains how rules take effect — matched top to bottom, the first that matches decides where a connection goes — and its header opens the current profile's rules editor
- 规则可临时停用，重载配置后自动恢复；停用了规则时显示提示条，数出条数并一键「全部恢复」/ A rule can be turned off until the configuration reloads, with a banner that counts them and turns them all back on at once
- 每条规则的 ⋯ 菜单：按配置文件写法复制规则、前往其策略组（在代理页展开并高亮该组）、或在此之前新建规则 / Every rule has a ⋯ menu to copy it as configurations write it, jump to its target group on the proxies page which opens and highlights it, or start a new rule before it
- 规则可按序号或命中次数排序，按「命中过」「从未命中」「已停用」筛选；内核未运行、配置没有规则、筛选无结果时都给出下一步 / Rules sort by hits and filter to those matched, never matched or turned off, and every empty state — core down, no rules, nothing matching — says what to do next
- 内核未运行时，代理、连接、规则、连通性、Tailscale 页与首页控制卡片都会说明原因，并提供「启动内核」按钮 / When the core is not running, the proxies, connections, rules, connectivity and Tailscale pages and the home control card say why and offer a *Start core* button
- 日志页新增「导出」，把当前显示的日志保存为文件 / The logs page exports the logs it currently shows to a file
- 代理页新增「隐藏不可用」，隐藏上次测速失败的节点，当前选中的节点始终保留 / The proxies page can hide nodes whose last test failed, always keeping the selected node

### ✨ 改进 / Improvements

- 名称改用配置文件与日常用语的写法：连接页的规则类型显示为 `DOMAIN-SUFFIX` 这类形式，代理组类型显示「手动选择」「自动测速」「故障转移」等，Tailscale 的状态、日志等级与查找进程的三种模式也不再是英文标识 / Names now read the way people use them: rule types on the connections page as configurations write them (DOMAIN-SUFFIX), group types as Manual, Fastest and Fallback, and the Tailscale state, log levels and find-process modes named rather than shown as identifiers
- 十余项没有说明的设置补上了说明 / Over a dozen settings that did not say what they do gained a description
- 订阅更新失败的原因点击即可展开全文，旁边有「重试」；删除正在使用的订阅时，提示将自动切换到列表中的下一个 / A failed subscription update expands to its full reason on a click, with a *Retry* button, and deleting the profile in use says another takes its place
- 连通性页的解锁结果注明是经由哪个节点测得，换了节点后旧结果标为「节点已更换」/ Unlock results say which node they went through, and a result taken through a node no longer in use is marked as stale
- 单个节点测速无法完成时显示真实原因，不再一律显示「超时」/ A node test that could not run says why instead of always showing a timeout
- 退出应用前会先确认，因为退出会停止代理并关闭系统代理与 TUN / Quitting asks first, as it stops the proxy and turns off the system proxy and TUN
- 弹窗与菜单关闭时播放反向的打开动画，期间保持关闭前的内容，不再直接消失；通知消失时先滑出并让出位置，其余通知平滑补位 / Dialogs and menus play their opening animation backwards as they close, still showing what they last showed, and a dismissed toast slides out and gives back its room so the others move up rather than jump
- 代理组展开收起改为一调动效：内容平滑长出与收回，箭头随之转动，中途可反向 / Proxy groups open and close in one motion: the nodes grow into view and shrink away, turning back midway, with the arrow turning along with them
- 设置行里随开关出现的分钟数、新建订阅的远程字段与同步向导的提示都改为平滑展开，弹窗高度随之平滑变化 / The minutes that appear beside a switch, the remote fields of a new profile and the notices of the sync setup expand smoothly, and the dialog's height changes with them
- 同步向导按前进或后退方向横向滑动，切换设置页标签时内容淡入上浮 / The sync wizard slides the way it is navigated, and a settings tab switched to fades in and up
- 订阅卡片拖拽排序后、编辑器里上移下移条目时，条目平滑滑到新位置，新增的卡片淡入；所有动画遵循系统「减少动态效果」设置 / Profiles dragged into a new order and items moved up or down in the editors glide to their place, new cards fade in, and every animation honors the system's reduce-motion setting
- 文案补齐全部 15 种语言 / The wording is complete in all 15 languages

### 🐛 修复 / Fixes

- 订阅更新间隔填 0 原本会停止自动更新，与「按机场建议」的说明相反；现在按机场最近一次给出的建议更新，没有建议时每天一次，编辑框也会显示当前实际生效的间隔 / An update interval of 0 stopped updating, unlike the hint that said it follows the provider; it now follows the suggestion from the last download, else once a day, and the dialog shows the interval actually in use
- 「后台时暂停图表」原本保存后没有任何作用；现在窗口在后台时图表保持最后一帧，回到窗口后补齐 / "Pause graphs in the background" was saved but read nowhere; graphs now hold their last picture while the window is in the background and catch up when you return
- 规则、节点与代理组编辑器在点取消、按 Esc 或切到「以 YAML 编辑」时会直接丢弃改动；现在会先询问是否放弃 / The rules, nodes and groups editor silently dropped unsaved changes on Cancel, Esc or "Edit as YAML"; it asks first now
- Esc 原本会穿透确认提示关掉底下的编辑器；现在只关闭最上层的弹窗 / Esc reached past a confirmation and closed the editor under it; it now closes only the dialog on top
- 端口设置把应用自己正在使用的 SOCKS、HTTP、redir 与 TProxy 端口误报为「已占用」/ The ports dialog wrongly called the app's own SOCKS, HTTP, redir and TProxy ports in use
- 连接详情的数据冻结在打开的那一刻，并对已关闭的连接仍显示「关闭连接」；现在随连接实时刷新，并标注已关闭 / Connection details froze when opened and offered to close a connection already closed; they now follow the connection and mark it closed
- 默认展开的代理组第一次点击没有反应 / The first click on a proxy group that was open by default did nothing

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
