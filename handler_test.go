package main

import (
	"bytes"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestHandler(draws *int) *imageHandler {
	h := newImageHandler(NewGrapher("."), assetsVersion("."), 64<<20)
	draw := h.draw
	h.draw = func(domain string, size int) []byte {
		*draws++
		return draw(domain, size)
	}
	return h
}

func get(h http.Handler, url string, etag string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, url, nil)
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestServesSizedImageFromCache(t *testing.T) {
	draws := 0
	h := newTestHandler(&draws)

	first := get(h, "/img?d=pancakeswap&s=256", "")
	if first.Code != http.StatusOK || first.Header().Get("Cache-Control") == "" {
		t.Fatalf("status %d, headers %v", first.Code, first.Header())
	}
	img, err := png.Decode(bytes.NewReader(first.Body.Bytes()))
	if err != nil || img.Bounds().Dx() != 256 {
		t.Fatalf("want a 256px PNG, got %v %v", img.Bounds(), err)
	}

	second := get(h, "/img?d=pancakeswap&s=256", "")
	if !bytes.Equal(first.Body.Bytes(), second.Body.Bytes()) || draws != 1 {
		t.Fatalf("want one draw for a repeated request, got %d", draws)
	}

	get(h, "/img?d=pancakeswap", "")
	if draws != 2 {
		t.Fatalf("want a separate draw per size, got %d", draws)
	}
}

func TestRevalidatesByVersionedEtag(t *testing.T) {
	draws := 0
	h := newTestHandler(&draws)
	etag := get(h, "/img?d=pancakeswap", "").Header().Get("ETag")

	if code := get(h, "/img?d=pancakeswap", etag).Code; code != http.StatusNotModified {
		t.Fatalf("want 304 for the current ETag, got %d", code)
	}

	h.version = "changed"
	if code := get(h, "/img?d=pancakeswap", etag).Code; code != http.StatusOK {
		t.Fatalf("want a new image after the version changes, got %d", code)
	}
}

func TestRejectsInvalidInput(t *testing.T) {
	draws := 0
	h := newTestHandler(&draws)

	for _, url := range []string{
		"/img?d=abc", "/img?d=-abcd", "/img?d=abcd-", "/img?d=ABCD", "/img?d=a.bcd", "/img",
		"/img?d=abcd&s=100", "/img?d=abcd&s=x",
	} {
		if code := get(h, url, "").Code; code != http.StatusBadRequest {
			t.Errorf("%s: want 400, got %d", url, code)
		}
	}
	if draws != 0 {
		t.Fatalf("want no draws for invalid input, got %d", draws)
	}
}

func TestCacheEvictsLeastRecentlyUsed(t *testing.T) {
	c := newLruCache(10)
	c.put("a", make([]byte, 4))
	c.put("b", make([]byte, 4))
	c.get("a")
	c.put("c", make([]byte, 4))

	if _, ok := c.get("b"); ok {
		t.Fatal("want b evicted")
	}
	if _, ok := c.get("a"); !ok {
		t.Fatal("want a kept")
	}
}
