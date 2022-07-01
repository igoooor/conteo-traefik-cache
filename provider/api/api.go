// Package conteo_traefik_cache is a plugin to cache responses to disk.
package api

import (
	"encoding/base64"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type FileCache struct {
	path   string
	status bool
}

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

func (c *FileCache) Get(key string) ([]byte, error) {
	response, err := http.Get(c.path + encodeKey(key))

	if err != nil {
		return nil, err
	}

	if response.StatusCode != http.StatusOK {
		return nil, nil
	}

	responseData, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	return responseData, nil
}

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

func (c *FileCache) Delete(key string) {

}

func (c *FileCache) Set(key string, val []byte, expiry time.Duration) error {
	req, err := http.NewRequest(http.MethodPut, c.path+encodeKey(key), strings.NewReader(string(val)))
	if err != nil {
		return err
	}

	req.Header.Set("X-TTL", strconv.Itoa(int(expiry.Seconds())))
	client := &http.Client{}

	_, err = client.Do(req)
	if err != nil {
		return err
	}

	return nil
}

func (c *FileCache) Close() {

}
