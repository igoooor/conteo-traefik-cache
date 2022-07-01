package main

import (
	"fmt"
	"log"
	"strconv"
	"time"

	provider "github.com/igoooor/plugin-simplecache-conteo/provider/badger"
	"github.com/xujiajun/nutsdb"
)

type CacheSystem interface {
	Get(string) ([]byte, error)
	DeleteAll(string)
	Delete(string)
	Set(string, []byte, time.Duration) error
	Close()
}

// provider "github.com/igoooor/plugin-simplecache-conteo/provider/badger"
// provider "github.com/igoooor/plugin-simplecache-conteo/provider/local"
// provider "github.com/igoooor/plugin-simplecache-conteo/provider/nutsdb"

func main() {
	fmt.Println("Hello World")

	var cache CacheSystem
	cache, err := provider.NewFileCache("/Users/igorweigel/webpages/plugin-simplecache/cdn", time.Duration(60)*time.Second, false)
	if err != nil {
		return
	}

	defer cache.Close()

	for n := 0; n < 1000000; n++ {
		cache.Set("test"+strconv.Itoa(n), []byte("test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1"), time.Duration(5)*time.Second)
		//cache.Set("test", []byte("test2"), time.Duration(60)*time.Second)

		/*val, err := cache.Get("test")
		if err != nil {
			return
		}
		fmt.Println(string(val))*/

	}
	log.Printf("done writing")

	for n := 0; n < 10; n++ {
		val, err := cache.Get("test" + strconv.Itoa(n))
		if err != nil {
			fmt.Printf("%v\n", err)
			continue
		}
		fmt.Println(string(val))
	}
	//cache.DeleteAll("")

	return
	//cache.Delete("test6")
	//cache.Delete("test6")
	cache.DeleteAll("")
	//cache.DeleteAll("")
	val, err := cache.Get("test")
	if err != nil {
		//fmt.Printf("%v\n", err)
		return
	}
	fmt.Println(string(val))
}

func mainWhat() {
	// Open the database located in the /tmp/nutsdb directory.
	// It will be created if it doesn't exist.
	opt := nutsdb.DefaultOptions
	opt.Dir = "/tmp/nutsdb"
	db, err := nutsdb.Open(opt)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	key := []byte("name12")
	bucket := "bucket1"

	/*db.Update(
	func(tx *nutsdb.Tx) error {
		return tx.Put(bucket, key, []byte("yolo"), uint32(60))
	})*/

	if err := db.View(
		func(tx *nutsdb.Tx) error {
			if e, err := tx.Get(bucket, key); err != nil {
				return err
			} else {
				fmt.Println(string(e.Value)) // "val1-modify"
			}
			return nil
		}); err != nil {
		log.Println(err)
	}
}
