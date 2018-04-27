FROM alpine:latest

RUN mkdir -p /etc/agg-exporter

COPY prometheus-aggregate-exporter /usr/local/bin/aggregate-exporter
COPY docker-entrypoint.sh /.

ENTRYPOINT ["/docker-entrypoint.sh"]
CMD ["aggregate-exporter"]