package main

import (
	"flag"
	"log"
	"os"
	"strings"

	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"syscall"
	"time"

	"github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

//Config is used to store the configuration of this program
type Config struct {
	Server struct {
		Bind string
	}
	Timeout int
	Targets []string
}

var (
	//Version if the version of this program
	Version = "unknown"

	verboseFlag            *bool
	versionFlag            *bool
	targetLabelsEnabled    *bool
	targetLabelName        *string
	serverBind             *string
	targetScrapeTimeout    *int
	targets                *string
	insecureSkipVerifyFlag *bool
)

func init() {
	verboseFlag = boolFlag(flag.CommandLine, "verbose", false, "Log more information")
	versionFlag = boolFlag(flag.CommandLine, "version", false, "Show version and exit")
	serverBind = stringFlag(flag.CommandLine, "server.bind", ":8080", "Bind the HTTP server to this address e.g. 127.0.0.1:8080 or just :8080. For unix socket use unix:/path/to/file.sock")

	targetScrapeTimeout = intFlag(flag.CommandLine, "targets.scrape.timeout", 1000, "If a target metrics pages does not responde with this many miliseconds then timeout")
	targets = stringFlag(flag.CommandLine, "targets", "", "comma separated list of targets e.g. http://localhost:8081/metrics,http://localhost:8082/metrics or url1=http://localhost:8081/metrics,url2=http://localhost:8082/metrics for custom label values")
	targetLabelsEnabled = boolFlag(flag.CommandLine, "targets.label", true, "Add a label to metrics to show their origin target")
	targetLabelName = stringFlag(flag.CommandLine, "targets.label.name", "ae_source", "Label name to use if a target name label is appended to metrics")

	insecureSkipVerifyFlag = boolFlag(flag.CommandLine, "insecure-skip-verify", false, "Disable verification of TLS certificates")

	flag.Parse()
}

func main() {

	if *versionFlag {
		fmt.Print(Version)
		os.Exit(0)
	}

	config := &Config{
		Server: struct {
			Bind string
		}{
			Bind: *serverBind,
		},
		Timeout: *targetScrapeTimeout,
		Targets: filterEmptyStrings(strings.Split(*targets, ",")),
	}

	if len(config.Targets) < 1 {
		log.Fatal("No targets configured")
	}

	// enable InsecureSkipVerify
	if *insecureSkipVerifyFlag {
		log.Printf("disabled verification of TLS certificates")
		http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
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

	log.Printf("Starting server on %s with targets:\n", config.Server.Bind)
	for _, t := range config.Targets {
		log.Printf("  - %s\n", t)
	}

	s := strings.Split(config.Server.Bind, ":")
	if s[0] == "unix" {
		if len(s) != 2 {
			log.Fatal("Socket file not specified!")
		}
		if _, err := os.Stat(s[1]); err == nil {
			err = os.Remove(s[1])
			if err != nil {
				log.Fatal(err)
			}
		}
		syscall.Umask(0000)
		unixListener, err := net.Listen("unix", s[1])
		if err != nil {
			log.Fatal(err)
		}
		log.Fatal(http.Serve(unixListener, mux))
	} else {
		log.Fatal(http.ListenAndServe(config.Server.Bind, mux))
	}

}

type Result struct {
	URL          string
	Name         string
	SecondsTaken float64
	MetricFamily map[string]*io_prometheus_client.MetricFamily
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

		numResults := 0

		allFamilies := make(map[string]*io_prometheus_client.MetricFamily)

		for {
			if numTargets == numResults {
				break
			}
			select {
			case result := <-resultChan:
				numResults++

				if result.Error != nil {
					log.Printf("Fetch error: %s", result.Error.Error())
					continue
				}

				for mfName, mf := range result.MetricFamily {
					if *targetLabelsEnabled {
						for _, m := range mf.Metric {
							m.Label = append(m.Label, &io_prometheus_client.LabelPair{Name: targetLabelName, Value: &result.Name})
						}
					}
					if existingMf, ok := allFamilies[mfName]; ok {
						for _, m := range mf.Metric {
							existingMf.Metric = append(existingMf.Metric, m)
						}
					} else {
						allFamilies[*mf.Name] = mf
					}
				}
				if *verboseFlag {
					log.Printf("OK: %s=%s was refreshed in %.3f seconds", result.Name, result.URL, result.SecondsTaken)
				}
			}
		}

		encoder := expfmt.NewEncoder(output, expfmt.FmtText)
		for _, f := range allFamilies {
			encoder.Encode(f)
		}

	}(len(targets), resultChan)
}

func (f *Aggregator) fetch(target string, resultChan chan *Result) {

	s := strings.Split(target, "=")
	url := s[0]
	name := s[0]
	if len(s) > 1 {
		url = strings.Join(s[1:], "=")
	}

	startTime := time.Now()
	res, err := f.HTTP.Get(url)

	result := &Result{URL: url, Name: name, SecondsTaken: time.Since(startTime).Seconds(), Error: nil}
	if res != nil {
		result.MetricFamily, err = getMetricFamilies(res.Body)
		if err != nil {
			result.Error = fmt.Errorf("failed to add labels to target %s metrics: %s", target, err.Error())
			resultChan <- result
			return
		}
	}
	if err != nil {
		result.Error = fmt.Errorf("failed to fetch URL %s due to error: %s", target, err.Error())
	}
	resultChan <- result
}

func getMetricFamilies(sourceData io.Reader) (map[string]*io_prometheus_client.MetricFamily, error) {
	parser := expfmt.TextParser{}
	metricFamiles, err := parser.TextToMetricFamilies(sourceData)
	if err != nil {
		return nil, err
	}
	return metricFamiles, nil
}

func filterEmptyStrings(ss []string) []string {
	filtered := []string{}
	for _, s := range ss {
		if s != "" {
			filtered = append(filtered, s)
		}
	}
	return filtered
}
