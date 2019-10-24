Aggregate Exporter
============================

Aggregates many exporters to a single page to reduce number of
exposed endpoints.

__NOTE__

This doesn't actually aggregate metrics (as in it doesn't sum them up etc.) 
and is possibly quite badly named. As a result each target
has it's metrics tagged with a label so that they are not duplicated in
the aggregate view. This can be modified 

### Options

```
  -server.bind (SERVER_BIND) string
    	Bind the HTTP server to this address e.g. 127.0.0.1:8080 or just :8080 (default ":8080")
    	
  -target.scrape.timeout (TARGET_SCRAPE_TIMEOUT) int
    	If a target metrics pages does not responde with this many miliseconds then timeout (default 1000)
    	
  -targets (TARGETS) string
    	comma separated list of targets e.g. http://localhost:8081/metrics,http://localhost:8082/metrics or url1=http://localhost:8081/metrics,url2=http://localhost:8082/metrics for custom label values
    	
  -targets.label (TARGETS_LABEL) bool
    	Add a label to metrics to show their origin target (default true)
    	
  -targets.label.name (TARGETS_LABEL_NAME) string
    	Label name to use if a target name label is appended to metrics (default "ae_source")
    	
  -targets.scrape.timeout (TARGETS_SCRAPE_TIMEOUT) int
    	If a target metrics pages does not responde with this many miliseconds then timeout (default 1000)

  -verbose (VERBOSE)
    	Log more information
    	
  -version (VERSION)
    	Show version and exit

```

### Example Usage
```
./bin/prometheus-aggregate-exporter \
	-targets="http://localhost:3000/histogram.txt,http://localhost:3000/histogram-2.txt" \
	-server.bind=":8080"
```

or using environment variables instead of flags: 


```
TARGETS="http://localhost:3000/histogram.txt,http://localhost:3000/histogram-2.txt" \
SERVER_BIND=":8080" \
./bin/prometheus-aggregate-exporter 
```

or with docker

```
docker run -it -p 8080:8080 -e TARGETS="http://localhost:3000/metrics" warmans/aggregate-exporter:latest
```
