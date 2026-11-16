package app

import "testing"

func TestReleaseNotesIn(t *testing.T) {
	body := "修复版：不再截断。/ A fix release: nothing is cut.\n\n### 🐛 修复 / Fixes\n\n- 规则“直连/走代理”可添加 (#15) / Rules “direct / proxy” can be added (#15)\n- **原生**窗口 / A **native** window\n- plain line"
	zh := "修复版：不再截断。\n\n### 修复\n\n- 规则“直连/走代理”可添加 (#15)\n- **原生**窗口\n- plain line"
	en := "A fix release: nothing is cut.\n\n### Fixes\n\n- Rules “direct / proxy” can be added (#15)\n- A **native** window\n- plain line"
	if got := releaseNotesIn(body, true); got != zh {
		t.Errorf("zh:\n%s\nwant:\n%s", got, zh)
	}
	if got := releaseNotesIn(body, false); got != en {
		t.Errorf("en:\n%s\nwant:\n%s", got, en)
	}
}

func TestCleanReleaseBodyDropsRuleAndDownloads(t *testing.T) {
	got := cleanReleaseBody("说明 / Notes\n\n---\n\n## 📥 下载地址 / Downloads\n\n- a.dmg")
	if got != "说明 / Notes" {
		t.Errorf("got %q", got)
	}
}

func TestReleaseVersionAtLeast(t *testing.T) {
	for _, test := range []struct {
		tag, minimum string
		want         bool
	}{
		{tag: "v1.0.0", minimum: "v1.0.0", want: true},
		{tag: "1.0.5", minimum: "v1.0.0", want: true},
		{tag: "v0.2.32-beta", minimum: "v1.0.0", want: false},
		{tag: "stable", minimum: "v1.0.0", want: false},
	} {
		if got := releaseVersionAtLeast(test.tag, test.minimum); got != test.want {
			t.Errorf("releaseVersionAtLeast(%q, %q) = %v, want %v", test.tag, test.minimum, got, test.want)
		}
	}
}
