package server

import (
	"context"
	"net/url"
	"strings"
	"testing"
)

func TestWarmupWithoutDB(t *testing.T) {
	app := newTestApp(t)

	app.Warmup()

	if len(app.cache.entries) != 0 {
		t.Fatalf("expected empty cache, got %d entries", len(app.cache.entries))
	}
}

func TestWarmupCacheKeyConsistency(t *testing.T) {
	key := cacheKey("fi-origin-monthly", url.Values{})
	if key != "fi-origin-monthly" {
		t.Fatalf("expected empty-query cache key to match dataset id, got %q", key)
	}
}

func TestWarmupCachesDatasetResponses(t *testing.T) {
	app := newCacheTestApp(t)

	app.Warmup()

	body, ok := app.cache.Get(cacheKey("fi-origin-monthly", url.Values{}))
	if !ok {
		t.Fatal("expected warmup cache entry")
	}
	if !strings.Contains(string(body), `"row_count": 1`) {
		t.Fatalf("expected cached warmup body to include row_count, got %s", string(body))
	}
}

func TestFetchDatasetRows(t *testing.T) {
	app := newCacheTestApp(t)

	contract, ok := app.catalog.Get("fi-origin-monthly")
	if !ok {
		t.Fatal("expected fi-origin-monthly contract")
	}

	columns, items, err := app.fetchDatasetRows(context.Background(), contract, contract.Limit.Default, url.Values{})
	if err != nil {
		t.Fatalf("fetch dataset rows: %v", err)
	}
	if len(columns) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(columns))
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0]["fi_origin"] != "MKPF" {
		t.Fatalf("expected fi_origin MKPF, got %#v", items[0]["fi_origin"])
	}
}
