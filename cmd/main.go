package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"github.com/prometheus/client_golang/prometheus"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"syscall"
	"time"

	"github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"gopkg.in/yaml.v3"
)

// Config is used to store the configuration of this program
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
	targetUpMetric         *bool
	serverBind             *string
	tlsServerCert          *string
	tlsServerKey           *string
	targetScrapeTimeout    *int
	targets                *string
	targetsConfigPath      *string
	insecureSkipVerifyFlag *bool
	authType               *string
	authUsername           *string
	authPassword           *string
	authPasswordFile       *string
	authToken              *string
	authTokenFile          *string
	cacheFilePath          *string
	dynamicRegistration    *bool
)

func init() {
	verboseFlag = boolFlag(flag.CommandLine, "verbose", false, "Log more information")
	versionFlag = boolFlag(flag.CommandLine, "version", false, "Show version and exit")
	serverBind = stringFlag(flag.CommandLine, "server.bind", ":8080", "Bind the HTTP server to this address e.g. 127.0.0.1:8080 or just :8080. For unix socket use unix:/path/to/file.sock")

	tlsServerCert = stringFlag(flag.CommandLine, "tls.server-cert", "", "Path to the TLS server cert for serving via HTTPS")
	tlsServerKey = stringFlag(flag.CommandLine, "tls.server-key", "", "Path to the TLS server key for serving via HTTPS")

	targetScrapeTimeout = intFlag(flag.CommandLine, "targets.scrape.timeout", 1000, "If a target metrics pages does not responde with this many miliseconds then timeout")
	targets = stringFlag(flag.CommandLine, "targets", "", "comma separated list of targets e.g. http://localhost:8081/metrics,http://localhost:8082/metrics or url1=http://localhost:8081/metrics,url2=http://localhost:8082/metrics for custom label values")
	targetsConfigPath = stringFlag(flag.CommandLine, "targets.config", "", "Path to JSON config file for per-target configuration (auth, etc.)")
	targetLabelsEnabled = boolFlag(flag.CommandLine, "targets.label", true, "Add a label to metrics to show their origin target")
	targetLabelName = stringFlag(flag.CommandLine, "targets.label.name", "ae_source", "Label name to use if a target name label is appended to metrics")
	targetUpMetric = boolFlag(flag.CommandLine, "targets.up", false, "Enables an additional reachability metric for each downstream exporter.")

	insecureSkipVerifyFlag = boolFlag(flag.CommandLine, "insecure-skip-verify", false, "Disable verification of TLS certificates")
	authType = stringFlag(flag.CommandLine, "targets.auth.type", "", "Authentication type for all targets: basic or bearer")
	authUsername = stringFlag(flag.CommandLine, "targets.auth.username", "", "Username for basic auth")
	authPassword = stringFlag(flag.CommandLine, "targets.auth.password", "", "Password for basic auth")
	authPasswordFile = stringFlag(flag.CommandLine, "targets.auth.password_file", "", "File containing password for basic auth")
	authToken = stringFlag(flag.CommandLine, "targets.auth.token", "", "Bearer token for all targets")
	authTokenFile = stringFlag(flag.CommandLine, "targets.auth.token_file", "", "File containing bearer token for all targets")

	dynamicRegistration = boolFlag(flag.CommandLine, "targets.dynamic.registration", false, "Enabled dynamic targets registration/deregistration using /register and /unregister endpoints")
	cacheFilePath = stringFlag(flag.CommandLine, "targets.cache.path", "", "Path to file used as cache of targets usable in case of application restart with additional targets registered")
}

