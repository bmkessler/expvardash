package main

import (
	_ "expvar"
	"flag"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"time"
)

var (
	delay  = flag.Duration("delay", 1*time.Second, "Interval to allocate memory at")
	amount = flag.Int64("amount", 1000, "Bytes to allocate per interval")
	total  = flag.Int64("total", 30000, "Total bytes to allocate before resetting")
	port   = flag.Int64("port", 8123, "Port to serve expvar monitoring from")
)

func main() {
	flag.Parse()
	go allocate()
	log.Printf("Allocating %d bytes every %v up to a total of %d\n", *amount, *delay, *total)
	log.Printf("Serving expvar data on port %d\n", *port)
	log.Fatalln(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
}

func allocate() {
	N := int(*total / *amount)
	buffer := make([]*[]byte, N, N)
	var i int
	for {
		time.Sleep(*delay)
		if i == N { // free the memory
			i = 0
			for j := 0; j < N; j++ {
				buffer[i] = nil
			}
			runtime.GC()
		}
		allocatedBytes := make([]byte, int(*amount), int(*amount))
		buffer[i] = &allocatedBytes
		i++
	}
}
