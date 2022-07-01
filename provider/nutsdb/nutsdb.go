// Package plugin_simplecache_conteo is a plugin to cache responses to disk.
package nutsdb

import (
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/xujiajun/nutsdb"
)

var errCacheMiss = errors.New("cache miss")

type FileCache struct {
	NutsDB *nutsdb.DB
	bucket string
}

func NewFileCache(path string, vacuum time.Duration, memory bool) (*FileCache, error) {
	err := os.MkdirAll(path, os.ModePerm)
	if err != nil {
		return nil, fmt.Errorf("invalid cache path: %w", err)
	}

	opt := nutsdb.DefaultOptions
	opt.Dir = path
	opt.SegmentSize = 16 * 1024 * 1024 // 16MB
	// opt.SyncEnable = false
	//opt.RWMode = nutsdb.MMap
	//opt.StartFileLoadingMode = nutsdb.FileIO
	nutsDB, err := nutsdb.Open(opt)
	if err != nil {
		panic(err)
	}

	fc := &FileCache{
		NutsDB: nutsDB,
		bucket: "bucket1",
	}

	return fc, nil
}

func (c *FileCache) Get(key string) ([]byte, error) {
	var err error
	var entry *nutsdb.Entry
	err = c.NutsDB.View(
		func(tx *nutsdb.Tx) error {
			entry, err = tx.Get(c.bucket, []byte(key))
			return err
		})

	if entry == nil {
		return nil, err
	} else {
		return entry.Value, nil
	}
}

func (c *FileCache) DeleteAll(flushType string) {
	if err := c.NutsDB.Update(
		func(tx *nutsdb.Tx) error {
			return tx.DeleteBucket(nutsdb.DataStructureBPTree, c.bucket)
		}); err != nil {
		log.Fatal(err)
	}
}

func (c *FileCache) Delete(key string) {
	if err := c.NutsDB.Update(
		func(tx *nutsdb.Tx) error {
			if err := tx.Delete(c.bucket, []byte(key)); err != nil {
				return err
			}
			return nil
		}); err != nil {
		log.Fatal(err)
	}
}

func (c *FileCache) Set(key string, val []byte, expiry time.Duration) error {
	return c.NutsDB.Update(
		func(tx *nutsdb.Tx) error {
			return tx.Put(c.bucket, []byte(key), val, uint32(expiry))
		})
}

func (c *FileCache) Close() {
	c.NutsDB.Close()
}
