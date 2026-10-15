package app

import (
	"regexp"
	"slices"
	"testing"

	"github.com/egoist/mygo/i18n"
)

var verbRe = regexp.MustCompile(`%[a-z]`)

func TestTranslations(t *testing.T) {
	for _, l := range languages {
		if l == "en" {
			continue
		}
		tr, ok := translations[l]
		if !ok {
			t.Errorf("no translations in %s", l)
			continue
		}
		for key, en := range messages {
			s, ok := tr[key]
			if !ok || s == "" {
				t.Errorf("%s: no %q", l, key)
				continue
			}
			if want, got := verbRe.FindAllString(en, -1), verbRe.FindAllString(s, -1); !slices.Equal(want, got) {
				t.Errorf("%s: %q has verbs %v, want %v", l, key, got, want)
			}
		}
		for key := range tr {
			if _, ok := messages[key]; !ok {
				t.Errorf("%s: %q is not a message", l, key)
			}
		}
	}
	for l := range translations {
		if !slices.Contains(languages, l) {
			t.Errorf("translations in %s, which is not a language", l)
		}
	}
}

func TestMatchLanguage(t *testing.T) {
	for in, want := range map[string]string{
		"":            "en",
		"en-US":       "en",
		"zh":          "zh-CN",
		"zh-CN":       "zh-CN",
		"zh_CN.UTF-8": "zh-CN",
		"zh-Hans-HK":  "zh-CN",
		"zh-TW":       "zh-TW",
		"zh-Hant":     "zh-TW",
		"zh_HK":       "zh-TW",
		"ja_JP":       "ja",
		"pt-PT":       "pt-BR",
		"pt-BR":       "pt-BR",
		"fa-IR":       "fa",
		"ar":          "ar",
		"nb-NO":       "en",
		"C":           "en",
	} {
		if got := matchLanguage(in); got != want {
			t.Errorf("matchLanguage(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDelayKind(t *testing.T) {
	for d, want := range map[int]delayKind{-1: delayNone, 0: delayBad, 1: delayGood, 199: delayGood, 200: delayOK, 599: delayOK, 600: delayBad} {
		if got := delayKindOf(d); got != want {
			t.Errorf("delayKindOf(%d) = %v, want %v", d, got, want)
		}
	}
}

// TestSystemLanguages checks that the system's languages reach the
// interface's through MyGo's matching (App.Locale), each of them.
func TestSystemLanguages(t *testing.T) {
	var reached []string
	for _, l := range systemLanguages {
		reached = append(reached, matchLanguage(l))
	}
	if !slices.Equal(reached, languages) {
		t.Errorf("systemLanguages reach %v, want %v", reached, languages)
	}
	for _, tc := range []struct {
		preferred []string
		want      string
	}{
		{[]string{"zh-Hans-CN"}, "zh-CN"},
		{[]string{"zh-HK"}, "zh-TW"},
		{[]string{"zh-Hant-TW"}, "zh-TW"},
		{[]string{"pt-PT"}, "pt-BR"},
		{[]string{"it-IT", "ja-JP"}, "ja"},
		{[]string{"it-IT"}, "en"},
	} {
		if got := matchLanguage(i18n.Match(tc.preferred, systemLanguages)); got != tc.want {
			t.Errorf("%v: %s, want %s", tc.preferred, got, tc.want)
		}
	}
}
