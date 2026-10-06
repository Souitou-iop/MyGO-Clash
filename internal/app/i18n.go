package app

import (
	"strings"

	"github.com/egoist/mygo"
)

// strings of the Go side: the tray, the quick panel, notifications and
// errors. The page has its own.
var messages = map[string][2]string{ // key: {en, zh}
	"importing":       {"Importing profile…", "正在导入订阅…"},
	"imported":        {"Profile imported", "订阅已导入"},
	"importFailed":    {"Could not import the profile", "导入订阅失败"},
	"importFile":      {"Import a profile", "导入配置文件"},
	"serviceFallback": {"The service could not start the core; running it without privileges (no TUN)", "服务无法启动内核，已改为普通模式运行（TUN 不可用）"},
	"coreStartFailed": {"The core did not start", "内核启动失败"},
	"coreCrashed":     {"The core stopped unexpectedly; restarting it", "内核意外停止，正在重启"},
	"applyFailed":     {"Could not apply the configuration", "应用配置失败"},
	"updateFailed":    {"Could not update %s", "更新订阅 %s 失败"},
	"tunNeedsService": {"TUN mode needs the service: install it first", "TUN 模式需要先安装服务"},
	"modeFailed":      {"Could not switch the mode", "切换代理模式失败"},
	"sysproxyFailed":  {"Could not set the system proxy", "设置系统代理失败"},
	"guardStopped":    {"The system proxy guard stopped after repeated failures", "系统代理守卫连续失败，已停止"},
	"hotkeyFailed":    {"Could not register the shortcut", "无法注册快捷键"},
	"invalidProfile":  {"The profile is invalid", "配置文件无效"},
	"noRuntime":       {"No configuration runs yet", "当前没有运行中的配置"},
	"controllerOff":   {"Turn the external controller on first", "请先启用外部控制器"},
	"servicePrompt":   {"MyGO-Clash needs to install its service, which TUN mode requires.", "MyGO-Clash 需要安装服务以启用 TUN 模式。"},
	"uwpPrompt":       {"MyGO-Clash needs to let Store apps reach the proxy.", "MyGO-Clash 需要为 UWP 应用解除回环限制。"},
	"tsNoLoginURL":    {"Tailscale gave no login link; check the control server", "Tailscale 未返回登录链接，请检查控制服务器"},
	"tsOff":           {"Tailscale is off", "Tailscale 未启用"},
	"syncNotSetUp":    {"Sync is not set up", "尚未配置同步"},
	"syncBusy":        {"A sync is running", "正在同步"},
	"syncConflicts":   {"%d items changed on two devices; choose which to keep", "%d 项在多台设备上都有修改，请选择保留哪一份"},
	"syncFailed":      {"Sync failed", "同步失败"},
	"conflictFrom":    {"from", "来自"},

	// Tray and panel.
	"running":       {"Running", "运行中"},
	"stopped":       {"Stopped", "已停止"},
	"starting":      {"Starting…", "启动中…"},
	"error":         {"Error", "错误"},
	"dashboard":     {"Open Dashboard", "打开面板"},
	"quickPanel":    {"Quick Panel", "快捷面板"},
	"mode":          {"Mode", "代理模式"},
	"rule":          {"Rule", "规则"},
	"global":        {"Global", "全局"},
	"direct":        {"Direct", "直连"},
	"proxies":       {"Proxies", "代理组"},
	"profiles":      {"Profiles", "订阅"},
	"systemProxy":   {"System Proxy", "系统代理"},
	"tun":           {"TUN Mode", "虚拟网卡模式"},
	"tunNeeds":      {"TUN Mode (install service)", "虚拟网卡模式（需安装服务）"},
	"copyEnv":       {"Copy Proxy Command", "复制环境变量"},
	"openDir":       {"Open Folder", "打开目录"},
	"dataDir":       {"Data", "配置目录"},
	"coreDir":       {"Core", "内核目录"},
	"logsDir":       {"Logs", "日志目录"},
	"more":          {"More", "更多"},
	"restartCore":   {"Restart Core", "重启内核"},
	"reapply":       {"Reload Configuration", "重新加载配置"},
	"updateGeo":     {"Update GeoData", "更新 GeoData"},
	"lightweight":   {"Lightweight Mode", "轻量模式"},
	"restartApp":    {"Restart App", "重启应用"},
	"quit":          {"Quit", "退出"},
	"tailscale":     {"Tailscale", "Tailscale"},
	"exitNode":      {"Exit Node", "出口节点"},
	"none":          {"None", "无"},
	"copyIP":        {"Copy My Address", "复制本机地址"},
	"adminConsole":  {"Admin Console", "管理控制台"},
	"tsNeedsLogin":  {"Needs login", "需要登录"},
	"testDelay":     {"Test Delays", "测试延迟"},
	"noProfile":     {"No profile", "无订阅"},
	"upload":        {"Up", "上传"},
	"download":      {"Down", "下载"},
	"group":         {"Group", "代理组"},
	"profile":       {"Profile", "订阅"},
	"notRunning":    {"The core is not running", "内核未运行"},
	"settings":      {"Settings…", "设置…"},
	"connected":     {"Connected", "已连接"},
	"disconnected":  {"Not connected", "未连接"},
	"syncNow":       {"Sync Now", "立即同步"},
}

// lang returns "zh" or "en", from the settings or the system.
func lang(a *App) string {
	l := ""
	if a != nil && a.settings != nil {
		l = a.settings.Get().Language
	}
	if l == "" {
		l = mygo.App.Locale()
	}
	if strings.HasPrefix(strings.ToLower(l), "zh") {
		return "zh"
	}
	return "en"
}

// tr translates a message of the Go side.
func tr(a *App, key string) string {
	m, ok := messages[key]
	if !ok {
		return key
	}
	if lang(a) == "zh" {
		return m[1]
	}
	return m[0]
}
