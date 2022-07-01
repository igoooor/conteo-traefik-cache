// Package plugin_simplecache_conteo is a plugin to cache responses to disk.
package badger

import (
	"errors"
	"fmt"
	"os"
	"time"

	badger "github.com/dgraph-io/badger/v3"
)

var errCacheMiss = errors.New("cache miss")

type FileCache struct {
	BadgerDB *badger.DB
}

func NewFileCache(path string, vacuum time.Duration, memory bool) (*FileCache, error) {
	err := os.MkdirAll(path, os.ModePerm)
	if err != nil {
		return nil, fmt.Errorf("invalid cache path: %w", err)
	}

	opts := badger.DefaultOptions(path)
	opts.BaseTableSize = 16 << 20
	//opts.MemTableSize = 20 << 20
	//opts.Logger = nil
	//opts = opts.WithBaseTableSize(16 << 20)

	badgerDB, err := badger.Open(opts)
	if err != nil {
		panic(err)
	}

	fc := &FileCache{
		BadgerDB: badgerDB,
	}

	return fc, nil
}

func (c *FileCache) Get(key string) ([]byte, error) {
	var valCopy []byte
	err := c.BadgerDB.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}

		valCopy, err = item.ValueCopy(nil)
		if err != nil {
			return err
		}

		return nil
	})

	return valCopy, err
}

func (c *FileCache) DeleteAll(flushType string) {
	c.BadgerDB.DropAll()
}

func (c *FileCache) Delete(key string) {

}

func (c *FileCache) Set(key string, val []byte, expiry time.Duration) error {
	return c.BadgerDB.Update(func(txn *badger.Txn) error {
		e := badger.NewEntry([]byte(key), []byte(val)).WithTTL(time.Hour)
		err := txn.SetEntry(e)
		return err
	})
}

func (c *FileCache) Close() {
	c.BadgerDB.Close()
}
