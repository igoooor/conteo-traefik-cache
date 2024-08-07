// Package conteo_traefik_cache is a plugin to cache responses to disk.
package conteo_traefik_cache

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"

	// "regexp"
	"strings"
	"time"

	"github.com/igoooor/conteo-traefik-cache/provider/api"

	// "github.com/igoooor/conteo-traefik-cache/provider/local"
	"github.com/pquerna/cachecontrol"
)

const healthcheckPeriod = 300 * time.Second

// Config configures the middleware.
type Config struct {
	Path            string     `json:"path" yaml:"path" toml:"path"`
	MaxExpiry       int        `json:"maxExpiry" yaml:"maxExpiry" toml:"maxExpiry"`
	Cleanup         int        `json:"cleanup" yaml:"cleanup" toml:"cleanup"`
	Memory          bool       `json:"memory" yaml:"memory" toml:"memory"`
	AddStatusHeader bool       `json:"addStatusHeader" yaml:"addStatusHeader" toml:"addStatusHeader"`
	FlushHeader     string     `json:"flushHeader" yaml:"flushHeader" toml:"flushHeader"`
	NextGenFormats  []string   `json:"nextGenFormats" yaml:"nextGenFormats" toml:"nextGenFormats"`
	Headers         []string   `json:"headers" yaml:"headers" toml:"headers"`
	Key             KeyContext `json:"key" yaml:"key" toml:"key"`
	Debug           bool       `json:"debug" yaml:"debug" toml:"debug"`
	// SurrogateKeys   map[string]SurrogateKeys `json:"surrogateKeys" yaml:"surrogateKeys" toml:"surrogateKeys"`
}

type KeyContext struct {
	DisableHost   bool `json:"disableHost" yaml:"disableHost" toml:"disableHost"`
	DisableMethod bool `json:"disableMethod" yaml:"disableMethod" toml:"disableMethod"`
}

/*type SurrogateKeys struct {
	URL     string            `json:"url" yaml:"url"`
	Headers map[string]string `json:"headers" yaml:"headers"`
}

type keysRegexpInner struct {
	Headers map[string]*regexp.Regexp
	Url     *regexp.Regexp
}*/

// CreateConfig returns a config instance.
func CreateConfig() *Config {
	return &Config{
		MaxExpiry:       int((5 * time.Minute).Seconds()),
		Cleanup:         int((5 * time.Minute).Seconds()),
		Memory:          false,
		AddStatusHeader: true,
		FlushHeader:     "X-Cache-Flush",
		NextGenFormats:  []string{},
		Headers:         []string{},
		Key: KeyContext{
			DisableHost:   false,
			DisableMethod: false,
		},
		Debug: false,
	}
}

const (
	cacheHeader       = "Cache-Status"
	ageHeader         = "Age"
	etagHeader        = "Etag"
	requestEtagHeader = "If-None-Match"
	skipEtagHeader    = "X-Skip-Etag"
	cacheHitStatus    = "hit; ttl=%d"
	cacheMissStatus   = "miss"
	cacheErrorStatus  = "error"
	acceptHeader      = "Accept"
)

type CacheSystem interface {
	Get(string, string) ([]byte, bool, error)
	Delete(string)
	Set(string, []byte, time.Duration, string) error
	Check(bool) bool
}

type cache struct {
	name           string
	cache          api.FileCache
	cfg            *Config
	next           http.Handler
	cacheAvailable bool
	// keysRegexp map[string]keysRegexpInner
}

// New returns a plugin instance.
func New(_ context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	if cfg.MaxExpiry <= 1 {
		return nil, errors.New("maxExpiry must be greater or equal to 1")
	}

	if cfg.Cleanup <= 1 {
		return nil, errors.New("cleanup must be greater or equal to 1")
	}

	// temporarily disable local backup if api not available
	fc, err := api.NewFileCache(cfg.Path)
	if err != nil {
		return nil, err
	}

	//cacheAvailable := fc.Check(true)

	m := &cache{
		name:           name,
		cache:          *fc,
		cfg:            cfg,
		next:           next,
		cacheAvailable: true,
		//cacheAvailable: cacheAvailable,
		//keysRegexp: keysRegexp,
	}

	// go m.cacheHealthcheck(healthcheckPeriod)

	return m, nil
}

type cacheData struct {
	Status  int
	Headers map[string][]string
	Body    []byte
	Created uint64
	Etag    string
	Expiry  uint64
}

