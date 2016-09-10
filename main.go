package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"runtime"
	"sync"
	"time"
)

type debugVars struct {
	Cmdline  []string
	Memstats runtime.MemStats
}

type debugStats struct {
	SampleTime    time.Time
	Cmd           string
	Alloc         uint64
	Sys           uint64
	HeapAlloc     uint64
	HeapInuse     uint64
	GCCPUFraction float64
}

type debugBuffer struct {
	sync.RWMutex
	currentPos int
	length     int
	data       []debugStats
}

func NewDebugBuffer(length int) *debugBuffer {
	return &debugBuffer{currentPos: 0, length: length, data: make([]debugStats, length, length)}
}

func (dBuf *debugBuffer) Add(dStats debugStats) {
	dBuf.Lock()
	defer dBuf.Unlock()
	dBuf.data[dBuf.currentPos] = dStats
	dBuf.currentPos = (dBuf.currentPos + 1) % dBuf.length
}

func (dBuf *debugBuffer) Read() []debugStats {
	dBuf.RLock()
	defer dBuf.RUnlock()
	return append(dBuf.data[dBuf.currentPos:], dBuf.data[:dBuf.currentPos]...)
}

var (
	monitoringHost  = flag.String("monhost", "localhost", "Host to monitor")
	monitoringPort  = flag.Int64("monport", 8123, "Port to monitor")
	port            = flag.Int64("port", 8080, "Port to serve expvar monitoring from")
	pollingInterval = flag.Duration("pollingint", 5*time.Second, "Interval to poll host at")
	sampleStats     = NewDebugBuffer(10)
)

func main() {

	flag.Parse()

	log.Printf("Monitoring application at http://%s:%d/debug/vars", *monitoringHost, *monitoringPort)
	log.Printf("Starting http server on port %v", *port)
	log.Printf("Raw endpoint at http://localhost:%v/raw", *port)
	log.Printf("Processed endpoint at http://localhost:%v/processed", *port)

	ticker := time.NewTicker(*pollingInterval)

	go updateStats(ticker)

	http.HandleFunc("/raw", func(w http.ResponseWriter, r *http.Request) {

		if resp, err := http.Get(fmt.Sprintf("http://%s:%d/debug/vars", monitoringHost, monitoringPort)); err != nil {
			log.Println(err)
			fmt.Fprintf(w, "Error reading http://%s:%d/debug/vars\n %v", monitoringHost, monitoringPort, err)
		} else {
			if _, err := io.Copy(w, resp.Body); err != nil {
				log.Println(err)
			}
		}
	})

	http.HandleFunc("/processed", func(w http.ResponseWriter, r *http.Request) {
		stats, err := getStats("localhost", 8123)
		if err != nil {
			log.Println(err)
			fmt.Fprintf(w, "Error reading http://%s:%d/debug/vars\n %v", monitoringHost, monitoringPort, err)

		} else {
			fmt.Fprintf(w, "cmd: %v\nalloc: %v\nsys: %v\nheap alloc: %v\nheap in use: %v\nGC CPU use: %v\nGC pause time: %v\n", stats.Cmdline[0],
				stats.Memstats.Alloc, stats.Memstats.Sys, stats.Memstats.HeapAlloc, stats.Memstats.HeapInuse,
				stats.Memstats.GCCPUFraction, stats.Memstats.PauseNs[(stats.Memstats.NumGC+255)%256])
		}
	})

	http.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		if data, err := json.Marshal(sampleStats.Read()); err != nil {
			log.Println(err)
			fmt.Fprintf(w, "Error marshalling stats: %v", err)
		} else {
			fmt.Fprintf(w, string(data))
		}

	})

	http.Handle("/", http.StripPrefix("/", http.FileServer(http.Dir("./static/"))))

	log.Fatalln(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))

}

func getStats(host string, port int64) (*debugVars, error) {
	url := fmt.Sprintf("http://%s:%d/debug/vars", host, port)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var stats debugVars
	if err := json.Unmarshal(data, &stats); err != nil {
		return nil, err
	}
	return &stats, nil
}

func updateStats(ticker *time.Ticker) {
	for t := range ticker.C {
		debugVars, err := getStats(*monitoringHost, *monitoringPort)
		if err != nil {
			log.Printf("Error updating stats: %v", err)
			return
		}
		sampleStats.Add(debugStats{
			SampleTime:    t,
			Cmd:           debugVars.Cmdline[0],
			Alloc:         debugVars.Memstats.Alloc,
			Sys:           debugVars.Memstats.Sys,
			HeapAlloc:     debugVars.Memstats.HeapAlloc,
			HeapInuse:     debugVars.Memstats.HeapInuse,
			GCCPUFraction: debugVars.Memstats.GCCPUFraction,
		})
	}
}
