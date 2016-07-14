package main

import (
	"flag"
	"io/ioutil"
	"log"
	"os"

	"gopkg.in/yaml.v2"

	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

var (
	configPath = flag.String("config", "config.yml", "Path to config YAML file.")
	verbose = flag.Bool("verbose", false, "Log more information")
)

type Config struct {
	Server   struct{ Bind string }
	Interval int
	Targets  []string
}

func main() {
	flag.Parse()

	configFile, err := os.Open(*configPath)
	if err != nil {
		log.Fatalf("Failed to open config file at path %s due to error: %s", *configPath, err.Error())
	}
	defer configFile.Close()

	configData, err := ioutil.ReadAll(configFile)
	if err != nil {
		log.Fatalf("Failed to read config file at path %s due to error: %s", *configPath, err.Error())
	}

	config := &Config{}
	if err := yaml.Unmarshal(configData, config); err != nil {
		log.Fatalf("Failed to unmarshal YAML data in config: %s", err.Error())
	}

	aggregator := &Aggregator{HTTP: &http.Client{Timeout: 5 * time.Second}}
	go aggregator.Start(config.Targets, time.Duration(config.Interval) * time.Second)

	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(rw http.ResponseWriter, r *http.Request) {
		aggregator.Export(rw)
	})

	log.Printf("Starting server on %s...", config.Server.Bind)
	log.Fatal(http.ListenAndServe(config.Server.Bind, mux))
}

type Aggregator struct {
	HTTP          *http.Client
	aggregateData [][]byte
	sync.RWMutex
}

func (f *Aggregator) Export(w io.Writer) {
	if f.aggregateData == nil {
		return
	}
	f.RLock()
	for _, d := range f.aggregateData {
		if len(d) > 0 {
			w.Write(d)
			w.Write([]byte("\n"))
		}
	}
	f.RUnlock()
}

func (f *Aggregator) Start(targets []string, interval time.Duration) {

	log.Print("Target list:")
	for tk, t := range targets {
		log.Printf("%d. %s", tk, t)
	}

	f.aggregateData = make([][]byte, len(targets), len(targets))

	//initial refresh
	f.fetchAll(targets)

	//start updating
	ticker := time.NewTicker(interval)
	for {
		select {
		case <-ticker.C:
			f.fetchAll(targets)
		}
	}
}

func (f *Aggregator) fetchAll(targets []string) {
	wait := sync.WaitGroup{}
	for targetKey, target := range targets {
		wait.Add(1)
		go func(key int, url string) {
			startTime := time.Now()
			res, err := f.fetch(url)
			if err == nil {
				f.Lock()
				f.aggregateData[key] = res
				f.Unlock()
				if *verbose {
					log.Printf("OK: %s was refreshed in %.3f seconds", url, time.Since(startTime).Seconds())
				}
			} else {
				log.Printf("ERROR: %s", err.Error())
			}

			wait.Done()
		}(targetKey, target)

	}
	wait.Wait()
}

func (f *Aggregator) fetch(target string) ([]byte, error) {
	res, err := f.HTTP.Get(target)
	if err != nil {
		return []byte(""), fmt.Errorf("Failed to fetch URL %s due to error: %s", target, err.Error())
	}
	defer res.Body.Close()
	return ioutil.ReadAll(res.Body)
}
