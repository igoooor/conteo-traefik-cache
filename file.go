// Package plugin_simplecache_conteo is a plugin to cache responses to disk.
package plugin_simplecache_conteo

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var errCacheMiss = errors.New("cache miss")

type fileCache struct {
	path   string
	pm     *pathMutex
	items  map[string]CacheItem
	memory bool
}

type CacheItem struct {
	Value        string `json:"v"`
	Created      uint64 `json:"c"`
	Expiry       uint64 `json:"e"`
	LastAccessed uint64 `json:"l"`
}

func newFileCache(path string, vacuum time.Duration, memory bool) (*fileCache, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("invalid cache path: %w", err)
	}

	if !info.IsDir() {
		return nil, errors.New("path must be a directory")
	}

	fc := &fileCache{
		path:   path,
		pm:     &pathMutex{lock: map[string]*fileLock{}},
		items:  map[string]CacheItem{},
		memory: memory,
	}

	go fc.vacuum(vacuum)

	return fc, nil
}

func (c *fileCache) vacuum(interval time.Duration) {
	timer := time.NewTicker(interval)
	defer timer.Stop()

	for range timer.C {
		log.Println(">>> vaccum file cache")
		_ = filepath.Walk(c.path, c.vacuumFile)
	}
}

func (c *fileCache) vacuumFile(path string, info os.FileInfo, err error) error {
	switch {
	case err != nil:
		return err
	case info.IsDir():
		return nil
	}

	mu := c.pm.MutexAt(filepath.Base(path))
	mu.Lock()
	defer mu.Unlock()

	rawData, err := ioutil.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil
	}

	data := CacheItem{}
	err = json.Unmarshal([]byte(rawData), &data)
	if err != nil {
		return nil
	}

	expires := time.Unix(int64(data.Expiry), 0)
	if !expires.Before(time.Now()) {
		return nil
	}

	_ = os.Remove(path)
	return nil
}

func (c *fileCache) readFromMemory(path string) (CacheItem, bool) {
	var data = CacheItem{}
	if c.memory {
		var ok bool
		data, ok = c.items[path]
		if ok {
			log.Printf(">>>>>>>>>>>>>>>>>>> in-memory cache hit")
			return data, true
		}
	}

	return data, false
}

func (c *fileCache) Get(key string) ([]byte, error) {
	mu := c.pm.MutexAt(key)
	mu.RLock()
	defer mu.RUnlock()

	p := keyPath(c.path, key)
	var data = CacheItem{}
	data, foundInMemory := c.readFromMemory(p)

	if !foundInMemory {
		if info, err := os.Stat(p); err != nil || info.IsDir() {
			return nil, errCacheMiss
		}

		rawData, err := ioutil.ReadFile(filepath.Clean(p))
		if err != nil {
			return nil, fmt.Errorf("error reading file %q: %w", p, err)
		}

		err = json.Unmarshal([]byte(rawData), &data)
		if err != nil {
			_ = os.Remove(p)
			return nil, errCacheMiss
		}
		log.Printf(">>>>>>>>>>>>>>>>>>> file cache hit")
	}

	expires := time.Unix(int64(data.Expiry), 0)
	if expires.Before(time.Now()) {
		_ = os.Remove(p)
		return nil, errCacheMiss
	}

	// store it back into memory
	if c.memory && !foundInMemory {
		c.items[p] = data
	}

	return []byte(data.Value), nil
}

func (c *fileCache) DeleteAll(flushType string) {
	if flushType == "all" || flushType == "file" {
		log.Println(">>> delete file cache")
		_ = filepath.Walk(c.path, c.deleteFile)
	}
	if c.memory && (flushType == "all" || flushType == "memory") {
		c.items = map[string]CacheItem{}
	}
}

func (c *fileCache) deleteFile(path string, info os.FileInfo, err error) error {
	switch {
	case err != nil:
		return err
	case info.IsDir():
		return nil
	}

	mu := c.pm.MutexAt(filepath.Base(path))
	mu.Lock()
	defer mu.Unlock()

	rawData, err := ioutil.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil
	}

	data := CacheItem{}
	err = json.Unmarshal([]byte(rawData), &data)
	if err != nil {
		return nil
	}

	_ = os.Remove(path)
	return nil
}

func (c *fileCache) Set(key string, val []byte, expiry time.Duration) error {
	mu := c.pm.MutexAt(key)
	mu.Lock()
	defer mu.Unlock()

	p := keyPath(c.path, key)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return fmt.Errorf("error creating file path: %w", err)
	}

	f, err := os.OpenFile(filepath.Clean(p), os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return fmt.Errorf("error creating file: %w", err)
	}

	defer func() {
		_ = f.Close()
	}()

	//timestamp := uint64(time.Now().Add(expiry).Unix())

	//var t [8]byte

	/*binary.LittleEndian.PutUint64(t[:], timestamp)

	if _, err = f.Write(t[:]); err != nil {
		return fmt.Errorf("error writing file: %w", err)
	}*/

	nowTimestamp := uint64(time.Now().Unix())

	item := &CacheItem{
		Value:        string(val),
		Created:      nowTimestamp,
		Expiry:       uint64(time.Now().Add(expiry).Unix()),
		LastAccessed: nowTimestamp,
	}

	jsonData, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("error json marshal: %w", err)
	}

	if _, err = f.Write(jsonData); err != nil {
		return fmt.Errorf("error writing file: %w", err)
	}

	if c.memory {
		c.items[p] = *item
	}

	return nil
}

func keyHash(key string) [4]byte {
	h := crc32.Checksum([]byte(key), crc32.IEEETable)

	var b [4]byte

	binary.LittleEndian.PutUint32(b[:], h)

	return b
}

func keyPath(path, key string) string {
	h := keyHash(key)
	key = strings.NewReplacer("/", "-", ":", "_").Replace(key)

	return filepath.Join(
		path,
		hex.EncodeToString(h[0:1]),
		hex.EncodeToString(h[1:2]),
		hex.EncodeToString(h[2:3]),
		hex.EncodeToString(h[3:4]),
		key,
	)
}

type pathMutex struct {
	mu   sync.Mutex
	lock map[string]*fileLock
}

func (m *pathMutex) MutexAt(path string) *fileLock {
	m.mu.Lock()
	defer m.mu.Unlock()

	if fl, ok := m.lock[path]; ok {
		fl.ref++
		log.Println(">>> Lock exists, fl.ref: ", fl.ref)
		return fl
	}

	fl := &fileLock{ref: 1}
	fl.cleanup = func() {
		m.mu.Lock()
		defer m.mu.Unlock()

		fl.ref--
		log.Println(">>> Lock cleanup, path: ", path)
		log.Println(">>> Lock cleanup, fl.ref: ", fl.ref)
		if fl.ref == 0 {
			log.Println(">>> ref = 0, delete lock: ", path)
			delete(m.lock, path)
		}
	}
	m.lock[path] = fl
	log.Println(">>> Lock: ", path)
	log.Println(">>> fl.ref: ", fl.ref)

	return fl
}

type fileLock struct {
	ref     int
	cleanup func()

	mu sync.RWMutex
}

func (l *fileLock) RLock() {
	l.mu.RLock()
}

func (l *fileLock) RUnlock() {
	l.mu.RUnlock()
	l.cleanup()
}

func (l *fileLock) Lock() {
	l.mu.Lock()
}

func (l *fileLock) Unlock() {
	l.mu.Unlock()
	l.cleanup()
}
