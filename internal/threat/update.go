package threat

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"time"
)

const (
	// maxDownload bounds a single list download.
	maxDownload = 20 << 20
	// downloadTimeout bounds a single list download, start to finish.
	downloadTimeout = 30 * time.Second
)

func newClient() *http.Client {
	return &http.Client{CheckRedirect: httpsOnlyRedirect}
}

// httpsOnlyRedirect follows a few redirects, none of them to plain HTTP.
func httpsOnlyRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return errors.New("too many redirects")
	}
	if req.URL.Scheme != "https" {
		return fmt.Errorf("refusing redirect to %s", req.URL.Scheme)
	}
	return nil
}

// Update downloads every list, sending If-None-Match / If-Modified-Since
// so unchanged lists cost almost nothing. Each new list is validated,
// saved atomically under dir/threat and swapped into the snapshot as soon
// as it is ready. A list that fails to download or validate keeps its
// previous copy and reports the problem in ListInfo.Err. The returned
// error joins the failures; nil means every list is current.
// Concurrent calls run one after the other.
func (d *DB) Update(ctx context.Context) error {
	d.updateMu.Lock()
	defer d.updateMu.Unlock()
	var errs []error
	for i, src := range d.sources {
		if err := ctx.Err(); err != nil {
			errs = append(errs, err)
			break
		}
		old := d.snap.Load().lists[i]
		l, err := d.refresh(ctx, src, old)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", src.Name, err))
			cp := *old
			cp.err = err.Error()
			l = &cp
		}
		d.swap(i, l)
	}
	if err := d.saveMeta(d.snap.Load()); err != nil {
		errs = append(errs, fmt.Errorf("saving list state: %w", err))
	}
	return errors.Join(errs...)
}

// swap publishes a snapshot with list i replaced. Only Update writes the
// snapshot, under updateMu.
func (d *DB) swap(i int, l *list) {
	next := &snapshot{lists: slices.Clone(d.snap.Load().lists)}
	next.lists[i] = l
	d.snap.Store(next)
}

// refresh fetches one list; it returns a new list or an error.
func (d *DB) refresh(ctx context.Context, src *source, old *list) (*list, error) {
	u, err := url.Parse(src.URL)
	if err != nil || u.Scheme != "https" {
		return nil, errors.New("list URL must be https")
	}
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "MiddleLAN")
	if old.loaded {
		if old.etag != "" {
			req.Header.Set("If-None-Match", old.etag)
		}
		if old.lastMod != "" {
			req.Header.Set("If-Modified-Since", old.lastMod)
		}
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	now := time.Now()
	switch {
	case resp.StatusCode == http.StatusNotModified && old.loaded:
		cp := *old
		cp.updated, cp.err = now, ""
		return &cp, nil
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("download: %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDownload+1))
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	if len(body) > maxDownload {
		return nil, fmt.Errorf("download: larger than %d MB", maxDownload>>20)
	}
	l, err := parseList(src, body)
	if err != nil {
		return nil, err
	}
	l.updated = now
	l.etag, l.lastMod = resp.Header.Get("ETag"), resp.Header.Get("Last-Modified")
	if dir := d.cacheDir(); dir != "" {
		if err := writeAtomic(filepath.Join(dir, src.File), body); err != nil {
			// Use the new list now, but without validators, so the next
			// update downloads it in full instead of trusting the cache.
			l.etag, l.lastMod = "", ""
			l.err = "saving: " + err.Error()
		}
	}
	return l, nil
}
