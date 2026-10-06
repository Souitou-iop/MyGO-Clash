# AGENTS.md — MyGO-Clash 协作须知

跨平台代理客户端：mihomo 内核（独立 core 进程，配置仅内存传递）+ mygo 框架 GUI（React 19，15 种语言）+ mygo ui 包绘制的原生托盘快捷面板 + Tailscale + WebDAV 端到端加密同步。GPL-3.0，公开仓库。

## 构建与环境
- 所有构建走 package.json 脚本：bun run dev / build / generate / typecheck（已带 -tags=with_gvisor），不要手动拼命令
- go.mod 要求 go 1.27.1；go test ./... 与 tsc --noEmit 必须全过再交付
- dev 调试：MYGO_CLASH_KEYRING=file MYGO_CLASH_DEBUG=127.0.0.1:47391 bun run dev；调试端口 /capture 截图、/eval 执行 JS；代理页测试节点显示"超时"是正常的（假节点）

## 发布流程
- 版本号在 mygo.json，发版时与 tag 保持一致；发布 = workflow_dispatch（输入 tag；notes_only=true 则只重渲染已有版本的说明，不构建）
- 发布说明 = CHANGELOG.md 对应版本小节 + release-notes.sh（按发布页实际上传文件生成下载链接）+ release-template.md 的 FAQ；这段小节同时是应用内更新窗口的文案
- GitHub 上传会改名（空格和 ~ 变点），下载链接必须按实际文件名生成，禁止硬编码
- macOS 分架构 DMG（-arm64 / -x64）：mygo 的 DMG 文件名不含架构，workflow 里有重命名步骤，不要删；更新清单按架构自动生成，应用各自取用
- CHANGELOG 每条中英对照（中文在前、英文在后）

## 更新签名（重要）
- Ed25519 密钥对由 `mygo keygen` 生成：公钥在 mygo.json 的 updates.publicKey，私钥在 GitHub secret MYGO_UPDATER_PRIVATE_KEY
- 私钥丢失 = 已安装应用永远拒绝后续更新；轮换流程：重新 keygen → 改 mygo.json 公钥 → 重设 secret。未配私钥时构建只跳过签名并警告，不报错

## 协作偏好（用户明确要求）
- 不要请求或等待 Codex / 自动 PR 审查；PR 就绪即可请用户合并
- 设计类任务用户只给方向，细节自行联网调研并决策；MyGO!!!!! 官方物料实测色：主蓝 #0B88BB、闪光黄 #FDFE88；logo 无官方罗盘图标，"指南针"属粉丝考察
- 审美/文案改动交付前后截图对比（明暗主题都要）；功能对位参考 clash-verge-rev
