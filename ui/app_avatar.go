package ui

import (
	"hash/fnv"
	"strings"

	"github.com/xmhrnx/giode"
	props "github.com/xmhrnx/giode/styles/properties"
)

// avatarTones are muted, warm tones for program initials, in keeping
// with the gold and graphite palette; a name always gets the same one.
var avatarTones = []giode.Color{"#d9b36c", "#c9a27e", "#a9b48f", "#b98b7a", "#9fb0b8", "#c2b59b", "#b79aa6", "#8fa89a"}

// appAvatar is a program's initials on a tinted disc, size dp across.
func appAvatar(name string, size int) giode.Element {
	h := fnv.New32a()
	h.Write([]byte(strings.ToLower(name)))
	tone := avatarTones[h.Sum32()%uint32(len(avatarTones))]
	return giode.Avatar(name, giode.Styles{
		Width:      props.Width(size),
		Background: giode.Mix(tone, "theme.surface", 0.2),
		Color:      tone,
	})
}
