// Package plugin_simplecache_conteo is a plugin to cache responses to disk.
package api

import (
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type FileCache struct {
	path string
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

	return fc, nil
}

func (c *FileCache) Get(key string) ([]byte, error) {
	response, err := http.Get(c.path + key)

	if err != nil {
		return nil, err
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
	req, err := http.NewRequest(http.MethodPut, c.path+key, strings.NewReader(string(val)))
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