// ServeHTTP serves an HTTP request.
func (m *cache) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := m.cacheKey(r)

	if r.Method == "DELETE" {
		w.WriteHeader(204)
		_, _ = w.Write([]byte{})
		m.deleteCacheFile(key, r)

		return
	}

	cs := cacheMissStatus

	if m.bypassingHeaders(r) {
		rw := &responseWriter{ResponseWriter: w}
		m.next.ServeHTTP(rw, r)

		return
	}

	cache, err := m.getCache()
	if err != nil {
		rw := &responseWriter{ResponseWriter: w}
		m.next.ServeHTTP(rw, r)

		return
	}

	b, matchEtag, err := cache.Get(key, getRequestEtag(r))
	if matchEtag {
		if m.cfg.Debug {
			log.Printf("[Cache] DEBUG hit + match etag")
		}
		w.WriteHeader(304)
		return
	}
	if err != nil {
		if m.handleCacheErrorAndExit(err, w, r) {
			return
		}
	} else if b != nil {
		var data cacheData

		err := json.Unmarshal(b, &data)
		if err != nil || data.Status > 299 || m.invalidCacheBody(data) {
			if m.cfg.Debug {
				if err != nil {
					log.Printf("[Cache] DEBUG error: unmarshalling cache item: %v", err)
				} else if data.Status > 299 {
					log.Printf("[Cache] DEBUG error: cache item status: %d", data.Status)
				} else {
					log.Printf("[Cache] DEBUG error: invalid body")
				}
			}
			cs = cacheErrorStatus
			// if cache error, delete the cache data
			cache.Delete(key)
		} else {
			m.sendCacheFile(w, data, r, key)
			return
		}
	}

	if m.cfg.Debug {
		log.Printf("[Cache] DEBUG cs: %s", cs)
	}

	if m.cfg.AddStatusHeader {
		w.Header().Set(cacheHeader, cs)
	}

	rw := &responseWriter{ResponseWriter: w}
	m.next.ServeHTTP(rw, r)

	if m.cfg.Debug {
		log.Printf("[Cache] DEBUG Backend response Body length: %d", len(rw.body))
	}

	expiry, ok := m.cacheable(r, w, rw)
	if !ok {
		return
	}

	createdTs := uint64(time.Now().Unix())
	data := cacheData{
		Status:  rw.status,
		Headers: w.Header(),
		Body:    rw.body,
		Created: createdTs,
		Etag:    calculateEtag(createdTs),
		Expiry:  uint64(time.Now().Add(expiry).Unix()),
	}

	b, err = json.Marshal(data)
	if err != nil {
		log.Printf("Error serializing cache item: %v", err)
	}

	if err = cache.Set(key, b, expiry, data.Etag); err != nil {
		log.Println("Error setting cache item")
		if m.handleCacheErrorAndExit(err, w, r) {
			return
		}
	}
	if m.cfg.Debug {
		log.Printf("[Cache] DEBUG set %s", key)
	}
}

func (m *cache) invalidCacheBody(data cacheData) bool {
	if contentLength, ok := data.Headers["Content-Length"]; ok && len(contentLength) >= 1 {
		if cl, err := strconv.Atoi(contentLength[0]); err == nil && cl != len(data.Body) {
			return true
		}
	}

	return false
}

func (m *cache) cacheable(r *http.Request, w http.ResponseWriter, rw *responseWriter) (time.Duration, bool) {
	if rw.status > 299 {
		return 0, false
	}

	bodyLength := len(rw.body)
	if bodyLength == 0 {
		return 0, false
	}

	if contentLength := w.Header().Get("Content-Length"); contentLength != "" {
		if cl, err := strconv.Atoi(contentLength); err == nil && cl != bodyLength {
			return 0, false
		}
	}

	reasons, expireBy, err := cachecontrol.CachableResponseWriter(r, rw.status, w, cachecontrol.Options{})
	if err != nil || len(reasons) > 0 {
		return 0, false
	}

	expiry := time.Until(expireBy)
	maxExpiry := time.Duration(m.cfg.MaxExpiry) * time.Second

	if maxExpiry < expiry {
		expiry = maxExpiry
	}

	return expiry, true
}

func (m *cache) deleteCacheFile(key string, r *http.Request) {
	if flushHeader := r.Header.Get(m.cfg.FlushHeader); flushHeader != "" {
		if cache, err := m.getCache(); err == nil {
			cache.Delete(key)
		}
	}
}

