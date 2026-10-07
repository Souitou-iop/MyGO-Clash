package corehost

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/metacubex/mihomo/component/mmdb"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/log"

	"github.com/mygo-clash/mygo-clash/internal/corehost/geodata"
	"github.com/mygo-clash/mygo-clash/internal/yamlx"
)

// geoMirrors serve the files of MetaCubeX/meta-rules-dat, GitHub last:
// it is out of reach on many networks, and mihomo would try nothing else.
var geoMirrors = []string{
	"https://fastly.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/",
	"https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/",
	"https://cdn.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/",
	"https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/",
}

type geoFile struct {
	key  string        // its key under geox-url
	name string        // its name in meta-rules-dat
	path func() string // where mihomo looks for it
	mmdb bool
}

// ensureGeo downloads the geo databases a configuration uses and the home
// lacks. mihomo would download them itself while parsing the
// configuration, directly from GitHub alone, and fail to start where
// GitHub is out of reach: on a new machine, everything else works.
func ensureGeo(ctx context.Context, raw string) {
	cfg, err := yamlx.Parse([]byte(raw))
	if err != nil {
		return // the parse reports it
	}
	text := strings.ToUpper(raw)
	datMode, _ := cfg.Bool("geodata-mode")
	var need []geoFile
	if strings.Contains(text, "GEOIP") {
		if datMode {
			need = append(need, geoFile{"geoip", "geoip.dat", C.Path.GeoIP, false})
		} else {
			need = append(need, geoFile{"mmdb", "geoip.metadb", C.Path.MMDB, true})
		}
	}
	if strings.Contains(text, "GEOSITE") {
		need = append(need, geoFile{"geosite", "geosite.dat", C.Path.GeoSite, false})
	}
	if strings.Contains(text, "IP-ASN") {
		need = append(need, geoFile{"asn", "GeoLite2-ASN.mmdb", C.Path.ASN, true})
	}
	// The app waits a minute for the answer.
	ctx, cancel := context.WithTimeout(ctx, 50*time.Second)
	defer cancel()
	urls := cfg.Map("geox-url")
	for _, f := range need {
		path := f.path()
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			continue
		}
		// The copy this build carries, before any download.
		if b, ok := geodata.Read(f.name); ok {
			err := place(path, bytes.NewReader(b), f.mmdb)
			if err == nil {
				log.Infoln("[Geo] %s from the app", filepath.Base(path))
				continue
			}
			log.Warnln("[Geo] the app's %s: %v", f.name, err)
		}
		var sources []string
		if urls != nil {
			if u := strings.TrimSpace(urls.String(f.key)); u != "" {
				sources = append(sources, u)
			}
		}
		for _, m := range geoMirrors {
			if u := m + f.name; !slices.Contains(sources, u) {
				sources = append(sources, u)
			}
		}
		var errs []string
		for _, u := range sources {
			if ctx.Err() != nil {
				return
			}
			log.Infoln("[Geo] downloading %s from %s", filepath.Base(path), u)
			if err := download(ctx, u, path, f.mmdb); err != nil {
				log.Warnln("[Geo] %s: %v", u, err)
				errs = append(errs, err.Error())
				continue
			}
			errs = nil
			break
		}
		if errs != nil {
			log.Errorln("[Geo] no source of %s worked", f.name)
		}
	}
}

// download fetches url into path.
func download(ctx context.Context, url, path string, isMMDB bool) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	// A source out of reach fails fast and leaves the time to the next.
	client := &http.Client{Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 8 * time.Second}).DialContext,
		TLSHandshakeTimeout:   8 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	}}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %s", resp.Status)
	}
	return place(path, resp.Body, isMMDB)
}

// place writes a database to path through a temporary file that must
// check out, so a broken copy never stands in for it.
func place(path string, r io.Reader, isMMDB bool) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".geo-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	head := make([]byte, 64)
	n, _ := io.ReadFull(r, head)
	if bytes.HasPrefix(bytes.TrimSpace(head[:n]), []byte("<")) {
		tmp.Close()
		return fmt.Errorf("got a web page, not the database")
	}
	_, err = tmp.Write(head[:n])
	if err == nil {
		_, err = io.Copy(tmp, r)
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if fi, err := os.Stat(tmp.Name()); err != nil || fi.Size() < 1024 {
		return fmt.Errorf("the file is too small")
	}
	if isMMDB && !mmdb.Verify(tmp.Name()) {
		return fmt.Errorf("not a valid MMDB file")
	}
	_ = os.Chmod(tmp.Name(), 0o644)
	return os.Rename(tmp.Name(), path)
}
