package main

import (
	"bytes"
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/png"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"sync"
	"time"

	xdraw "golang.org/x/image/draw"
)

// Bump when the drawing code changes. The ETag carries it, so clients drop the images drawn the old way
const renderVersion = "1"

const fullSize = 1024

// Clients revalidate after this time, so a changed image reaches them within a day
const cacheControl = "public, max-age=86400"

var allowedSizes = map[int]bool{256: true, 512: true, fullSize: true}

// The base name rules of the DNS collection contract (`check_domain_string`)
var domainFormat = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,124}[a-z0-9]$`)

var assetFiles = []string{
	"g24-left.png", "g24-right.png", "g24-center.png",
	"normal.png", "medium.png", "rare.png", "super.png",
	"JetBrainsMono-ExtraBold.ttf",
}

// The images change only with the assets or the drawing code, so their hash versions every image
func assetsVersion(pathPrefix string) string {
	h := sha256.New()
	h.Write([]byte(renderVersion))
	for _, name := range assetFiles {
		data, err := os.ReadFile(pathPrefix + "/" + name)
		if err != nil {
			panic(err)
		}
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

type imageHandler struct {
	draw    func(domain string, size int) []byte
	version string
	cache   *lruCache
}

func newImageHandler(g *Grapher, version string, cacheBytes int) *imageHandler {
	return &imageHandler{
		draw:    func(domain string, size int) []byte { return encodeImage(g.Draw(domain).Image(), size) },
		version: version,
		cache:   newLruCache(cacheBytes),
	}
}

func (h *imageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	domain := r.URL.Query().Get("d")
	if !domainFormat.MatchString(domain) {
		http.Error(w, "invalid domain", http.StatusBadRequest)
		return
	}

	size := fullSize
	if s := r.URL.Query().Get("s"); s != "" {
		parsed, err := strconv.Atoi(s)
		if err != nil || !allowedSizes[parsed] {
			http.Error(w, "invalid size", http.StatusBadRequest)
			return
		}
		size = parsed
	}

	key := strconv.Itoa(size) + ":" + domain
	data, ok := h.cache.get(key)
	if !ok {
		data = h.draw(domain, size)
		h.cache.put(key, data)
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", cacheControl)
	w.Header().Set("ETag", `"`+h.version+"-"+key+`"`)
	// Answers `If-None-Match` with 304
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
}

func encodeImage(img image.Image, size int) []byte {
	if size != img.Bounds().Dx() {
		scaled := image.NewRGBA(image.Rect(0, 0, size, size))
		xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), img, img.Bounds(), xdraw.Src, nil)
		img = scaled
	}

	var buf bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestCompression}
	if err := encoder.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// An LRU of encoded images bounded by their total size
type lruCache struct {
	mu       sync.Mutex
	maxBytes int
	bytes    int
	order    *list.List
	items    map[string]*list.Element
}

type lruEntry struct {
	key  string
	data []byte
}

func newLruCache(maxBytes int) *lruCache {
	return &lruCache{maxBytes: maxBytes, order: list.New(), items: map[string]*list.Element{}}
}

func (c *lruCache) get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[key]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(el)
	return el.Value.(*lruEntry).data, true
}

func (c *lruCache) put(key string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.items[key]; ok {
		return
	}
	c.items[key] = c.order.PushFront(&lruEntry{key: key, data: data})
	c.bytes += len(data)

	for c.bytes > c.maxBytes {
		oldest := c.order.Back()
		entry := oldest.Value.(*lruEntry)
		c.order.Remove(oldest)
		delete(c.items, entry.key)
		c.bytes -= len(entry.data)
	}
}
