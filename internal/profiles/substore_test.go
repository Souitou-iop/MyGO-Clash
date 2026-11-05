package profiles

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSubStoreList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/secret/api/subs":
			_, _ = w.Write([]byte(`{"status":"success","data":[{"name":"my airport","displayName":"Airport"},{"name":""}]}`))
		case "/secret/api/collections":
			_, _ = w.Write([]byte(`{"status":"success","data":[{"name":"all"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	items, err := SubStoreList(context.Background(), srv.Client(), srv.URL+"/secret/")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].URL != srv.URL+"/secret/download/my%20airport?target=ClashMeta" || items[1].Kind != "collection" ||
		items[1].URL != srv.URL+"/secret/download/collection/all?target=ClashMeta" {
		t.Fatalf("items: %+v", items)
	}
	if _, err := SubStoreList(context.Background(), srv.Client(), srv.URL); err == nil {
		t.Fatal("a server that is not Sub-Store was listed")
	}
}
