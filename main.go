package main

import (
	"flag"
	"io/ioutil"
	"log"
	"os"

	"gopkg.in/yaml.v2"

	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var (
	Version = "unknown"
)

var (
	configPathFlag = flag.String("config", "config.yml", "Path to config YAML file.")
	verboseFlag    = flag.Bool("verbose", false, "Log more information")
	versionFlag    = flag.Bool("version", false, "Show version and exit")
	stripComments  = flag.Bool("strip", false, "remove any comment lines in aggregated output")
)

type Config struct {
	Server  struct{ Bind string }
	Timeout int
	Targets []string
}

func main() {

	flag.Parse()

	if *versionFlag {
		fmt.Print(Version)
		os.Exit(0)
	}

	configFile, err := os.Open(*configPathFlag)
	if err != nil {
		log.Fatalf("Failed to open config file at path %s due to error: %s", *configPathFlag, err.Error())
	}
	defer configFile.Close()

	configData, err := ioutil.ReadAll(configFile)
	if err != nil {
		log.Fatalf("Failed to read config file at path %s due to error: %s", *configPathFlag, err.Error())
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

				err = result.Payload.Close()
				if err != nil {
					log.Printf("Result body close error: %s", err.Error())
				}

				if *verboseFlag {
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
		if !*stripComments {
			result.Payload = res.Body
		} else {
			var buff *bytes.Buffer
			scanner := bufio.NewScanner(res.Body)
			for scanner.Scan() {
				if strings.HasPrefix(scanner.Text(), "#") {
					continue
				}
				if _, err := buff.WriteString(scanner.Text() + "\n"); err != nil {
					result.Error = fmt.Errorf("failed writing deduplicated data to buffer: %s", err)
					resultChan <- result
					return
				}
			}
			result.Payload = ioutil.NopCloser(buff)
		}
	}
	if err != nil {
		result.Error = fmt.Errorf("Failed to fetch URL %s due to error: %s", target, err.Error())
	}
	resultChan <- result
}
