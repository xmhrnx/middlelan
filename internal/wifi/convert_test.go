package wifi

import "testing"

func TestQualityFromDBm(t *testing.T) {
	for _, c := range []struct{ dbm, q int }{
		{-30, 100}, {-50, 100}, {-51, 98}, {-75, 50}, {-99, 2}, {-100, 0}, {-110, 0}, {0, 0},
	} {
		if got := QualityFromDBm(c.dbm); got != c.q {
			t.Errorf("QualityFromDBm(%d) = %d, want %d", c.dbm, got, c.q)
		}
	}
}

func TestDBmFromQuality(t *testing.T) {
	for _, c := range []struct{ q, dbm int }{{100, -50}, {50, -75}, {0, -100}, {150, -50}, {-5, -100}} {
		if got := DBmFromQuality(c.q); got != c.dbm {
			t.Errorf("DBmFromQuality(%d) = %d, want %d", c.q, got, c.dbm)
		}
	}
	for q := 0; q <= 100; q += 2 {
		if got := QualityFromDBm(DBmFromQuality(q)); got != q {
			t.Errorf("round trip of %d gave %d", q, got)
		}
	}
}

func TestChannelAndBand(t *testing.T) {
	for _, c := range []struct {
		mhz, ch int
		band    string
	}{
		{2412, 1, "2.4 GHz"}, {2437, 6, "2.4 GHz"}, {2472, 13, "2.4 GHz"}, {2484, 14, "2.4 GHz"},
		{5180, 36, "5 GHz"}, {5500, 100, "5 GHz"}, {5745, 149, "5 GHz"}, {5885, 177, "5 GHz"},
		{5935, 2, "6 GHz"}, {5955, 1, "6 GHz"}, {6135, 37, "6 GHz"}, {7115, 233, "6 GHz"},
		{4920, 184, "5 GHz"}, {60480, 0, ""}, {0, 0, ""},
	} {
		if got := ChannelFromFreq(c.mhz); got != c.ch {
			t.Errorf("ChannelFromFreq(%d) = %d, want %d", c.mhz, got, c.ch)
		}
		if got := BandOf(c.mhz); got != c.band {
			t.Errorf("BandOf(%d) = %q, want %q", c.mhz, got, c.band)
		}
		if c.ch != 0 {
			if got := freqFromChannel(c.ch, c.band); got != c.mhz {
				t.Errorf("freqFromChannel(%d, %q) = %d, want %d", c.ch, c.band, got, c.mhz)
			}
		}
	}
	if got := freqFromChannel(149, ""); got != 5745 {
		t.Errorf("freqFromChannel(149, \"\") = %d", got)
	}
}

func TestNormMAC(t *testing.T) {
	for in, want := range map[string]string{
		"A4-2B-B0-11-22-33":  "a4:2b:b0:11:22:33",
		"02:00:00:00:00:00":  "",
		"garbage":            "",
		" aa:bb:cc:dd:ee:ff": "aa:bb:cc:dd:ee:ff",
	} {
		if got := normMAC(in); got != want {
			t.Errorf("normMAC(%q) = %q, want %q", in, got, want)
		}
	}
}