func main() {

	flag.Parse()

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

	var configTargets []TargetSpec
	if targetsConfigPath != nil && *targetsConfigPath != "" {
		loaded, err := loadTargetsConfig(*targetsConfigPath)
		if err != nil {
			log.Fatalf("FATAL: Failed to load targets config: %s", err.Error())
		}
		configTargets = loaded
	}

	if len(config.Targets) < 1 {
		if len(configTargets) > 0 || *dynamicRegistration {
			log.Print("No initial targets configured, using registration only")
		} else {
			log.Fatal("FATAL: No initial targets configured and dynamic registration is disabled.")
		}
	}

	var cacheFile string
	if *dynamicRegistration {
		log.Println("Dynamic target registration enabled")
		if *cacheFilePath != "" {
			log.Printf("Using targets cache file %s\n", *cacheFilePath)
			cacheFile = *cacheFilePath
		}
	} else {
		if *cacheFilePath != "" {
			// cache makes no sense if dynamic registration is not enabled.
			log.Printf("WARN: Dynamic registration is disabled but cache file path was given. Cache will be ignored.")
		}
	}

	// enable InsecureSkipVerify
	if *insecureSkipVerifyFlag {
		log.Printf("disabled verification of TLS certificates")
		http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	aggregator := &Aggregator{
		HTTP:             &http.Client{Timeout: time.Duration(config.Timeout) * time.Millisecond},
		AuthType:         *authType,
		AuthUsername:     *authUsername,
		AuthPassword:     *authPassword,
		AuthPasswordFile: *authPasswordFile,
		AuthToken:        *authToken,
		AuthTokenFile:    *authTokenFile,
	}

	targets := NewTargets(config.Targets, cacheFile)

	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(rw http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		err := r.ParseForm()
		if err != nil {
			http.Error(rw, "Bad Request", http.StatusBadRequest)
			return
		}
		allSpecs := append([]TargetSpec{}, configTargets...)
		allSpecs = append(allSpecs, targetSpecsFromStrings(targets.Targets())...)

		if t := r.Form.Get("t"); t != "" {
			targetKey, err := strconv.Atoi(t)
			if err != nil || len(allSpecs)-1 < targetKey {
				http.Error(rw, "Bad Request", http.StatusBadRequest)
				return
			}
			aggregator.Aggregate([]TargetSpec{allSpecs[targetKey]}, rw)
		} else {
			aggregator.Aggregate(allSpecs, rw)
		}
	})
	mux.HandleFunc("/alive", func(rw http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		rw.WriteHeader(http.StatusOK)
	})
	if *dynamicRegistration {
		mux.HandleFunc("/register", func(rw http.ResponseWriter, r *http.Request) {
			defer r.Body.Close()
			err := r.ParseForm()
			if err != nil {
				http.Error(rw, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
				return
			}
			name := r.Form.Get("name")
			address := r.Form.Get("address")
			if name == "" || address == "" {
				http.Error(rw, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
				return
			}

			schema := r.Form.Get("schema")
			if schema == "" {
				schema = "http"
			}

			uri := schema + "://" + address
			targets.AddTarget(name + "=" + uri)
			log.Printf("Registered target %s with name %s\n", uri, name)
		})
		mux.HandleFunc("/unregister", func(rw http.ResponseWriter, r *http.Request) {
			defer r.Body.Close()
			err := r.ParseForm()
			if err != nil {
				http.Error(rw, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
				return
			}
			name := r.Form.Get("name")
			address := r.Form.Get("address")
			if name == "" || address == "" {
				http.Error(rw, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
				return
			}

			schema := r.Form.Get("schema")
			if schema == "" {
				schema = "http"
			}

			uri := schema + "://" + address
			targets.RemoveTarget(name + "=" + uri)
			log.Printf("Unregistered target %s with name %s\n", uri, name)
		})
	}

	log.Printf("Starting server on %s with targets:\n", config.Server.Bind)
	for _, t := range targets.Targets() {
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
		if tlsServerCert != nil && *tlsServerCert != "" && tlsServerKey != nil && *tlsServerKey != "" {
			if _, err := os.Stat(*tlsServerCert); err != nil {
				log.Fatalf("Failed to load TLS server certificate: '%v'", err)
			}

			if _, err := os.Stat(*tlsServerKey); err != nil {
				log.Fatalf("Failed to load TLS server key: '%v'", err)
			}
			log.Fatal(http.ListenAndServeTLS(config.Server.Bind, *tlsServerCert, *tlsServerKey, mux))
		} else {
			log.Fatal(http.ListenAndServe(config.Server.Bind, mux))
		}
	}

}

func NewTargets(initialTargets []string, cachePath string) *Targets {
	t := &Targets{
		cachePath: cachePath,
		targets:   make(map[string]struct{}),
		lock:      sync.RWMutex{},
	}
	t.tryLoadCache()
	for _, v := range initialTargets {
		t.AddTarget(v)
	}
	return t
}

type Targets struct {
	cachePath string
	targets   map[string]struct{}
	lock      sync.RWMutex
}

func (t *Targets) AddTarget(target string) {
	target = strings.TrimSpace(target)
	if target == "" {
		return
	}
	t.lock.Lock()
	defer func() {
		t.lock.Unlock()
		t.updateCache()
	}()
	t.targets[target] = struct{}{}
}

func (t *Targets) RemoveTarget(target string) {
	target = strings.TrimSpace(target)
	t.lock.Lock()
	defer func() {
		t.lock.Unlock()
		t.updateCache()
	}()
	delete(t.targets, target)
}

func (t *Targets) Targets() []string {
	t.lock.RLock()
	defer t.lock.RUnlock()

	ts := []string{}
	for k := range t.targets {
		ts = append(ts, k)
	}

	return ts
}

func (t *Targets) updateCache() {
	if t.cachePath == "" {
		return
	}
	err := writeLines(t.Targets(), t.cachePath)
	if err != nil {
		log.Fatal("Error while saving targets cache")
	}
}

func (t *Targets) tryLoadCache() {
	if t.cachePath == "" {
		return
	}
	targetsFromFile, err := readLines(t.cachePath)
	if err == nil {
		for _, v := range targetsFromFile {
			t.AddTarget(v)
			log.Printf("Recovered target %s from cache file\n", v)
		}
	} else {
		log.Printf("Failed to load cache: %s\n", err.Error())
	}
}

type Result struct {
	URL          string
	Name         string
	SecondsTaken float64
	MetricFamily map[string]*io_prometheus_client.MetricFamily
	Error        error
}

type TargetSpec struct {
	Name string
	URL  string
	Auth *AuthConfig
}

type TargetsConfig struct {
	Targets []TargetConfig `json:"targets" yaml:"targets"`
}

type TargetConfig struct {
	Name string      `json:"name" yaml:"name"`
	URL  string      `json:"url" yaml:"url"`
	Auth *AuthConfig `json:"auth" yaml:"auth"`
}

type AuthConfig struct {
	Type         string `json:"type" yaml:"type"`
	Username     string `json:"username" yaml:"username"`
	Password     string `json:"password" yaml:"password"`
	PasswordFile string `json:"password_file" yaml:"password_file"`
	Token        string `json:"token" yaml:"token"`
	TokenFile    string `json:"token_file" yaml:"token_file"`
}

type Aggregator struct {
	HTTP             *http.Client
	AuthType         string
	AuthUsername     string
	AuthPassword     string
	AuthPasswordFile string
	AuthToken        string
	AuthTokenFile    string
}

func readLines(path string) ([]string, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		} else {
			return nil, err
		}
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("WARN: failed to close cache file after reading")
		}
	}()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

// writeLines writes the lines to the given file.
func writeLines(lines []string, path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("WARN: failed to close cache file after writing")
		}
	}()

	w := bufio.NewWriter(file)
	for _, line := range lines {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return w.Flush()
}

func (f *Aggregator) Aggregate(targets []TargetSpec, output io.Writer) {

	resultChan := make(chan *Result, 100)

	for _, target := range targets {
		go f.fetch(target, resultChan)
	}

	func(numTargets int, resultChan chan *Result) {

		numResults := 0

		allFamilies := make(map[string]*io_prometheus_client.MetricFamily)

		upReg := prometheus.NewRegistry()
		upMetric := prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "up",
				Help: "Information about if the exporter reached the downstream exporters",
			},
			// Even if targetLabels are not enabled, we need to label the up metric accordingly.
			[]string{*targetLabelName},
		)
		upReg.MustRegister(upMetric)

		for {
			if numTargets == numResults {
				break
			}

			result := <-resultChan
			numResults++

			if result.Error != nil {
				upMetric.WithLabelValues(result.Name).Set(0)
				log.Printf("Fetch error: %s", result.Error.Error())
				continue
			}

			upMetric.WithLabelValues(result.Name).Set(1)

			for mfName, mf := range result.MetricFamily {
				if *targetLabelsEnabled {
					for _, m := range mf.Metric {
						m.Label = append(m.Label, &io_prometheus_client.LabelPair{Name: targetLabelName, Value: &result.Name})
					}
				}
				if existingMf, ok := allFamilies[mfName]; ok {
					existingMf.Metric = append(existingMf.Metric, mf.Metric...)
				} else {
					allFamilies[*mf.Name] = mf
				}
			}
			if *verboseFlag {
				log.Printf("OK: %s=%s was refreshed in %.3f seconds", result.Name, result.URL, result.SecondsTaken)
			}

		}

		fmtText := expfmt.NewFormat(expfmt.TypeTextPlain)
		encoder := expfmt.NewEncoder(output, fmtText)

		if *targetUpMetric {
			upMetricFamilys, err := upReg.Gather()
			if err != nil {
				log.Printf("Failed to gather uptime metrics: %s", err.Error())
			}

			if err := encoder.Encode(upMetricFamilys[0]); err != nil {
				log.Printf("Failed to encode up family: %s", err.Error())
			}
		}

		for _, f := range allFamilies {
			if err := encoder.Encode(f); err != nil {
				log.Printf("Failed to encode familty: %s", err.Error())
			}
		}

	}(len(targets), resultChan)
}

