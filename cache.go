// Package plugin_simplecache_conteo is a plugin to cache responses to disk.
package plugin_simplecache_conteo

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/pquerna/cachecontrol"
)

// Config configures the middleware.
type Config struct {
	Path            string     `json:"path" yaml:"path" toml:"path"`
	MaxExpiry       int        `json:"maxExpiry" yaml:"maxExpiry" toml:"maxExpiry"`
	Cleanup         int        `json:"cleanup" yaml:"cleanup" toml:"cleanup"`
	AddStatusHeader bool       `json:"addStatusHeader" yaml:"addStatusHeader" toml:"addStatusHeader"`
	NextGenFormats  []string   `json:"nextGenFormats" yaml:"nextGenFormats" toml:"nextGenFormats"`
	Headers         []string   `json:"headers" yaml:"headers" toml:"headers"`
	BypassHeaders   []string   `json:"bypassHeaders" yaml:"bypassHeaders" toml:"bypassHeaders"`
	Key             KeyContext `json:"key" yaml:"key" toml:"key"`
}

type KeyContext struct {
	DisableHost   bool `json:"disable_host" yaml:"disable_host" toml:"disable_host"`
	DisableMethod bool `json:"disable_method" yaml:"disable_method" toml:"disable_method"`
}

// CreateConfig returns a config instance.
func CreateConfig() *Config {
	return &Config{
		MaxExpiry:       int((5 * time.Minute).Seconds()),
		Cleanup:         int((5 * time.Minute).Seconds()),
		AddStatusHeader: true,
		NextGenFormats:  []string{},
		Headers:         []string{},
		BypassHeaders:   []string{},
		Key: KeyContext{},
	}
}

const (
	cacheHeader      = "Cache-Status"
	cacheHitStatus   = "hit"
	cacheMissStatus  = "miss"
	cacheErrorStatus = "error"
	acceptHeader     = "Accept"
)

type cache struct {
	name  string
	cache *fileCache
	cfg   *Config
	next  http.Handler
}

// New returns a plugin instance.
func New(_ context.Context, next http.Handler, cfg *Config, name string) (http.Handler, error) {
	if cfg.MaxExpiry <= 1 {
		return nil, errors.New("maxExpiry must be greater or equal to 1")
	}

	if cfg.Cleanup <= 1 {
		return nil, errors.New("cleanup must be greater or equal to 1")
	}

	fc, err := newFileCache(cfg.Path, time.Duration(cfg.Cleanup)*time.Second)
	if err != nil {
		return nil, err
	}

	log.Printf("%v", cfg)

	m := &cache{
		name:  name,
		cache: fc,
		cfg:   cfg,
		next:  next,
	}

	return m, nil
}

type cacheData struct {
	Status  int
	Headers map[string][]string
	Body    []byte
}

// ServeHTTP serves an HTTP request.
func (m *cache) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := m.cacheKey(r)

	log.Printf("Key: %s", key)

	if r.Method == "DELETE" {
		result := m.cache.Delete(key)
		log.Printf("delete: ")
		log.Printf("%t", result)
		w.WriteHeader(204)
		_, _ = w.Write([]byte{})
		
		return
	}

	cs := cacheMissStatus

	if m.bypassingHeaders(r) {
		rw := &responseWriter{ResponseWriter: w}
		m.next.ServeHTTP(rw, r)
		
		return
	}

	b, err := m.cache.Get(key)
	if err == nil {
		var data cacheData

		err := json.Unmarshal(b, &data)
		if err != nil {
			cs = cacheErrorStatus
		} else {
			m.sendCacheFile(w, data)
			return
		}
	}

	if m.cfg.AddStatusHeader {
		w.Header().Set(cacheHeader, cs)
	}

	rw := &responseWriter{ResponseWriter: w}
	m.next.ServeHTTP(rw, r)

	expiry, ok := m.cacheable(r, w, rw.status)
	if !ok {
		return
	}

	data := cacheData{
		Status:  rw.status,
		Headers: w.Header(),
		Body:    rw.body,
	}

	b, err = json.Marshal(data)
	if err != nil {
		log.Printf("Error serializing cache item: %v", err)
	}

	if err = m.cache.Set(key, b, expiry); err != nil {
		log.Printf("Error setting cache item: %v", err)
	}
}

func (m *cache) cacheable(r *http.Request, w http.ResponseWriter, status int) (time.Duration, bool) {
	reasons, expireBy, err := cachecontrol.CachableResponseWriter(r, status, w, cachecontrol.Options{})
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

func (m *cache) sendCacheFile(w http.ResponseWriter, data cacheData) {
	for key, vals := range data.Headers {
		for _, val := range vals {
			w.Header().Add(key, val)
		}
	}
	
	if m.cfg.AddStatusHeader {
		w.Header().Set(cacheHeader, cacheHitStatus)
	}

	w.WriteHeader(data.Status)
	_, _ = w.Write(data.Body)
}

func (m *cache) bypassingHeaders(r *http.Request) bool {
	for _, header := range m.cfg.BypassHeaders {
		if r.Header.Get(header) != "" {
			return true
		}
	}

	return false
}

func (m *cache) cacheKey(r *http.Request) string {
	key := r.URL.Path
	if !m.cfg.Key.DisableMethod {
		key += "-" + r.Method
	}

	log.Printf("DisableHost: %t", m.cfg.Key.DisableHost)
	if !m.cfg.Key.DisableHost {
		key += "-" + r.Host
	}
	
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
					// key += strings.ReplaceAll(format, "/", "")
					key += "-" + strings.ReplaceAll(format, " ", "")
					break out
				}
			}
		}
	}

	return key
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
