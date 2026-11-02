package ui

import (
	"testing"

	"github.com/xmhrnx/giode/i18n"
	"github.com/xmhrnx/middlelan/locales"
)

func TestTranslations(t *testing.T) {
	if err := i18n.Load(locales.FS, "."); err != nil {
		t.Fatal(err)
	}
	defer i18n.SetLocale("en")
	check := func(locale string, f func() string, want string) {
		t.Helper()
		i18n.SetLocale(locale)
		if got := f(); got != want {
			t.Errorf("%s: got %q, want %q", locale, got, want)
		}
	}
	check("en", func() string { return plural(1, "connection", "connections") }, "1 connection")
	check("en", func() string { return plural(3, "connection", "connections") }, "3 connections")
	check("en", func() string { return trn("{count} programs use the network", 1) }, "1 program uses the network")
	check("es", func() string { return plural(1, "connection", "connections") }, "1 conexión")
	check("es", func() string { return plural(4, "connection", "connections") }, "4 conexiones")
	check("de", func() string { return tr("Wake-up packet sent to {device}", "device", "nas") }, "Weckpaket an nas gesendet")
	check("fr", func() string { return plural(0, "check", "checks") }, "0 vérification")
	check("fa", func() string { return tr("Settings") }, "تنظیمات")
	check("es", func() string { return tr("Not a message") }, "Not a message")
	check("fa", func() string { return arrowForward() }, " ← ")
}