func (f *Aggregator) fetch(target TargetSpec, resultChan chan *Result) {
	name, targetURL, err := parseTarget(target.Name, target.URL)
	if err != nil {
		resultChan <- &Result{
			Name:         target.Name,
			URL:          target.URL,
			SecondsTaken: 0,
			Error:        fmt.Errorf("failed to parse target %s due to error: %s", target.URL, err.Error()),
		}
		return
	}

	startTime := time.Now()
	req, err := http.NewRequest(http.MethodGet, targetURL.String(), nil)
	if err != nil {
		resultChan <- &Result{
			Name:         name,
			URL:          hideURLUserInfo(targetURL),
			SecondsTaken: 0,
			Error:        fmt.Errorf("failed to build request for target %s due to error: %s", hideURLUserInfo(targetURL), err.Error()),
		}
		return
	}
	if err := f.applyAuth(req, target.Auth); err != nil {
		resultChan <- &Result{
			Name:         name,
			URL:          hideURLUserInfo(targetURL),
			SecondsTaken: 0,
			Error:        fmt.Errorf("failed to apply auth for target %s due to error: %s", hideURLUserInfo(targetURL), err.Error()),
		}
		return
	}
	res, err := f.HTTP.Do(req)

	result := &Result{URL: hideURLUserInfo(targetURL), Name: name, SecondsTaken: time.Since(startTime).Seconds(), Error: nil}
	if res != nil {
		defer func() {
			if closeErr := res.Body.Close(); closeErr != nil {
				log.Printf("WARN: failed to close response body")
			}
		}()
		if res.StatusCode >= http.StatusBadRequest {
			result.Error = fmt.Errorf("failed to fetch URL %s due to status code: %d", hideURLUserInfo(targetURL), res.StatusCode)
			resultChan <- result
			return
		}
		result.MetricFamily, err = getMetricFamilies(res.Body)
		if err != nil {
			result.Error = fmt.Errorf("failed to add labels to target %s metrics: %s", hideURLUserInfo(targetURL), err.Error())
			resultChan <- result
			return
		}
	}
	if err != nil {
		result.Error = fmt.Errorf("failed to fetch URL %s due to error: %s", hideURLUserInfo(targetURL), err.Error())
	}
	resultChan <- result
}