func (m *cache) sendCacheFile(w http.ResponseWriter, data cacheData, r *http.Request, cacheKey string) {
	if m.cfg.Debug {
		log.Printf("[Cache] DEBUG hit")
		log.Printf("[Cache] DEBUG Cache Body length: %d", len(data.Body))
	}

	if getRequestEtag(r) == data.Etag {
		w.WriteHeader(304)
		return
	}

	w.Header().Set("X-Cache-Key", "/"+encodeKey(cacheKey))

	for key, vals := range data.Headers {
		for _, val := range vals {
			w.Header().Add(key, val)
		}
	}

	if strings.Contains(r.Host, "cdn") {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	}

	if m.cfg.AddStatusHeader {
		now := uint64(time.Now().Unix())
		age := now - data.Created
		ttl := data.Expiry - now
		w.Header().Set(cacheHeader, fmt.Sprintf(cacheHitStatus, ttl))
		w.Header().Set(ageHeader, strconv.FormatUint(age, 10))
	}

	w.Header().Set(etagHeader, data.Etag)
	w.WriteHeader(data.Status)

	_, _ = w.Write(data.Body)
}

func getRequestEtag(r *http.Request) string {
	if r.Header.Get(skipEtagHeader) != "" {
		return "n/a"
	}
	return r.Header.Get(requestEtagHeader)
}

func calculateEtag(created uint64) string {
	bs := make([]byte, 8)
	binary.LittleEndian.PutUint64(bs, created)
	return base64.URLEncoding.EncodeToString(bs)
}

func (m *cache) bypassingHeaders(r *http.Request) bool {
	return r.Header.Get("X-Conteo-Cache-Control") == "no-cache"
}

/*func (m *cache) matchSurrogateKeys(r *http.Request) []string {
	matchKeys := []string{}

	return matchKeys
}*/

func (m *cache) cacheKey(r *http.Request) string {
	key := ""
	if !m.cfg.Key.DisableMethod {
		if r.Method == "DELETE" {
			key += "-GET" // DELETE requests are treated as GET for key generation
		} else {
			key += "-" + r.Method
		}
	}

	if !m.cfg.Key.DisableHost {
		key += "-" + r.Host
	}

	key += "-" + strings.Split(r.URL.Path, "#")[0]

	headers := ""

	for _, header := range m.cfg.Headers {
		if r.Header.Get(header) != "" {
			headers += strings.ReplaceAll(r.Header.Get(header), " ", "")
		}
	}

	if headers != "" {
		headers = base64.StdEncoding.EncodeToString([]byte(headers))
		key += "-" + headers
	}

	if r.Header.Get(acceptHeader) != "" {
		accept := r.Header.Get(acceptHeader)
		acceptedFormats := strings.Split(accept, ",")

	out:
		for _, format := range m.cfg.NextGenFormats {
			for _, acceptedFormat := range acceptedFormats {
				if format == strings.ToLower(acceptedFormat) {
					key += "-" + strings.ReplaceAll(format, " ", "")
					break out
				}
			}
		}
	}

	key = strings.TrimLeft(key, "-")

	if m.cfg.Debug {
		log.Printf("[Cache] DEBUG key: %s", key)
	}

	return key
}

func (m *cache) getCache() (CacheSystem, error) {
	if m.cacheAvailable {
		return &m.cache, nil
	}

	return &m.cache, errors.New("Cache not available")
}

func (m *cache) handleCacheErrorAndExit(err error, w http.ResponseWriter, r *http.Request) bool {
	log.Println(err)
	if strings.Contains(err.Error(), "connect: connection refused") {
		//m.cacheAvailable = false
		rw := &responseWriter{ResponseWriter: w}
		m.next.ServeHTTP(rw, r)

		return true
	}

	return false
}

func (m *cache) cacheHealthcheck(interval time.Duration) {
	timer := time.NewTicker(interval)
	defer timer.Stop()

	for range timer.C {
		m.cacheAvailable = m.cache.Check(true)
		if m.cfg.Debug {
			log.Printf("[Cache] DEBUG healthcheck: %v", m.cacheAvailable)
		}
	}
}

type responseWriter struct {
	http.ResponseWriter
	status int
	body   []byte
}

func (rw *responseWriter) Header() http.Header {
	return rw.ResponseWriter.Header()
}

func (rw *responseWriter) Write(p []byte) (int, error) {
	rw.body = append(rw.body, p...)
	return rw.ResponseWriter.Write(p)
}

func (rw *responseWriter) WriteHeader(s int) {
	rw.status = s
	rw.ResponseWriter.WriteHeader(s)
}

func encodeKey(key string) string {
	return base64.URLEncoding.EncodeToString([]byte(key))
}
