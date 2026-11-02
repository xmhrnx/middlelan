package threat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// metaFile records per-list download state next to the cached lists.
const metaFile = "meta.json"

// listMeta is what is remembered about a cached list between runs.
type listMeta struct {
	ETag         string    `json:"etag,omitempty"`
	LastModified string    `json:"lastModified,omitempty"`
	Updated      time.Time `json:"updated"`
}

func (d *DB) cacheDir() string {
	if d.dir == "" {
		return ""
	}
	return filepath.Join(d.dir, "threat")
}

// loadCache builds the startup snapshot from dir/threat/*.txt.
func (d *DB) loadCache() *snapshot {
	s := &snapshot{lists: make([]*list, len(d.sources))}
	dir := d.cacheDir()
	meta := map[string]listMeta{}
	if dir != "" {
		if b, err := os.ReadFile(filepath.Join(dir, metaFile)); err == nil {
			_ = json.Unmarshal(b, &meta) // a broken file only costs a full download
		}
	}
	for i, src := range d.sources {
		s.lists[i] = d.loadList(dir, src, meta[src.File])
	}
	return s
}

func (d *DB) loadList(dir string, src *source, m listMeta) *list {
	empty := &list{src: src}
	if dir == "" {
		return empty
	}
	path := filepath.Join(dir, src.File)
	st, err := os.Stat(path)
	if err != nil {
		return empty
	}
	if st.Size() > maxDownload {
		empty.err = "cached copy unusable: too large"
		return empty
	}
	data, err := os.ReadFile(path)
	if err != nil {
		empty.err = "cached copy unusable: " + err.Error()
		return empty
	}
	l, err := parseList(src, data)
	if err != nil {
		empty.err = "cached copy unusable: " + err.Error()
		return empty
	}
	l.etag, l.lastMod, l.updated = m.ETag, m.LastModified, m.Updated
	if l.updated.IsZero() {
		l.updated = st.ModTime()
	}
	return l
}

// saveMeta records the download state of every loaded list.
func (d *DB) saveMeta(s *snapshot) error {
	dir := d.cacheDir()
	if dir == "" {
		return nil
	}
	meta := map[string]listMeta{}
	for _, l := range s.lists {
		if l.loaded {
			meta[l.src.File] = listMeta{ETag: l.etag, LastModified: l.lastMod, Updated: l.updated}
		}
	}
	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, metaFile), b)
}

// writeAtomic replaces path with data so that readers (and a crash) see
// either the old or the new file, never a partial one.
func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		_ = os.Remove(tmp)
	}
	return err
}
