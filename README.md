Aggregate Exporter
============================

Aggregates many exporters to a single page to reduce number of
exposed endpoints.

```
./prometheus-aggregate-exporter -config="config.yml"
```

#### Sample config.yml

```
server:
  bind: ":8080"
interval: 10 #seconds
targets:
  - "http://localhost:8081/metrics"
  - "http://localhost:8081/metrics"

```

