package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	provider "github.com/igoooor/conteo-traefik-cache/provider/api"
	"github.com/igoooor/conteo-traefik-cache/provider/local"
	// "github.com/xujiajun/nutsdb"
)

type CacheSystem interface {
	Get(string, string) ([]byte, bool, error)
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
