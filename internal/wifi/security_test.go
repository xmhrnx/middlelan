package wifi

import "testing"

func TestSecurityFromText(t *testing.T) {
	for in, want := range map[string]Security{
		"":                                      SecOpen,
		"--":                                    SecOpen,
		"WPA2":                                  SecWPA2,
		"WPA1 WPA2":                             SecWPA2,
		"WPA1":                                  SecWPA,
		"WPA3":                                  SecWPA3,
		"WPA2 WPA3":                             SecWPA2WPA3,
		"WPA2 802.1X":                           SecEnterprise,
		"WEP":                                   SecWEP,
		"OWE":                                   SecWPA3,
		"WPA2-Personal":                         SecWPA2,
		"WPA-Personal":                          SecWPA,
		"WPA3-Personal":                         SecWPA3,
		"WPA2-Enterprise":                       SecEnterprise,
		"Shared":                                SecWEP,
		"NONE":                                  SecOpen,
		"WPA2(PSK/AES/AES)":                     SecWPA2,
		"WPA2(802.1x/AES/AES)":                  SecEnterprise,
		"RSN(SAE/AES/AES)":                      SecWPA3,
		"spairport_security_mode_wpa2_personal": SecWPA2,
		"spairport_security_mode_wpa3_transition": SecWPA2WPA3,
		"spairport_security_mode_wpa3_personal":   SecWPA3,
		"spairport_security_mode_none":            SecOpen,
		"spairport_security_mode_wep":             SecWEP,
		"spairport_security_mode_wpa_personal":    SecWPA,
		"spairport_security_mode_wpa2_enterprise": SecEnterprise,
		"something new":                           SecUnknown,
	} {
		if got := securityFromText(in); got != want {
			t.Errorf("securityFromText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSecurityFromAndroidCaps(t *testing.T) {
	for in, want := range map[string]Security{
		"[WPA2-PSK-CCMP][RSN-PSK-CCMP][ESS]":           SecWPA2,
		"[RSN-PSK+SAE-CCMP][ESS][MFPC]":                SecWPA2WPA3,
		"[RSN-SAE-CCMP][ESS][MFPR][MFPC]":              SecWPA3,
		"[WPA-PSK-TKIP][ESS]":                          SecWPA,
		"[WPA-PSK-TKIP+CCMP][WPA2-PSK-TKIP+CCMP][ESS]": SecWPA2,
		"[WPA2-EAP/SHA1-CCMP][RSN-EAP/SHA1-CCMP][ESS]": SecEnterprise,
		"[WEP][ESS]":                      SecWEP,
		"[ESS]":                           SecOpen,
		"[ESS][WPS]":                      SecOpen,
		"":                                SecOpen,
		"[RSN-OWE-CCMP][ESS][MFPR]":       SecWPA3,
		"[RSN-SUITE_B_192-GCMP-256][ESS]": SecEnterprise,
	} {
		if got := securityFromAndroidCaps(in); got != want {
			t.Errorf("securityFromAndroidCaps(%q) = %q, want %q", in, got, want)
		}
	}
	if got := securityFromAndroidType(4); got != SecWPA3 {
		t.Errorf("type 4 = %q", got)
	}
	if got := securityFromAndroidType(-1); got != SecUnknown {
		t.Errorf("type -1 = %q", got)
	}
}

func TestWeak(t *testing.T) {
	for s, weak := range map[Security]bool{
		SecOpen: true, SecWEP: true, SecWPA: true,
		SecWPA2: false, SecWPA3: false, SecWPA2WPA3: false, SecEnterprise: false, SecUnknown: false,
	} {
		got, why := Weak(s)
		if got != weak || (why != "") != weak {
			t.Errorf("Weak(%q) = %v, %q", s, got, why)
		}
	}
}
