// Command ouigen downloads the IEEE OUI registry (as published by
// github.com/silverwind/oui-data) and writes the compact gzipped table that
// package nettools embeds for MAC vendor lookups.
//
// Each output line is "PREFIX\tVendor\n" where PREFIX is 6, 7 or 9
// upper-case hex digits (MA-L, MA-M and MA-S assignments).
//
// Usage (from internal/nettools): go generate ./...
package main

import (
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

const defaultURL = "https://raw.githubusercontent.com/silverwind/oui-data/master/index.json"

func main() {
	url := flag.String("url", defaultURL, "source JSON (URL or local file path)")
	out := flag.String("out", "oui_data.gz", "output file")
	flag.Parse()
	if err := run(*url, *out); err != nil {
		log.Fatal(err)
	}
}

func run(src, out string) error {
	raw, err := fetch(src)
	if err != nil {
		return err
	}
	var data map[string]string
	if err := json.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("decode %s: %w", src, err)
	}
	keys := make([]string, 0, len(data))
	for k := range data {
		k2 := strings.ToUpper(strings.TrimSpace(k))
		if l := len(k2); l != 6 && l != 7 && l != 9 {
			continue
		}
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return strings.ToUpper(keys[i]) < strings.ToUpper(keys[j]) })

	f, err := os.Create(out)
	if err != nil {
		return err
	}
	zw, err := gzip.NewWriterLevel(f, gzip.BestCompression)
	if err != nil {
		_ = f.Close()
		return err
	}
	n := 0
	for _, k := range keys {
		name := shortName(data[k])
		if name == "" {
			continue
		}
		if _, err := fmt.Fprintf(zw, "%s\t%s\n", strings.ToUpper(strings.TrimSpace(k)), name); err != nil {
			_ = zw.Close()
			_ = f.Close()
			return err
		}
		n++
	}
	if err := zw.Close(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	log.Printf("wrote %d prefixes to %s", n, out)
	return nil
}

// shortName returns the first line of the registry text with whitespace
// collapsed and tabs removed.
func shortName(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return strings.Join(strings.Fields(line), " ")
}

func fetch(src string) ([]byte, error) {
	if !strings.HasPrefix(src, "http://") && !strings.HasPrefix(src, "https://") {
		return os.ReadFile(src)
	}
	c := &http.Client{Timeout: 2 * time.Minute}
	resp, err := c.Get(src)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck // read errors are reported
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", src, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}
