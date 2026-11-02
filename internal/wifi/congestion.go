package wifi

import (
	"cmp"
	"slices"
)

// ChannelUse is how busy one channel is: how many networks use it and the
// strongest signal among them.
type ChannelUse struct {
	Band         string
	Channel      int
	Networks     int
	StrongestDBm int
}

// Candidate channels Congestion may suggest: the non-overlapping 2.4 GHz
// channels, the 5 GHz channels outside DFS (radar) ranges, and the 6 GHz
// preferred scanning channels.
var candidates = map[string][]int{
	"2.4 GHz": {1, 6, 11},
	"5 GHz":   {36, 40, 44, 48, 149, 153, 157, 161, 165},
	"6 GHz":   {5, 21, 37, 53, 69, 85, 101, 117, 133, 149, 165, 181, 197, 213, 229},
}

var bandOrder = map[string]int{"2.4 GHz": 0, "5 GHz": 1, "6 GHz": 2}

// Congestion summarizes channel use: for each band and channel, how many
// networks use it and their strongest signal, sorted by band then channel.
// best maps each band seen in nets to the least crowded candidate channel,
// counting overlapping neighbours on 2.4 GHz (channels within 4 of each
// other interfere) and breaking ties by the weaker strongest signal, then
// the lower channel. Networks without a known band or channel are ignored.
func Congestion(nets []Network) (uses []ChannelUse, best map[string]int) {
	type key struct {
		band string
		ch   int
	}
	idx := map[key]int{}
	for _, n := range nets {
		band, ch := n.Band, n.Channel
		if band == "" {
			band = BandOf(n.FrequencyMHz)
		}
		if ch == 0 {
			ch = ChannelFromFreq(n.FrequencyMHz)
		}
		if band == "" || ch == 0 {
			continue
		}
		dbm := n.SignalDBm
		if dbm == 0 {
			dbm = DBmFromQuality(n.Quality)
		}
		k := key{band, ch}
		i, ok := idx[k]
		if !ok {
			i = len(uses)
			idx[k] = i
			uses = append(uses, ChannelUse{Band: band, Channel: ch, StrongestDBm: dbm})
		}
		uses[i].Networks++
		uses[i].StrongestDBm = max(uses[i].StrongestDBm, dbm)
	}
	slices.SortFunc(uses, func(a, b ChannelUse) int {
		if c := cmp.Compare(bandRank(a.Band), bandRank(b.Band)); c != 0 {
			return c
		}
		return cmp.Compare(a.Channel, b.Channel)
	})

	best = map[string]int{}
	for band := range bandsIn(uses) {
		type score struct{ ch, nets, strongest int }
		var top *score
		for _, ch := range candidates[band] {
			s := score{ch: ch, strongest: -200}
			for _, u := range uses {
				if u.Band != band || !overlaps(band, u.Channel, ch) {
					continue
				}
				s.nets += u.Networks
				s.strongest = max(s.strongest, u.StrongestDBm)
			}
			if top == nil || s.nets < top.nets || s.nets == top.nets && s.strongest < top.strongest {
				top = &s
			}
		}
		if top != nil {
			best[band] = top.ch
		}
	}
	return uses, best
}

func bandRank(b string) int {
	if r, ok := bandOrder[b]; ok {
		return r
	}
	return len(bandOrder)
}

func bandsIn(uses []ChannelUse) map[string]bool {
	m := map[string]bool{}
	for _, u := range uses {
		m[u.Band] = true
	}
	return m
}

// overlaps reports whether a network on channel a interferes with channel
// b. 2.4 GHz channels are 5 MHz apart but 20 MHz wide; elsewhere only the
// same primary channel is counted, since channel widths aren't known.
func overlaps(band string, a, b int) bool {
	if band == "2.4 GHz" {
		d := a - b
		return d > -5 && d < 5
	}
	return a == b
}
