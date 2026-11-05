package profiles

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// SubStoreItem is a subscription or a collection of a Sub-Store backend.
type SubStoreItem struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName,omitempty"`
	// Kind is "sub" or "collection".
	Kind   string `json:"kind"`
	Remark string `json:"remark,omitempty"`
	Icon   string `json:"icon,omitempty"`
	// URL downloads it as a mihomo configuration.
	URL string `json:"url"`
}

// SubStoreList lists the subscriptions and collections of a Sub-Store
// backend at base (its address, with the path prefix it may have).
func SubStoreList(ctx context.Context, c *http.Client, base string) ([]SubStoreItem, error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if u, err := url.Parse(base); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("%q is not the address of a Sub-Store backend", base)
	}
	out := []SubStoreItem{}
	for _, kind := range []string{"sub", "collection"} {
		path := "/api/subs"
		if kind == "collection" {
			path = "/api/collections"
		}
		items, err := subStoreGet(ctx, c, base+path)
		if err != nil {
			if kind == "collection" && len(out) > 0 {
				break // older backends have no collections
			}
			return nil, err
		}
		for _, it := range items {
			if it.Name == "" {
				continue
			}
			dl := base + "/download/" + url.PathEscape(it.Name) + "?target=ClashMeta"
			if kind == "collection" {
				dl = base + "/download/collection/" + url.PathEscape(it.Name) + "?target=ClashMeta"
			}
			out = append(out, SubStoreItem{Name: it.Name, DisplayName: it.DisplayName, Kind: kind, Remark: it.Remark, Icon: it.Icon, URL: dl})
		}
	}
	return out, nil
}

type subStoreEntry struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Remark      string `json:"remark"`
	Icon        string `json:"icon"`
}

func subStoreGet(ctx context.Context, c *http.Client, u string) ([]subStoreEntry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return nil, describeNetError(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the Sub-Store backend answered HTTP %d", resp.StatusCode)
	}
	var r struct {
		Status string          `json:"status"`
		Data   json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &r); err != nil || r.Status != "success" {
		return nil, errors.New("the address does not answer as a Sub-Store backend does")
	}
	var items []subStoreEntry
	if err := json.Unmarshal(r.Data, &items); err != nil {
		return nil, errors.New("the Sub-Store backend answered something unexpected")
	}
	return items, nil
}
