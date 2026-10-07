// Command geodata downloads the geo databases that release builds carry
// into internal/corehost/geodata/files. Copies younger than a week are
// kept, so local builds do not fetch them every time; -force fetches them
// anyway.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

var names = []string{"geoip.metadb", "geosite.dat"}

var sources = []string{
	"https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/",
	"https://fastly.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/",
	"https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/",
}

func main() {
	force := flag.Bool("force", false, "download even fresh copies")
	dir := flag.String("dir", "internal/corehost/geodata/files", "where the databases go")
	flag.Parse()
	for _, name := range names {
		path := filepath.Join(*dir, name)
		if fi, err := os.Stat(path); err == nil && !*force && time.Since(fi.ModTime()) < 7*24*time.Hour {
			continue
		}
		if err := fetch(name, path); err != nil {
			log.Fatalf("geodata: %s: %v", name, err)
		}
	}
}

func fetch(name, path string) error {
	var last error
	for _, src := range sources {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		b, err := get(ctx, src+name)
		cancel()
		if err != nil {
			last = fmt.Errorf("%s: %w", src, err)
			continue
		}
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, b, 0o644); err != nil {
			return err
		}
		log.Printf("geodata: %s, %d bytes, from %s", name, len(b), src)
		return os.Rename(tmp, path)
	}
	return last
}

func get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %s", resp.Status)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	// A database, not an error page.
	if len(b) < 64<<10 || bytes.HasPrefix(bytes.TrimSpace(b[:64]), []byte("<")) {
		return nil, fmt.Errorf("not a database (%d bytes)", len(b))
	}
	return b, nil
}
