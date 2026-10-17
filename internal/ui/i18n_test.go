package ui

import (
	"testing"

	"github.com/ivan-krapivtsev/chilltransfer/internal/engine"
)

// TestTablesCoverAllKeys asserts both language tables define every UI key with
// a non-empty value and contain no orphan entries beyond allKeys; allKeys must
// have no duplicates.
func TestTablesCoverAllKeys(t *testing.T) {
	listed := map[string]bool{}
	for _, k := range allKeys {
		if listed[k] {
			t.Errorf("duplicate key %q in allKeys", k)
		}
		listed[k] = true
	}
	tables := map[string]map[string]string{"ru": ruStrings, "en": enStrings}
	for name, table := range tables {
		for _, k := range allKeys {
			if v, ok := table[k]; !ok || v == "" {
				t.Errorf("%s: key %q missing or empty", name, k)
			}
		}
		for k := range table {
			if !listed[k] {
				t.Errorf("%s: table entry %q not listed in allKeys", name, k)
			}
		}
	}
}

// TestDetectLanguage verifies locale parsing: LC_ALL takes precedence over
// LANG, a "ru" prefix selects Russian (case-insensitively), and anything else
// — including empty values — falls back to English.
func TestDetectLanguage(t *testing.T) {
	cases := []struct {
		name  string
		lcAll string
		lang  string
		want  string
	}{
		{"ru LC_ALL", "ru_RU.UTF-8", "", LangRU},
		{"LC_ALL wins over LANG", "en_US.UTF-8", "ru_RU.UTF-8", LangEN},
		{"ru LANG", "", "ru_RU.UTF-8", LangRU},
		{"uppercase RU prefix", "", "RU_ru", LangRU},
		{"en LANG", "", "en_US", LangEN},
		{"empty values fall back to EN", "", "", LangEN},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LC_ALL", tc.lcAll)
			t.Setenv("LANG", tc.lang)
			if got := DetectLanguage(); got != tc.want {
				t.Errorf("DetectLanguage() = %q, want %q (LC_ALL=%q LANG=%q)", got, tc.want, tc.lcAll, tc.lang)
			}
		})
	}
}

// TestTFailSoft verifies T returns the key itself for unknown keys (including
// on a nil receiver), resolves known keys per language, and that NewI18N falls
// back to English for unknown language codes.
func TestTFailSoft(t *testing.T) {
	en := NewI18N(LangEN)
	if got := en.T("no.such.key"); got != "no.such.key" {
		t.Errorf("T(unknown key) = %q, want the key itself", got)
	}
	if got := (*I18N)(nil).T("any.key"); got != "any.key" {
		t.Errorf("nil T(key) = %q, want the key itself", got)
	}

	cases := []struct {
		i    *I18N
		key  string
		want string
	}{
		{NewI18N(LangRU), KeyBtnStart, "Старт"},
		{NewI18N(LangEN), KeyBtnStart, "Start"},
		{NewI18N(LangRU), KeyDropZone, "Перетащите файлы и папки сюда"},
		{NewI18N(LangEN), KeyDropZone, "Drop files and folders here"},
	}
	for _, tc := range cases {
		if got := tc.i.T(tc.key); got != tc.want {
			t.Errorf("%s T(%q) = %q, want %q", tc.i.Lang(), tc.key, got, tc.want)
		}
	}

	if got := NewI18N("de").T(KeyBtnStart); got != "Start" {
		t.Errorf("NewI18N(unknown lang).T = %q, want English fallback", got)
	}
	if got := NewI18N("RU").Lang(); got != LangRU {
		t.Errorf("NewI18N(\"RU\").Lang() = %q, want case-insensitive RU match", got)
	}
}

// TestStateText verifies engine states map to localized names and unknown
// states keep the engine identifier.
func TestStateText(t *testing.T) {
	cases := []struct {
		state engine.State
		ru    string
		en    string
	}{
		{engine.StateIdle, "ожидание", "idle"},
		{engine.StateCopying, "копирование", "copying"},
		{engine.StatePausedThermal, "тепловая пауза", "thermal pause"},
		{engine.StateResting, "отдых", "resting"},
		{engine.StateDone, "готово", "done"},
		{engine.StateError, "ошибка", "error"},
		{engine.StateCancelled, "отменено", "cancelled"},
	}
	for _, tc := range cases {
		s := tc.state
		if got := NewI18N(LangRU).StateText(s); got != tc.ru {
			t.Errorf("ru StateText(%v) = %q, want %q", s, got, tc.ru)
		}
		if got := NewI18N(LangEN).StateText(s); got != tc.en {
			t.Errorf("en StateText(%v) = %q, want %q", s, got, tc.en)
		}
	}

	unknown := engine.State(99)
	if got := NewI18N(LangEN).StateText(unknown); got != unknown.String() {
		t.Errorf("StateText(unknown) = %q, want engine identifier %q", got, unknown.String())
	}
}
