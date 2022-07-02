// Package api is a api cache
package api

import (
	"encoding/base64"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Cache DB implementation
type FileCache struct {
	path   string
	status bool
}

// NewFileCache creates a new FileCache instance.
func NewFileCache(path string) (*FileCache, error) {
	fc := &FileCache{
		path: strings.TrimSuffix(path, "/") + "/",
	}

	// temporarily disable local backup if api not available
	/*_, err := http.Get(fc.path)
	if err != nil {
		return nil, err
	}*/
	_, err := http.Get(fc.path)
	fc.status = err == nil

	return fc, nil
}

// Check availability of cache system
func (c *FileCache) Check(refresh bool) bool {
	if refresh {
		// _, err := http.Get(c.path)
		// c.status = err == nil

		req, err := http.NewRequest(http.MethodGet, c.path+"ping", nil)
		if err != nil {
			return false
		}

		client := &http.Client{}
		req.Host = "ping"

		_, err = client.Do(req)
		c.status = err == nil
	}
	return c.status
}

func encodeKey(key string) string {
	return base64.URLEncoding.EncodeToString([]byte(key))
}

// Get returns the value for the given key.
func (c *FileCache) Get(key string, etag string) ([]byte, bool, error) {
	req, err := http.NewRequest(http.MethodGet, c.path+encodeKey(key), nil)
	if err != nil {
		return nil, false, err
	}

	req.Header.Set("X-Etag", etag)
	client := &http.Client{}

	response, err := client.Do(req)
	if err != nil {
		return nil, false, err
	}

	if response.StatusCode != http.StatusOK {
		if response.StatusCode == http.StatusNotModified {
			return nil, true, nil
		}
		return nil, false, nil
	}

	responseData, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, false, err
	}

	return responseData, false, nil
}

// DeleteAll deletes all keys in the cache.
func (c *FileCache) DeleteAll(flushType string) {
	req, err := http.NewRequest(http.MethodDelete, c.path+flushType, nil)
	if err != nil {
		return
	}

	client := &http.Client{}

	_, err = client.Do(req)
	if err != nil {
		return
	}

	return
}

// Delete deletes the given key from the cache.
func (c *FileCache) Delete(key string) {

}

// Set sets the value for the given key.
func (c *FileCache) Set(key string, val []byte, expiry time.Duration, etag string) error {
	req, err := http.NewRequest(http.MethodPut, c.path+encodeKey(key), strings.NewReader(string(val)))
	if err != nil {
		return err
	}

	req.Header.Set("X-TTL", strconv.Itoa(int(expiry.Seconds())))
	req.Header.Set("X-Etag", etag)
	client := &http.Client{}

	_, err = client.Do(req)
	if err != nil {
		return err
	}

	return nil
}
