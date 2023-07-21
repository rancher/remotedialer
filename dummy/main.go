package main

import (
	"flag"
	"log"
	"net/http"
	"sync/atomic"
	"time"
)

var (
	counter int64
	listen  string
)

func main() {
	flag.StringVar(&listen, "listen", ":8125", "Listen address")
	flag.Parse()

	log.Println("listening ", listen)
	err := http.ListenAndServe(listen, http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		start := time.Now()
		next := atomic.AddInt64(&counter, 1)
		http.FileServer(http.Dir("./")).ServeHTTP(rw, req)
		log.Println("request", next, time.Since(start))
	}))

	if err != nil {
		panic(err)
	}
}
