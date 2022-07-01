// Package plugin_simplecache_conteo is a plugin to cache responses to disk.
package local

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var errCacheMiss = errors.New("cache miss")

type FileCache struct {
	path   string
	pm     *PathMutex
	items  map[string][]byte
	memory bool
}

func NewFileCache(path string, vacuum time.Duration, memory bool) (*FileCache, error) {
	err := os.MkdirAll(path, os.ModePerm)
	if err != nil {
		return nil, fmt.Errorf("invalid cache path: %w", err)
	}

	fc := &FileCache{
		path:   path,
		pm:     &PathMutex{Lock: map[string]*FileLock{}},
		items:  map[string][]byte{},
		memory: memory,
	}

	go fc.vacuum(vacuum)

	return fc, nil
}

func (c *FileCache) vacuum(interval time.Duration) {
	timer := time.NewTicker(interval)
	defer timer.Stop()

	for range timer.C {
		// log.Println(">>> vaccum file cache")
		_ = filepath.Walk(c.path, c.vacuumFile)
	}
}

func (c *FileCache) vacuumFile(path string, info os.FileInfo, err error) error {
	switch {
	case err != nil:
		return err
	case info.IsDir():
		return nil
	}

	mu := c.pm.MutexAt(filepath.Base(path))
	mu.Lock()
	defer mu.Unlock()

	data, err := readFile(path)
	if err != nil {
		return nil
	}

	expires := time.Unix(int64(binary.LittleEndian.Uint64(data[:8])), 0)
	if !expires.Before(time.Now()) {
		return nil
	}

	_ = os.Remove(path)
	return nil
}

func (c *FileCache) readFromMemory(path string) ([]byte, bool) {
	var data = []byte{}
	if c.memory {
		var ok bool
		data, ok = c.items[path]
		if ok {
			// log.Printf(">>>>>>>>>>>>>>>>>>> in-memory cache hit")
			return data, true
		}
	}

	return data, false
}

func (c *FileCache) Get(key string) ([]byte, error) {
	mu := c.pm.MutexAt(key)
	mu.RLock()
	defer mu.RUnlock()

	p := keyPath(c.path, key)
	var data = []byte{}
	data, foundInMemory := c.readFromMemory(p)

	if !foundInMemory {
		if info, err := os.Stat(p); err != nil || info.IsDir() {
			return nil, errCacheMiss
		}

		var err error
		data, err = readFile(p)
		if err != nil {
			return nil, err
		}

		// log.Printf(">>>>>>>>>>>>>>>>>>> file cache hit")
	}

	expires := time.Unix(int64(binary.LittleEndian.Uint64(data[:8])), 0)
	if expires.Before(time.Now()) {
		_ = os.Remove(p)
		return nil, errCacheMiss
	}

	// store it back into memory
	if c.memory && !foundInMemory {
		c.items[p] = data
	}

	return data[8:], nil
}

func (c *FileCache) DeleteAll(flushType string) {
	if flushType == "all" || flushType == "file" {
		// log.Println(">>> delete file cache")
		_ = filepath.Walk(c.path, c.deleteFile)
	}
	if c.memory && (flushType == "all" || flushType == "memory") {
		c.items = map[string][]byte{}
	}
}

func (c *FileCache) Delete(path string) {
	mu := c.pm.MutexAt(filepath.Base(path))
	mu.Lock()
	defer mu.Unlock()

	_, err := readFile(path)
	if err == nil {
		_ = os.Remove(path)
	}
}

func (c *FileCache) deleteFile(path string, info os.FileInfo, err error) error {
	switch {
	case err != nil:
		return err
	case info.IsDir():
		return nil
	}

	c.Delete(path)

	return nil
}

func readFile(path string) ([]byte, error) {
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return nil, errCacheMiss
	}

	b, err := ioutil.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("error reading file %q: %w", path, err)
	}

	return b, nil

}

func (c *FileCache) Set(key string, val []byte, expiry time.Duration) error {
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

	timestamp := uint64(time.Now().Add(expiry).Unix())

	var t [8]byte

	binary.LittleEndian.PutUint64(t[:], timestamp)

	if _, err = f.Write(t[:]); err != nil {
		return fmt.Errorf("error writing file: %w", err)
	}

	if _, err = f.Write(val); err != nil {
		return fmt.Errorf("error writing file: %w", err)
	}

	if c.memory {
		c.items[p] = append(t[:], val[:]...)
	}

	return nil
}

func (c *FileCache) Close() {
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

type PathMutex struct {
	mu   sync.Mutex
	Lock map[string]*FileLock
}

func (m *PathMutex) MutexAt(path string) *FileLock {
	m.mu.Lock()
	defer m.mu.Unlock()

	if fl, ok := m.Lock[path]; ok {
		fl.ref++
		// log.Println(">>> Lock exists, fl.ref: ", fl.ref)
		return fl
	}

	fl := &FileLock{ref: 1}
	fl.cleanup = func() {
		m.mu.Lock()
		defer m.mu.Unlock()

		fl.ref--
		// log.Println(">>> Lock cleanup, path: ", path)
		// log.Println(">>> Lock cleanup, fl.ref: ", fl.ref)
		if fl.ref == 0 {
			// log.Println(">>> ref = 0, delete lock: ", path)
			delete(m.Lock, path)
		}
	}
	m.Lock[path] = fl
	// log.Println(">>> Lock: ", path)
	// log.Println(">>> fl.ref: ", fl.ref)

	return fl
}

type FileLock struct {
	ref     int
	cleanup func()

	mu sync.RWMutex
}

func (l *FileLock) RLock() {
	l.mu.RLock()
}

func (l *FileLock) RUnlock() {
	l.mu.RUnlock()
	l.cleanup()
}

func (l *FileLock) Lock() {
	l.mu.Lock()
}

func (l *FileLock) Unlock() {
	l.mu.Unlock()
	l.cleanup()
}
