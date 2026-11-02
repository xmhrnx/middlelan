package nettools

import "strings"

// countryNames maps ISO 3166-1 alpha-2 codes to English short names.
var countryNames = func() map[string]string {
	const data = "AD:Andorra|AE:United Arab Emirates|AF:Afghanistan|AL:Albania|AM:Armenia|AO:Angola|AR:Argentina|AT:Austria|AU:Australia|AZ:Azerbaijan|" +
		"BA:Bosnia and Herzegovina|BD:Bangladesh|BE:Belgium|BG:Bulgaria|BH:Bahrain|BO:Bolivia|BR:Brazil|BY:Belarus|CA:Canada|CH:Switzerland|" +
		"CL:Chile|CN:China|CO:Colombia|CR:Costa Rica|CU:Cuba|CY:Cyprus|CZ:Czechia|DE:Germany|DK:Denmark|DO:Dominican Republic|DZ:Algeria|" +
		"EC:Ecuador|EE:Estonia|EG:Egypt|ES:Spain|ET:Ethiopia|FI:Finland|FR:France|GB:United Kingdom|GE:Georgia|GH:Ghana|GR:Greece|GT:Guatemala|" +
		"HK:Hong Kong|HN:Honduras|HR:Croatia|HU:Hungary|ID:Indonesia|IE:Ireland|IL:Israel|IN:India|IQ:Iraq|IR:Iran|IS:Iceland|IT:Italy|" +
		"JM:Jamaica|JO:Jordan|JP:Japan|KE:Kenya|KG:Kyrgyzstan|KH:Cambodia|KR:South Korea|KW:Kuwait|KZ:Kazakhstan|LA:Laos|LB:Lebanon|LK:Sri Lanka|" +
		"LT:Lithuania|LU:Luxembourg|LV:Latvia|LY:Libya|MA:Morocco|MD:Moldova|ME:Montenegro|MK:North Macedonia|MM:Myanmar|MN:Mongolia|MO:Macao|" +
		"MT:Malta|MX:Mexico|MY:Malaysia|NG:Nigeria|NI:Nicaragua|NL:Netherlands|NO:Norway|NP:Nepal|NZ:New Zealand|OM:Oman|PA:Panama|PE:Peru|" +
		"PH:Philippines|PK:Pakistan|PL:Poland|PR:Puerto Rico|PS:Palestine|PT:Portugal|PY:Paraguay|QA:Qatar|RO:Romania|RS:Serbia|RU:Russia|" +
		"SA:Saudi Arabia|SD:Sudan|SE:Sweden|SG:Singapore|SI:Slovenia|SK:Slovakia|SN:Senegal|SV:El Salvador|SY:Syria|TH:Thailand|TJ:Tajikistan|" +
		"TM:Turkmenistan|TN:Tunisia|TR:Turkey|TW:Taiwan|TZ:Tanzania|UA:Ukraine|UG:Uganda|US:United States|UY:Uruguay|UZ:Uzbekistan|" +
		"VE:Venezuela|VN:Vietnam|YE:Yemen|ZA:South Africa|ZM:Zambia|ZW:Zimbabwe"
	m := make(map[string]string)
	for _, e := range strings.Split(data, "|") {
		if k, v, ok := strings.Cut(e, ":"); ok {
			m[k] = v
		}
	}
	return m
}()

// countryName returns the English name for an ISO country code, or the code
// itself when unknown.
func countryName(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if n, ok := countryNames[code]; ok {
		return n
	}
	return code
}
