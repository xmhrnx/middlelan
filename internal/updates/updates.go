// Package updates tells the user when a newer MiddleLAN is out. With a
// signed update feed configured (giode.toml [update]) desktop builds can
// download and install it; otherwise it points at the GitHub release.
package updates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mehran9675/giode/update"
)

// releasesAPI is the repository's latest-release endpoint.
const releasesAPI = "https://api.github.com/repos/xmhrnx/middlelan/releases/latest"

// Release is a published version.
type Release struct {
	Version string // "1.2.0"
	Notes   string
	URL     string // the release page
	// Installable is set when the signed feed offers this build, so
	// Install can replace the app in place.
	Installable bool
	feed        *update.Release
}

// Checker checks for and installs updates.
type Checker struct {
	Current string // the running version; "" (a development build) never updates
	client  *http.Client
	updater *update.Updater
}

// New creates a checker for the running version.
func New(current string) *Checker {
	u := update.New()
	if u.Version == "" {
		u.Version = current
	}
	return &Checker{Current: current, client: &http.Client{Timeout: 20 * time.Second}, updater: u}
}

// ErrDevBuild means the running build has no version to compare.
var ErrDevBuild = errors.New("development builds don't check for updates")

// Check returns the newer release, or nil when up to date.
func (c *Checker) Check(ctx context.Context) (*Release, error) {
	if c.Current == "" {
		return nil, ErrDevBuild
	}
	if c.updater.FeedURL != "" && c.updater.PublicKey != "" {
		r, err := c.updater.Check(ctx)
		switch {
		case err == nil && r == nil:
			return nil, nil
		case err == nil:
			return &Release{Version: r.Version, Notes: r.Notes, URL: "https://github.com/xmhrnx/middlelan/releases", Installable: true, feed: r}, nil
		case !errors.Is(err, update.ErrManaged):
			return nil, err
		}
		// Installed by a package manager or store: just tell the user.
	}
	return c.latestOnGitHub(ctx)
}

func (c *Checker) latestOnGitHub(ctx context.Context) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releasesAPI, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "MiddleLAN")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // nothing published yet
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("checking for updates: %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	return parseLatest(body, c.Current)
}

func parseLatest(body []byte, current string) (*Release, error) {
	var gh struct {
		Tag     string `json:"tag_name"`
		Body    string `json:"body"`
		HTMLURL string `json:"html_url"`
		Draft   bool   `json:"draft"`
		Pre     bool   `json:"prerelease"`
	}
	if err := json.Unmarshal(body, &gh); err != nil {
		return nil, err
	}
	v := normalize(gh.Tag)
	if gh.Draft || gh.Pre || v == "" || !update.Newer(v, normalize(current)) {
		return nil, nil
	}
	return &Release{Version: v, Notes: gh.Body, URL: gh.HTMLURL}, nil
}

// normalize turns "v1.2.3.45" (four-part tags carry the build number)
// into "1.2.3".
func normalize(tag string) string {
	v := strings.TrimPrefix(strings.TrimSpace(tag), "v")
	parts := strings.Split(v, ".")
	if len(parts) > 3 {
		parts = parts[:3]
	}
	return strings.Join(parts, ".")
}

// Install downloads an installable release and replaces the app; on
// success it restarts and doesn't return.
func (c *Checker) Install(ctx context.Context, r *Release, progress func(done, total int64)) error {
	if r == nil || !r.Installable || r.feed == nil {
		return errors.New("this release can't be installed automatically; download it from the release page")
	}
	dl, err := c.updater.Download(ctx, r.feed, progress)
	if err != nil {
		return err
	}
	return dl.Install()
}
