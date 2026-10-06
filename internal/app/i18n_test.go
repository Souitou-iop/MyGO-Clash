package app

import (
	"regexp"
	"slices"
	"testing"
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
