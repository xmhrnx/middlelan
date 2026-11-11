package adblock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	maxDownload     = 30 << 20
	downloadTimeout = 60 * time.Second
	metaFile        = "meta.json"
)

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

func newClient() *http.Client {
	return &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if req.URL.Scheme != "https" {
			return fmt.Errorf("refusing redirect to %s", req.URL.Scheme)
		}
		return nil
	}}
}

// list is the state of one source.
type list struct {
	rules          *rules
	updated, tried time.Time
	err            string
	etag, lastMod  string
}

type listMeta struct {
	ETag         string    `json:"etag,omitempty"`
	LastModified string    `json:"lastModified,omitempty"`
	Updated      time.Time `json:"updated"`
}

func (b *Blocker) cacheDir() string {
	if b.dir == "" {
		return ""
	}
	return filepath.Join(b.dir, "adblock")
}

func (b *Blocker) loadCache() []*list {
	out := make([]*list, len(b.sources))
	dir := b.cacheDir()
	meta := map[string]listMeta{}
	if dir != "" {
		if data, err := os.ReadFile(filepath.Join(dir, metaFile)); err == nil {
			_ = json.Unmarshal(data, &meta)
		}
	}
	for i, src := range b.sources {
		l := &list{}
		out[i] = l
		if dir == "" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, src.ID+".txt"))
		if err != nil {
			continue
		}
		l.rules = parse(data)
		m := meta[src.ID]
		l.etag, l.lastMod, l.updated = m.ETag, m.LastModified, m.Updated
	}
	return out
}

// Update downloads every list (unchanged ones cost a conditional
// request) and swaps the new ones in. A list that fails keeps its last
// copy. Concurrent calls run one after the other.
func (b *Blocker) Update(ctx context.Context) error {
	b.updateMu.Lock()
	defer b.updateMu.Unlock()
	var errs []error
	for i, src := range b.sources {
		b.listsMu.Lock()
		old := *b.lists[i]
		b.listsMu.Unlock()
		next, err := b.fetch(ctx, src, old)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", src.Name, err))
			next = old
			next.err = err.Error()
		}
		next.tried = time.Now()
		b.listsMu.Lock()
		b.lists[i] = &next
		b.listsMu.Unlock()
	}
	b.rebuild()
	if err := b.saveMeta(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (b *Blocker) fetch(ctx context.Context, src Source, old list) (list, error) {
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
	if err != nil {
		return list{}, err
	}
	req.Header.Set("User-Agent", "MiddleMan")
	if old.rules != nil {
		if old.etag != "" {
			req.Header.Set("If-None-Match", old.etag)
		}
		if old.lastMod != "" {
			req.Header.Set("If-Modified-Since", old.lastMod)
		}
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return list{}, err
	}
	defer resp.Body.Close()
	now := time.Now()
	switch {
	case resp.StatusCode == http.StatusNotModified && old.rules != nil:
		old.updated, old.err = now, ""
		return old, nil
	case resp.StatusCode != http.StatusOK:
		return list{}, fmt.Errorf("download: %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxDownload+1))
	if err != nil {
		return list{}, fmt.Errorf("download: %w", err)
	}
	if len(data) > maxDownload {
		return list{}, fmt.Errorf("download: larger than %d MB", maxDownload>>20)
	}
	r := parse(data)
	if r.size() < src.MinEntries {
		return list{}, fmt.Errorf("only %d usable entries", r.size())
	}
	l := list{rules: r, updated: now, etag: resp.Header.Get("ETag"), lastMod: resp.Header.Get("Last-Modified")}
	if dir := b.cacheDir(); dir != "" {
		if err := writeAtomic(filepath.Join(dir, src.ID+".txt"), data); err != nil {
			l.etag, l.lastMod = "", ""
			l.err = "saving: " + err.Error()
		}
	}
	return l, nil
}

func (b *Blocker) saveMeta() error {
	dir := b.cacheDir()
	if dir == "" {
		return nil
	}
	meta := map[string]listMeta{}
	b.listsMu.Lock()
	for i, l := range b.lists {
		if l.rules != nil {
			meta[b.sources[i].ID] = listMeta{ETag: l.etag, LastModified: l.lastMod, Updated: l.updated}
		}
	}
	b.listsMu.Unlock()
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, metaFile), data)
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}