func parseTarget(name string, targetURLRaw string) (string, *url.URL, error) {
	targetURL, err := url.Parse(targetURLRaw)
	if err != nil {
		return "", nil, err
	}
	if targetURL.Scheme == "" || targetURL.Host == "" {
		return "", nil, fmt.Errorf("target must include scheme and host")
	}
	if targetURL.User != nil {
		return "", nil, fmt.Errorf("credentials in target URL are not allowed; use targets.auth.* flags")
	}
	if name == "" {
		name = hideURLUserInfo(targetURL)
	}
	return name, targetURL, nil
}

func hideURLUserInfo(targetURL *url.URL) string {
	u := *targetURL
	u.User = nil
	return u.String()
}

func resolveSecretValue(value string, filePath string) (string, error) {
	if filePath == "" {
		return value, nil
	}
	b, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func (f *Aggregator) resolveAuthPassword(auth *AuthConfig) (string, error) {
	if auth == nil {
		return resolveSecretValue(f.AuthPassword, f.AuthPasswordFile)
	}
	return resolveSecretValue(auth.Password, auth.PasswordFile)
}

func (f *Aggregator) resolveAuthToken(auth *AuthConfig) (string, error) {
	if auth == nil {
		return resolveSecretValue(f.AuthToken, f.AuthTokenFile)
	}
	return resolveSecretValue(auth.Token, auth.TokenFile)
}

func authModeFromConfig(authType string, username string, password string, passwordFile string, token string, tokenFile string) string {
	mode := strings.ToLower(strings.TrimSpace(authType))
	if mode == "" {
		if username != "" || password != "" || passwordFile != "" {
			return "basic"
		}
		if token != "" || tokenFile != "" {
			return "bearer"
		}
	}
	return mode
}

func (f *Aggregator) applyAuth(req *http.Request, auth *AuthConfig) error {
	if auth == nil {
		switch authModeFromConfig(f.AuthType, f.AuthUsername, f.AuthPassword, f.AuthPasswordFile, f.AuthToken, f.AuthTokenFile) {
		case "":
			return nil
		case "basic":
			if f.AuthUsername == "" {
				return fmt.Errorf("targets.auth.username is required for basic auth")
			}
			password, err := f.resolveAuthPassword(nil)
			if err != nil {
				return err
			}
			req.SetBasicAuth(f.AuthUsername, password)
			return nil
		case "bearer":
			token, err := f.resolveAuthToken(nil)
			if err != nil {
				return err
			}
			if token == "" {
				return fmt.Errorf("targets.auth.token or targets.auth.token_file is required for bearer auth")
			}
			req.Header.Set("Authorization", "Bearer "+token)
			return nil
		default:
			return fmt.Errorf("unsupported targets.auth.type %q (supported: basic, bearer)", f.AuthType)
		}
	}

	switch authModeFromConfig(auth.Type, auth.Username, auth.Password, auth.PasswordFile, auth.Token, auth.TokenFile) {
	case "":
		return nil
	case "basic":
		if auth.Username == "" {
			return fmt.Errorf("auth.username is required for basic auth")
		}
		password, err := f.resolveAuthPassword(auth)
		if err != nil {
			return err
		}
		req.SetBasicAuth(auth.Username, password)
		return nil
	case "bearer":
		token, err := f.resolveAuthToken(auth)
		if err != nil {
			return err
		}
		if token == "" {
			return fmt.Errorf("auth.token or auth.token_file is required for bearer auth")
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	default:
		return fmt.Errorf("unsupported auth.type %q (supported: basic, bearer)", auth.Type)
	}
}

func targetSpecsFromStrings(targets []string) []TargetSpec {
	specs := make([]TargetSpec, 0, len(targets))
	for _, t := range targets {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		name := ""
		targetURL := t
		if strings.Contains(t, "=") {
			split := strings.SplitN(t, "=", 2)
			name = split[0]
			targetURL = split[1]
		}
		specs = append(specs, TargetSpec{Name: name, URL: targetURL, Auth: nil})
	}
	return specs
}

func loadTargetsConfig(configPath string) ([]TargetSpec, error) {
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	cfg, baseDir, err := decodeTargetsConfig(absPath, b)
	if err != nil {
		return nil, err
	}
	if errs := validateTargetsConfig(cfg); len(errs) > 0 {
		return nil, formatConfigValidationError(absPath, errs)
	}

	specs := make([]TargetSpec, 0, len(cfg.Targets))
	for _, t := range cfg.Targets {
		auth := t.Auth
		if auth != nil {
			if auth.PasswordFile != "" && !filepath.IsAbs(auth.PasswordFile) {
				auth.PasswordFile = filepath.Join(baseDir, auth.PasswordFile)
			}
			if auth.TokenFile != "" && !filepath.IsAbs(auth.TokenFile) {
				auth.TokenFile = filepath.Join(baseDir, auth.TokenFile)
			}
		}
		specs = append(specs, TargetSpec{
			Name: strings.TrimSpace(t.Name),
			URL:  strings.TrimSpace(t.URL),
			Auth: auth,
		})
	}
	return specs, nil
}

type configFieldError struct {
	Path    string
	Message string
}

func decodeTargetsConfig(absPath string, b []byte) (*TargetsConfig, string, error) {
	ext := strings.ToLower(filepath.Ext(absPath))
	cfg := &TargetsConfig{}

	switch ext {
	case ".json":
		if err := decodeTargetsConfigJSON(b, cfg); err != nil {
			return nil, "", fmt.Errorf("failed to parse JSON: %s", err.Error())
		}
	case ".yml", ".yaml":
		if err := decodeTargetsConfigYAML(b, cfg); err != nil {
			return nil, "", fmt.Errorf("failed to parse YAML: %s", err.Error())
		}
	default:
		// If extension is unknown, try JSON then YAML for convenience.
		if err := decodeTargetsConfigJSON(b, cfg); err != nil {
			cfg = &TargetsConfig{}
			if err2 := decodeTargetsConfigYAML(b, cfg); err2 != nil {
				return nil, "", fmt.Errorf("unsupported config extension %q; JSON error: %s; YAML error: %s", ext, err.Error(), err2.Error())
			}
		}
	}

	return cfg, filepath.Dir(absPath), nil
}

func decodeTargetsConfigJSON(b []byte, out *TargetsConfig) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return formatJSONDecodeError(err)
	}
	// Ensure there is no trailing data.
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return fmt.Errorf("unexpected trailing JSON data")
	} else if !errors.Is(err, io.EOF) {
		return formatJSONDecodeError(err)
	}
	return nil
}

