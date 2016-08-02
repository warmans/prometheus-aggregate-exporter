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
	"strconv"
	"time"
)

var (
	configPath = flag.String("config", "config.yml", "Path to config YAML file.")
	verbose    = flag.Bool("verbose", false, "Log more information")
)

type Config struct {
	Server  struct{ Bind string }
	Timeout int
	Targets []string
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

	aggregator := &Aggregator{HTTP: &http.Client{Timeout: time.Duration(config.Timeout) * time.Millisecond}}

	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(rw http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		err := r.ParseForm()
		if err != nil {
			http.Error(rw, "Bad Request", http.StatusBadRequest)
			return
		}
		if t := r.Form.Get("t"); t != "" {
			targetKey, err := strconv.Atoi(t)
			if err != nil || len(config.Targets)-1 < targetKey {
				http.Error(rw, "Bad Request", http.StatusBadRequest)
				return
			}
			aggregator.Aggregate([]string{config.Targets[targetKey]}, rw)
		} else {
			aggregator.Aggregate(config.Targets, rw)
		}
	})

	log.Printf("Starting server on %s...", config.Server.Bind)
	log.Fatal(http.ListenAndServe(config.Server.Bind, mux))
}

type Result struct {
	URL          string
	SecondsTaken float64
	Payload      io.ReadCloser
	Error        error
}

type Aggregator struct {
	HTTP *http.Client
}

func (f *Aggregator) Aggregate(targets []string, output io.Writer) {

	resultChan := make(chan *Result, 100)

	for _, target := range targets {
		go f.fetch(target, resultChan)
	}

	func(numTargets int, resultChan chan *Result) {
		numResuts := 0
		for {
			if numTargets == numResuts {
				return
			}
			select {
			case result := <-resultChan:
				numResuts++
				if result.Error != nil {
					log.Printf("Fetch error: %s", result.Error.Error())
					continue
				}
				_, err := io.Copy(output, result.Payload)
				if err != nil {
					log.Printf("Copy error: %s", err.Error())
				}
				result.Payload.Close()
				output.Write([]byte("\n"))

				if *verbose {
					log.Printf("OK: %s was refreshed in %.3f seconds", result.URL, result.SecondsTaken)
				}
			}
		}
	}(len(targets), resultChan)
}

func (f *Aggregator) fetch(target string, resultChan chan *Result) {

	startTime := time.Now()
	res, err := f.HTTP.Get(target)
	result := &Result{URL: target, SecondsTaken: time.Since(startTime).Seconds(), Error: nil}
	if res != nil {
		result.Payload = res.Body
	}
	if err != nil {
		result.Error = fmt.Errorf("Failed to fetch URL %s due to error: %s", target, err.Error())
	}
	resultChan <- result
}
