package geo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// flexFloat accepts a JSON number or a number in a string, as geojs sends.
type flexFloat float64

func (f *flexFloat) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" || s == "nil" {
		*f = 0
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return err
	}
	*f = flexFloat(v)
	return nil
}

type geojsRecord struct {
	IP          string    `json:"ip"`
	Country     string    `json:"country"`
	CountryCode string    `json:"country_code"`
	Region      string    `json:"region"`
	City        string    `json:"city"`
	OrgName     string    `json:"organization_name"`
	ASN         flexFloat `json:"asn"`
	Lat         flexFloat `json:"latitude"`
	Lon         flexFloat `json:"longitude"`
}

var httpClient = &http.Client{Timeout: 15 * time.Second}

// lookupBatch locates up to 50 public addresses in one HTTPS request to
// geojs.io, a free service without an API key.
func lookupBatch(ctx context.Context, ips []string) ([]Info, error) {
	u := "https://get.geojs.io/v1/ip/geo.json?ip=" + url.QueryEscape(strings.Join(ips, ","))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "MiddleMan")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("geojs: %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	return parseGeojs(body)
}

func parseGeojs(body []byte) ([]Info, error) {
	var recs []geojsRecord
	if err := json.Unmarshal(body, &recs); err != nil {
		var one geojsRecord
		if err1 := json.Unmarshal(body, &one); err1 != nil {
			return nil, fmt.Errorf("geojs: %w", err)
		}
		recs = []geojsRecord{one}
	}
	out := make([]Info, 0, len(recs))
	for _, r := range recs {
		out = append(out, Info{
			IP: r.IP, Country: r.Country, CountryCode: r.CountryCode, Region: r.Region, City: r.City,
			Org: titleOrg(r.OrgName), ASN: int(r.ASN), Lat: float64(r.Lat), Lon: float64(r.Lon),
			Scope: ScopePublic, Located: r.Country != "" || r.Lat != 0 || r.Lon != 0,
		})
	}
	return out, nil
}

// titleOrg turns "GOOGLE-CLOUD-PLATFORM" into "Google Cloud Platform".
func titleOrg(s string) string {
	if s == "" || strings.ToUpper(s) != s {
		return s
	}
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return r == '-' || r == '_' || r == ' ' })
	for i, w := range words {
		if len(w) <= 3 && (w == "llc" || w == "inc" || w == "isp" || w == "aws") {
			words[i] = strings.ToUpper(w)
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}
