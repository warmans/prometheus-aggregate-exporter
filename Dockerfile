FROM alpine:latest

COPY ./bin/prometheus-aggregate-exporter /bin/aggregate-exporter

CMD ["aggregate-exporter"]