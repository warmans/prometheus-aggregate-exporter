Aggregate Exporter
============================

Aggregates many exporters to a single endpoint to reduce number of
exposed endpoints.

__NOTE__

This doesn't actually aggregate metrics (as in it doesn't sum them up etc.). 
As a result each target has its metrics tagged with a label so that they are not duplicated in
the aggregate view. 

### Options

```
  -server.bind (SERVER_BIND) string
    	Bind the HTTP server to this address e.g. 127.0.0.1:8080 or just :8080 (default ":8080"). For unix socket use unix:/path/to/file.sock
    	
  -targets (TARGETS) string
    	comma separated list of targets e.g. http://localhost:8081/metrics,http://localhost:8082/metrics or url1=http://localhost:8081/metrics,url2=http://localhost:8082/metrics for custom label values
    	
  -targets.label (TARGETS_LABEL) bool
    	Add a label to metrics to show their origin target (default true)
    	
  -targets.label.name (TARGETS_LABEL_NAME) string
    	Label name to use if a target name label is appended to metrics (default "ae_source")
    	
  -targets.scrape.timeout (TARGETS_SCRAPE_TIMEOUT) int
    	If a target metrics pages does not responde with this many miliseconds then timeout (default 1000)

  -targets.dynamic.registration (TARGETS_DYNAMIC_REGISTRATION) bool
        Enabled dynamic targets registration/deregistration using /register and /unregister endpoints (default false)
                
  -targets.cache.path
        Path to file used as cache of targets usable in case of application restart with dynamic targets registered (default empty)

  -verbose (VERBOSE)
    	Log more information
    	
  -version (VERSION)
    	Show version and exit
```

### How to build it

#### Build using the go binary

If you have go (1.14) installed on your machine, you can simply do:

```shell
cd cmd/
go build -o prometheus-aggregate-exporter
```

To build without CGO enabled, which removes the dependency on `libc`, do:

```shell
cd cmd/
CGO_ENABLED=0 go build -o prometheus-aggregate-exporter
```

Or you can use the provided Makefile:

```shell
make build
```

#### Build into a Docker image

If you have Docker installed (or any runtime understanding the Dockerfile format), you can simply invoke: 

    make docker-build
    
And you'll have Docker compile the binary and make it available under the image named `warmans/aggregate-exporter:latest`

Alternatively the image is available though docker-hub: https://hub.docker.com/r/warmans/prometheus-aggregate-exporter

### Manual Testing

You can run the exporter against some static fixture files by running the following make targets
in separate terminals.

```shell
make test.run-fixture-server
make test.run
```

then to view the `/metrics` page:

```shell
make test.fetch
```

### Example Usage

```shell
./bin/prometheus-aggregate-exporter \
	-targets="http://localhost:3000/histogram.txt,http://localhost:3000/histogram-2.txt" \
	-server.bind=":8080"
```

or using environment variables instead of flags: 


```shell
TARGETS="http://localhost:3000/histogram.txt,http://localhost:3000/histogram-2.txt" \
SERVER_BIND=":8080" \
./bin/prometheus-aggregate-exporter 
```

or with docker

```shell
docker run -it -p 8080:8080 -e TARGETS="http://localhost:3000/metrics" warmans/aggregate-exporter:latest
```

#### Custom labelling

By default, the labels values that will end up in your metrics will be equals to the target url; for example:

     http_requests_total{method="post",code="200",ae_source="http://localhost:3000/histogram.txt"} 1027 101 
     
But you can customize those labels; for example if you invoke the tool with:

     bin/prometheus-aggregate-exporter -targets="histo1=http://localhost:3000/histogram.txt,histo2=http://localhost:3000/histogram-2.txt" -server.bind=":8080" -targets.label.name="instance"

the metrics will rather look like:

    http_requests_total{method="post",code="200",instance="histo1"} 1027 1395066363000
         
#### Target URL containing a `=` character

In case one of your target urls contains a `=` character (for instance consul agent's exporter is available at `/v1/agent/metrics?format=prometheus`), you **must** use the custom labelling notation:

     bin/prometheus-aggregate-exporter -targets="consul=http://localhost:8500/v1/agent/metrics?format=prometheus"

#### Dynamic registration 

If `targets.dynamic.registration` is set to `true` additional `HTTP GET` endpoints are available `/register` and `/unregister`.
Those can be used for example for dynamic exporter discovery in multi replica `Kubernetes` environment when exact adress of exporter is not known at start.

Both support query parameters:
* name - name of registered exporter (same as name usable in `targets`)
* address - address of exporter without schema (ex. `localhost:9090/metrics`)
* schema - exporter schema (default `http`)

Example usage:
* register exporter: `localhost:8080/register?name=someExporter&address=localhost:3000/metrics`
* unregister exporter: `localhost:8080/unregister?name=xxx&address=localhost:3000`

### Releasing via github actions

A release is made whenever a tag following the normal semver pattern is pushed.

e.g. `v1.0.0` or `v1.0.0rc1`

### Releasing Manually

Releasing is done as follows (although it can only be done by the author).

1. git tag vX.X.X
2. make build 
3. make docker-build
4. make docker-publish 

