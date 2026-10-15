package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"runtime"
	"time"

	"github.com/egoist/mygo"

	"github.com/mygo-clash/mygo-clash/internal/service"
)

// updateServiceQuietly brings a service of an older version up to the
// app's, without asking for an administrator, once a launch: it downloads
// the app's own signed archive of its version and hands it to the service,
// which checks the signature itself (see service.Client.Update). The core
// in the service stops for the few seconds the service takes to restart.
// When it fails, the service stays as it is, and the settings still offer
// to update it with an administrator.
func (a *App) updateServiceQuietly(svc ServiceState) {
	if mygo.IsDev() || !svc.Installed || svc.Error != "" || !(svc.Behind || svc.Outdated) {
		return
	}
	if !a.serviceUpdate.CompareAndSwap(false, true) {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		if err := a.updateService(ctx); err != nil {
			log.Printf("service: updating to %s: %v", Version, err)
		}
	}()
}

func (a *App) updateService(ctx context.Context) error {
	client := service.NewClient(a.slug)
	st, err := client.Status(ctx)
	if err != nil {
		return err
	}
	if !st.Updates {
		return fmt.Errorf("the service of version %s cannot update itself", st.Version)
	}

	// The manifest of this version's release, which the app's updates
	// read the latest of.
	base := ReleasesURL + "/download/v" + Version + "/"
	var m struct {
		Version   string `json:"version"`
		URL       string `json:"url"`
		Size      int64  `json:"size"`
		Signature string `json:"signature"`
	}
	if err := getJSON(ctx, base+"update-"+runtime.GOOS+"-"+runtime.GOARCH+".json", &m); err != nil {
		return err
	}
	if m.Version != Version || m.URL == "" || m.Signature == "" || m.Size <= 0 {
		return errors.New("the release has no update of this version")
	}
	body, err := get(ctx, m.URL)
	if err != nil {
		return err
	}
	defer body.Close()
	if err := client.Update(ctx, Version, m.Signature, io.LimitReader(body, m.Size)); err != nil {
		return err
	}

	inService := a.core.State().Mode == "service"
	if inService {
		a.core.Stop(ctx)
	}
	if err := client.ApplyUpdate(ctx); err != nil {
		if inService {
			a.startCore(ctx)
		}
		return err
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		if s := a.checkService(ctx); s.Usable() && !s.Behind {
			break
		}
	}
	log.Printf("service: updated to %s", a.checkService(ctx).Version)
	// The core moves back to the service, or to it from the app, where an
	// outdated service had left it.
	if a.settings.Get().CoreMode != "sidecar" {
		a.startCore(ctx)
	}
	return nil
}

func get(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// The default client goes to GitHub through the core, directly or
	// through the mirror (see githubTransport).
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return resp.Body, nil
}

func getJSON(ctx context.Context, url string, v any) error {
	body, err := get(ctx, url)
	if err != nil {
		return err
	}
	defer body.Close()
	return json.NewDecoder(io.LimitReader(body, 1<<20)).Decode(v)
}
