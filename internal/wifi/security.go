package wifi

import "strings"

// Weak reports whether a network's security is weak (Open, WEP or WPA1)
// and why, for showing a warning. It returns false and "" otherwise,
// including for Unknown.
func Weak(s Security) (bool, string) {
	switch s {
	case SecOpen:
		return true, "Open network: traffic over the air isn't encrypted, so anyone nearby can read it."
	case SecWEP:
		return true, "WEP encryption is broken and can be cracked in minutes; switch the router to WPA2 or WPA3."
	case SecWPA:
		return true, "WPA (TKIP) is deprecated and has known attacks; switch the router to WPA2 or WPA3."
	}
	return false, ""
}

// secFlags collects what a tool said about a network's security before it
// is reduced to one Security value.
type secFlags struct {
	open       bool
	wep        bool
	wpa1       bool
	wpa2       bool
	wpa3       bool // SAE, or OWE ("Enhanced Open", encrypted without a password)
	enterprise bool
}

func (f secFlags) security() Security {
	switch {
	case f.enterprise:
		return SecEnterprise
	case f.wpa3 && f.wpa2:
		return SecWPA2WPA3
	case f.wpa3:
		return SecWPA3
	case f.wpa2:
		return SecWPA2 // WPA/WPA2 mixed mode counts as WPA2
	case f.wpa1:
		return SecWPA
	case f.wep:
		return SecWEP
	case f.open:
		return SecOpen
	}
	return SecUnknown
}

// securityFromText classifies the free-form security descriptions most
// tools print, such as nmcli's "WPA1 WPA2 802.1X", netsh's "WPA2-Personal",
// macOS's "spairport_security_mode_wpa3_transition" or airport's
// "WPA2(PSK/AES/AES)". An empty string or "--" means open.
func securityFromText(s string) Security {
	t := strings.ToUpper(strings.TrimSpace(s))
	if t == "" || t == "--" {
		return SecOpen
	}
	return flagsFromText(t).security()
}

func flagsFromText(t string) secFlags {
	var f secFlags
	t = strings.ToUpper(t)
	t = strings.TrimPrefix(t, "SPAIRPORT_SECURITY_MODE_")
	t = strings.TrimPrefix(t, "PAIRPORT_SECURITY_MODE_")
	words := strings.FieldsFunc(t, func(r rune) bool {
		return !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.')
	})
	has := func(w string) bool {
		for _, x := range words {
			if x == w {
				return true
			}
		}
		return false
	}
	if has("NONE") || has("OPEN") {
		f.open = true
	}
	if has("WEP") || has("SHARED") {
		f.wep = true
	}
	if has("WPA") || has("WPA1") {
		f.wpa1 = true
	}
	if has("WPA2") || has("RSN") {
		f.wpa2 = true
	}
	if has("WPA3") || has("SAE") || has("OWE") {
		f.wpa3 = true
		if has("TRANSITION") && !has("OWE") {
			f.wpa2 = true
		}
	}
	if has("802.1X") || has("8021X") || has("EAP") || has("ENTERPRISE") || has("SUITE") {
		f.enterprise = true
	}
	// "WPA2(PSK/...)" / "RSN-PSK" style: RSN with only SAE means WPA3.
	if f.wpa2 && has("SAE") && !has("PSK") && !has("WPA2") {
		f.wpa2 = false
	}
	return f
}

// securityFromAndroidCaps classifies an Android ScanResult capabilities
// string such as "[WPA2-PSK-CCMP][RSN-PSK+SAE-CCMP][ESS]".
func securityFromAndroidCaps(caps string) Security {
	caps = strings.ToUpper(caps)
	var f secFlags
	for _, part := range strings.Split(caps, "[") {
		part = strings.TrimSuffix(strings.TrimSpace(part), "]")
		if part == "" {
			continue
		}
		fields := strings.Split(part, "-")
		proto := fields[0]
		auth := ""
		if len(fields) > 1 {
			auth = fields[1]
		}
		switch proto {
		case "WEP":
			f.wep = true
		case "WPA":
			if strings.Contains(auth, "EAP") {
				f.enterprise = true
			} else {
				f.wpa1 = true
			}
		case "WPA2", "RSN":
			for _, a := range strings.Split(auth, "+") {
				switch {
				case strings.Contains(a, "EAP"), strings.Contains(a, "SUITE"):
					f.enterprise = true
				case strings.Contains(a, "SAE"), a == "OWE":
					f.wpa3 = true
				case strings.Contains(a, "PSK"):
					f.wpa2 = true
				}
			}
		case "WPA3":
			if strings.Contains(auth, "EAP") || strings.Contains(auth, "SUITE") {
				f.enterprise = true
			} else {
				f.wpa3 = true
			}
		case "OWE", "OWE_TRANSITION":
			f.wpa3 = true
		case "EAP", "EAP_SUITE_B_192", "IEEE8021X":
			f.enterprise = true
		}
	}
	if sec := f.security(); sec != SecUnknown {
		return sec
	}
	return SecOpen // only [ESS], [BSS], [WPS] and the like: no encryption
}

// securityFromAndroidType maps WifiInfo.getCurrentSecurityType (Android 12
// and later); -1 or an unknown value returns SecUnknown.
func securityFromAndroidType(t int) Security {
	switch t {
	case 0: // SECURITY_TYPE_OPEN
		return SecOpen
	case 1: // WEP
		return SecWEP
	case 2: // PSK
		return SecWPA2
	case 3, 5, 9, 10, 11, 12: // EAP, EAP_WPA3_ENTERPRISE_192_BIT, EAP_WPA3_ENTERPRISE, OSEN, PASSPOINT_*
		return SecEnterprise
	case 4, 6, 13: // SAE, OWE, DPP
		return SecWPA3
	}
	return SecUnknown
}
