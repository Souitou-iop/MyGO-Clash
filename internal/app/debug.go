package app

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/egoist/mygo"
)

// startDebug serves a debugging API on the loopback, in development builds
// only and when MYGO_CLASH_DEBUG names its address (127.0.0.1:port):
//
//	GET  /capture           a PNG of the main window
//	GET  /capture?w=panel   a PNG of the quick panel
//	POST /eval              runs the body as JavaScript in the page
//	POST /navigate?page=p   shows a page
//	POST /panel             toggles the quick panel
func (a *App) startDebug() {
	addr := os.Getenv("MYGO_CLASH_DEBUG")
	if addr == "" || !mygo.IsDev() {
		return
	}
	if host, _, err := net.SplitHostPort(addr); err != nil || (host != "127.0.0.1" && host != "localhost") {
		log.Printf("debug: %q is not a loopback address", addr)
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /capture", func(w http.ResponseWriter, r *http.Request) {
		var win *mygo.Window
		if r.URL.Query().Get("w") == "panel" {
			win = a.panel.window()
		} else {
			a.mu.Lock()
			win = a.win
			a.mu.Unlock()
		}
		if win == nil || win.IsDestroyed() {
			http.Error(w, "no window", http.StatusNotFound)
			return
		}
		png, err := win.CapturePage()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	})
	mux.HandleFunc("POST /eval", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		win := a.win
		a.mu.Unlock()
		if win == nil || win.Page() == nil {
			http.Error(w, "no page", http.StatusNotFound)
			return
		}
		code, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		v, err := win.Page().Eval(string(code))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(v)
	})
	mux.HandleFunc("POST /navigate", func(w http.ResponseWriter, r *http.Request) {
		a.navigate(r.URL.Query().Get("page"))
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /panel", func(w http.ResponseWriter, r *http.Request) {
		a.panel.toggle()
		w.WriteHeader(http.StatusNoContent)
	})
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Printf("debug API on http://%s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("debug: %v", err)
		}
	}()
}
