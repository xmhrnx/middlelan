package ui

import (
	"time"

	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/components/dropdown"
	"github.com/mehran9675/giode/components/input"
	"github.com/mehran9675/giode/components/segmented"
	"github.com/mehran9675/giode/components/toggle"
	"github.com/mehran9675/giode/i18n"
)

// The interface is translated with giode.Intl, the English text itself
// being the key: a message missing from locales/<lang>.json shows in
// English. Placeholders such as {name} are filled from name, value
// pairs; a {count} picks the plural form ("one", "other", ...) of
// messages that have them.

// tr translates s and fills its {placeholders} from name, value pairs.
func tr(s string, args ...any) string {
	return giode.Intl(s, args...)
}

// trn translates s, a message with plural forms, for count n; {count}
// in the message is n. English plural forms live in locales/en.json.
func trn(s string, n int, args ...any) string {
	return giode.Intl(s, append([]any{"count", n}, args...)...)
}

// msg marks English text in tables (navigation, tool catalog) that is
// translated with tr where it's shown.
func msg(s string) string { return s }

// weekdays are the short day names, Sunday first.
var weekdays = [...]string{msg("Sun"), msg("Mon"), msg("Tue"), msg("Wed"), msg("Thu"), msg("Fri"), msg("Sat")}

// weekdayShort is t's day of the week, abbreviated, in the language.
func weekdayShort(t time.Time) string { return tr(weekdays[t.Weekday()]) }

// arrowForward joins "from → to" pairs, pointing the reading direction.
func arrowForward() string {
	if i18n.RTL() {
		return " ← "
	}
	return " → "
}

// chevronBack and chevronForward name the chevron icons pointing back
// and forward in the reading direction.
func chevronBack() string {
	if i18n.RTL() {
		return "chevron-right"
	}
	return "chevron-left"
}

func chevronForward() string {
	if i18n.RTL() {
		return "chevron-left"
	}
	return "chevron-right"
}

// backButton is a ghost button with a back chevron, in the language.
func backButton(label string, onClick func()) *giode.ButtonElement {
	b := giode.Button(label, styleGhost).Variant(giode.VariantGhost).OnClick(onClick)
	localized(func() { b.Label(tr(label)).Icon(giode.Icon(chevronBack(), giode.Styles{Width: 16})) })
	return b
}

// placeName translates the geo package's placeholder for an address it
// couldn't locate; real places pass through.
func placeName(p string) string {
	if p == "Location unknown" {
		return tr("Location unknown")
	}
	return p
}

// ltr keeps a number and its unit ("25.8 Kbps") in that order inside
// right-to-left text: a left-to-right mark before the number binds it
// to the unit. Left to right, s is returned as it is.
func ltr(s string) string {
	if i18n.RTL() {
		return "\u200e" + s
	}
	return s
}

// Widgets built once keep the label they were given. localized runs a
// function that labels them now and again every time the language
// changes; syncLocale, called every frame, notices the change.
var (
	localeHooks []func()
	localeSeen  uint64
)

func localized(fn func()) {
	fn()
	localeHooks = append(localeHooks, fn)
}

func syncLocale() {
	v := i18n.Version()
	if v == localeSeen {
		return
	}
	localeSeen = v
	for _, fn := range localeHooks {
		fn()
	}
}

// buttonT is a button whose label follows the language.
func buttonT(label string, st giode.Styles) *giode.ButtonElement {
	b := giode.Button(label, st)
	localized(func() { b.Label(tr(label)) })
	return b
}

// textInputT is a text field whose placeholder follows the language.
func textInputT(placeholder string, st giode.Styles) *input.Input {
	in := giode.Input.Text(placeholder, st)
	localized(func() { in.Placeholder(tr(placeholder)) })
	return in
}

// segmentedT is a segmented control whose labels follow the language.
func segmentedT(labels ...string) *segmented.Control {
	c := giode.Input.Segmented(append([]string(nil), labels...)...)
	localized(func() { translateInto(c.Options(), labels) })
	return c
}

// selectT is a dropdown whose options follow the language; the
// selection stays on the same index.
func selectT(st giode.Styles, labels ...string) *dropdown.Select {
	s := giode.Input.Select(st, append([]string(nil), labels...)...)
	localized(func() { translateInto(s.Options(), labels) })
	return s
}

func translateInto(dst, labels []string) {
	for i := range dst {
		if i < len(labels) {
			dst[i] = tr(labels[i])
		}
	}
}

// switchT is a labelled switch whose label follows the language. A
// switch's label can't be changed, so a new language rebuilds it in
// place, keeping its state and onChange (which may be nil).
func switchT(label string, st giode.Styles, onChange func(bool)) *toggle.Switch {
	s := giode.Input.Switch(label, st)
	s.OnChange(onChange)
	first := true
	localized(func() {
		if first && tr(label) == label {
			first = false
			return
		}
		first = false
		on := s.Checked()
		fresh := giode.Input.Switch(tr(label), st)
		fresh.OnChange(onChange)
		fresh.SetChecked(on)
		*s = *fresh
	})
	return s
}

// languages are the interface languages, each named in itself; ""
// follows the system's language.
var languages = []struct{ code, name string }{
	{"", msg("System")}, {"en", "English"}, {"es", "Español"}, {"de", "Deutsch"},
	{"fr", "Français"}, {"pt", "Português"}, {"fa", "فارسی"},
}

// languagePicker chooses the interface language; onChange gets the
// chosen language's code.
func languagePicker(cur string, onChange func(code string)) *dropdown.Select {
	names := make([]string, len(languages))
	for i, l := range languages {
		names[i] = l.name
	}
	s := giode.Input.Select(styleField.With(giode.Styles{Width: 160}), names...)
	for i, l := range languages {
		if l.code == cur {
			s.SetSelected(i)
		}
	}
	localized(func() { s.Options()[0] = tr(languages[0].name) })
	s.OnChange(func(i int) { onChange(languages[i].code) })
	return s
}

// applyLanguage switches the interface language and redraws it; ""
// returns to the system's language.
func applyLanguage(code string) {
	if code == "" {
		code = i18n.SystemLocale()
	}
	giode.SetLocale(code)
}
