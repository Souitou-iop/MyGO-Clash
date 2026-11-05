package profiles

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRollback(t *testing.T) {
	body := sub
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
	defer srv.Close()
	m := newManager(t)
	ctx := context.Background()
	p, err := m.Create(ctx, NewProfile{Type: TypeRemote, URL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	if p.Prev != nil {
		t.Fatal("a new profile has a previous version")
	}
	// The same content again keeps no previous version.
	if p, _ = m.Update(ctx, p.UID, nil); p.Prev != nil {
		t.Fatal("an update that changed nothing kept a previous version")
	}
	body = strings.Replace(sub, "HK, type", "JP, type", 1) + "  - DOMAIN,x.com,DIRECT\n"
	body = strings.Replace(body, "[HK, DIRECT]", "[JP, DIRECT]", 1)
	if p, err = m.Update(ctx, p.UID, nil); err != nil || p.Prev == nil {
		t.Fatalf("update: %v %+v", err, p.Prev)
	}
	if p, err = m.Rollback(p.UID); err != nil {
		t.Fatal(err)
	}
	if c, _ := m.Content(p.UID); !strings.Contains(string(c), "name: HK") {
		t.Fatalf("after rollback: %s", c)
	}
	// Twice undoes it.
	if _, err = m.Rollback(p.UID); err != nil {
		t.Fatal(err)
	}
	if c, _ := m.Content(p.UID); !strings.Contains(string(c), "JP") {
		t.Fatalf("after the second rollback: %s", c)
	}
}
