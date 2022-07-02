package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	provider "github.com/igoooor/conteo-traefik-cache/provider/api"
	"github.com/igoooor/conteo-traefik-cache/provider/local"
	// "github.com/xujiajun/nutsdb"
)

type CacheSystem interface {
	Get(string, string) ([]byte, bool, error)
	DeleteAll(string)
	Delete(string)
	Set(string, []byte, time.Duration, string) error
	Check(bool) bool
}

// provider "github.com/igoooor/conteo-traefik-cache/provider/api"
// provider "github.com/igoooor/conteo-traefik-cache/provider/badger"
// provider "github.com/igoooor/conteo-traefik-cache/provider/local"
// provider "github.com/igoooor/conteo-traefik-cache/provider/nutsdb"

func main() {
	var cache CacheSystem
	cache, err := provider.NewFileCache("http://localhost:8081")
	available := cache.Check(true)
	log.Printf("available: %v", available)
	if err != nil {
		log.Println("Main cache not available, using local cache")
		cache, err = local.NewFileCache("cache", time.Duration(60)*time.Second, false)
		if err != nil {
			return
		}
	}

	cache.Set("test", []byte("test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1"), time.Duration(60)*time.Second, "")

	val, _, err := cache.Get("yoloo", "")
	if err != nil {
		message := err.Error()
		log.Println(message)
		if strings.Contains(message, "connect: connection refused") {
			log.Println("connect: connection refused")
		} else {
			log.Println("yolo")
		}
		// fmt.Printf("%v\n", err)
		return
	}
	fmt.Println(string(val))
}

func mainOld() {
	fmt.Println("Hello World")

	var cache CacheSystem
	// cache, err := provider.NewFileCache("/Users/igorweigel/webpages/conteo-traefik-cache/cdn", time.Duration(60)*time.Second, false)
	cache, err := provider.NewFileCache("/Users/igorweigel/webpages/conteo-traefik-cache/cdn" /*, time.Duration(60)*time.Second, false*/)
	if err != nil {
		return
	}

	for n := 0; n < 1000000; n++ {
		cache.Set("test"+strconv.Itoa(n), []byte("test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1test1"), time.Duration(5)*time.Second, "")
		// cache.Set("test", []byte("test2"), time.Duration(60)*time.Second)

		/*val, err := cache.Get("test")
		if err != nil {
			return
		}
		fmt.Println(string(val))*/

	}
	log.Printf("done writing")

	for n := 0; n < 10; n++ {
		val, _, err := cache.Get("test"+strconv.Itoa(n), "")
		if err != nil {
			fmt.Printf("%v\n", err)
			continue
		}
		fmt.Println(string(val))
	}
	// cache.DeleteAll("")

	return
	// cache.Delete("test6")
	// cache.Delete("test6")
	cache.DeleteAll("")
	// cache.DeleteAll("")
	val, _, err := cache.Get("test", "")
	if err != nil {
		// fmt.Printf("%v\n", err)
		return
	}
	fmt.Println(string(val))
}

/*
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

	db.Update(
	func(tx *nutsdb.Tx) error {
		return tx.Put(bucket, key, []byte("yolo"), uint32(60))
	})

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
*/
