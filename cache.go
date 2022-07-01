// Package plugin_simplecache_conteo is a plugin to cache responses to disk.
package plugin_simplecache_conteo

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"

	// "regexp"
	"strings"
	"time"

	"github.com/igoooor/plugin-simplecache-conteo/provider/api"
	// "github.com/igoooor/plugin-simplecache-conteo/provider/local"
	"github.com/pquerna/cachecontrol"
)

func main() {
	log.Println("Hello World")
}

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
	BypassHeaders   []string   `json:"bypassHeaders" yaml:"bypassHeaders" toml:"bypassHeaders"`
	Key             KeyContext `json:"key" yaml:"key" toml:"key"`
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
		BypassHeaders:   []string{},
		Key:             KeyContext{},
	}
}

const (
	cacheHeader      = "Cache-Status"
	ageHeader        = "Age"
	cacheHitStatus   = "hit; ttl=%d"
	cacheMissStatus  = "miss"
	cacheErrorStatus = "error"
	acceptHeader     = "Accept"
)

/*type CacheSystem interface {
	Get(string) ([]byte, error)
	DeleteAll(string)
	Delete(string)
	Set(string, []byte, time.Duration) error
	Close()
}*/

type cache struct {
	name  string
	cache api.FileCache
	cfg   *Config
	next  http.Handler
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
	// instead of:
	//var fc CacheSystem
	//var err error
	//fc, err = api.NewFileCache(cfg.Path)
	//if err != nil {
	//	log.Println("Main cache not available, using local cache")
	//	fc, err = local.NewFileCache(cfg.Path, time.Duration(cfg.Cleanup)*time.Second, cfg.Memory)
	//	if err != nil {
	//		return nil, err
	//	}
	//}

	/*keysRegexp := make(map[string]keysRegexpInner, len(cfg.SurrogateKeys))
	// baseRegexp := regexp.MustCompile(".+")

	for key, regexps := range cfg.SurrogateKeys {
		headers := make(map[string]*regexp.Regexp, len(regexps.Headers))
		for hk, hv := range regexps.Headers {
			//headers[hk] = baseRegexp
			headers[hk] = nil
			if hv != "" {
				headers[hk] = regexp.MustCompile(hv)
			}
		}

		//innerKey := keysRegexpInner{Headers: headers, Url: baseRegexp}
		innerKey := keysRegexpInner{Headers: headers, Url: nil}

		if regexps.URL != "" {
			innerKey.Url = regexp.MustCompile(regexps.URL)
		}

		keysRegexp[key] = innerKey
	}*/

	m := &cache{
		name:  name,
		cache: *fc,
		cfg:   cfg,
		next:  next,
		//keysRegexp: keysRegexp,
	}

	return m, nil
}

type cacheData struct {
	Status  int
	Headers map[string][]string
	Body    []byte
	Created uint64
	Expiry  uint64
}

// ServeHTTP serves an HTTP request.
func (m *cache) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := m.cacheKey(r)

	if r.Method == "DELETE" {
		w.WriteHeader(204)
		_, _ = w.Write([]byte{})
		m.flushAllCache(r)

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

	// log.Println("--------- cache miss:", key)
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
		Created: uint64(time.Now().Unix()),
		Expiry:  uint64(time.Now().Add(expiry).Unix()),
	}

	b, err = json.Marshal(data)
	if err != nil {
		log.Printf("Error serializing cache item: %v", err)
	}

	// matchSurrogateKeys := m.matchSurrogateKeys(r)

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

func (m *cache) flushAllCache(r *http.Request) {
	if flushType := r.Header.Get(m.cfg.FlushHeader); flushType != "" {
		m.cache.DeleteAll(flushType)
	}
}

func (m *cache) sendCacheFile(w http.ResponseWriter, data cacheData) {
	for key, vals := range data.Headers {
		for _, val := range vals {
			w.Header().Add(key, val)
		}
	}

	if m.cfg.AddStatusHeader {
		now := uint64(time.Now().Unix())
		age := now - data.Created
		ttl := data.Expiry - now
		w.Header().Set(cacheHeader, fmt.Sprintf(cacheHitStatus, ttl))
		w.Header().Set(ageHeader, strconv.FormatUint(age, 10))
	}

	w.WriteHeader(data.Status)
	_, _ = w.Write(data.Body)
}

func (m *cache) bypassingHeaders(r *http.Request) bool {
	if r.Header.Get("Cache-Control") == "no-cache" {
		return true
	}

	for _, header := range m.cfg.BypassHeaders {
		if r.Header.Get(header) != "" {
			return true
		}
	}

	return false
}

/*func (m *cache) matchSurrogateKeys(r *http.Request) []string {
	matchKeys := []string{}

	return matchKeys
}*/

func (m *cache) cacheKey(r *http.Request) string {
	key := ""
	if !m.cfg.Key.DisableMethod {
		key += "-" + r.Method
	}

	if !m.cfg.Key.DisableHost {
		key += "-" + r.Host
	}

	key += "-" + r.URL.Path

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