func decodeTargetsConfigYAML(b []byte, out *TargetsConfig) error {
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		return err
	}
	// Prevent multiple YAML documents (--- ... ---).
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return fmt.Errorf("multiple YAML documents are not supported")
	} else if !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func formatJSONDecodeError(err error) error {
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return fmt.Errorf("syntax error near byte offset %d", syntaxErr.Offset)
	}
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		if typeErr.Field != "" {
			return fmt.Errorf("invalid type for field %q near byte offset %d", typeErr.Field, typeErr.Offset)
		}
		return fmt.Errorf("invalid JSON type near byte offset %d", typeErr.Offset)
	}
	return err
}

func validateTargetsConfig(cfg *TargetsConfig) []configFieldError {
	errs := []configFieldError{}
	if cfg == nil {
		return []configFieldError{{Path: "", Message: "config is empty"}}
	}
	if len(cfg.Targets) == 0 {
		errs = append(errs, configFieldError{Path: "targets", Message: "must contain at least one entry"})
		return errs
	}
	for i, t := range cfg.Targets {
		p := fmt.Sprintf("targets[%d]", i)
		if strings.TrimSpace(t.URL) == "" {
			errs = append(errs, configFieldError{Path: p + ".url", Message: "is required"})
			continue
		}
		u, err := url.Parse(strings.TrimSpace(t.URL))
		if err != nil || u.Scheme == "" || u.Host == "" {
			errs = append(errs, configFieldError{Path: p + ".url", Message: "must be a valid absolute URL (scheme + host)"})
		} else if u.User != nil {
			errs = append(errs, configFieldError{Path: p + ".url", Message: "must not include credentials (userinfo); use auth.* instead"})
		}

		if t.Auth == nil {
			continue
		}

		mode := authModeFromConfig(t.Auth.Type, t.Auth.Username, t.Auth.Password, t.Auth.PasswordFile, t.Auth.Token, t.Auth.TokenFile)
		switch mode {
		case "":
			// empty auth block treated as no auth
		case "basic":
			if strings.TrimSpace(t.Auth.Username) == "" {
				errs = append(errs, configFieldError{Path: p + ".auth.username", Message: "is required for basic auth"})
			}
			if strings.TrimSpace(t.Auth.Password) == "" && strings.TrimSpace(t.Auth.PasswordFile) == "" {
				errs = append(errs, configFieldError{Path: p + ".auth.password", Message: "password or password_file is required for basic auth"})
			}
		case "bearer":
			if strings.TrimSpace(t.Auth.Token) == "" && strings.TrimSpace(t.Auth.TokenFile) == "" {
				errs = append(errs, configFieldError{Path: p + ".auth.token", Message: "token or token_file is required for bearer auth"})
			}
		default:
			errs = append(errs, configFieldError{Path: p + ".auth.type", Message: fmt.Sprintf("unsupported value %q (supported: basic, bearer)", t.Auth.Type)})
		}
	}
	return errs
}

func formatConfigValidationError(absPath string, errs []configFieldError) error {
	lines := make([]string, 0, len(errs))
	for _, e := range errs {
		if e.Path == "" {
			lines = append(lines, "- "+e.Message)
		} else {
			lines = append(lines, fmt.Sprintf("- %s: %s", e.Path, e.Message))
		}
	}
	return fmt.Errorf("invalid targets config %q:\n%s", absPath, strings.Join(lines, "\n"))
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
