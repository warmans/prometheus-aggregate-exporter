# build stage
FROM golang:1.20.4-alpine AS build-env
RUN apk --no-cache add build-base git gcc
ADD . /src
RUN cd /src/cmd && go build -o prometheus-aggregate-exporter

FROM alpine:latest
WORKDIR /app

ARG USER=nobody
USER nobody

COPY --from=build-env --chown=nobody /src/cmd/prometheus-aggregate-exporter /app/
ENTRYPOINT ["./prometheus-aggregate-exporter"]
