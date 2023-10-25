# build stage
FROM golang:1.20.4-alpine AS build-env
RUN apk --no-cache add build-base git gcc
ARG GIT_TAG=unknown
ADD . /src
RUN cd /src/cmd && go build -ldflags "-X main.Version=${GIT_TAG}" -o prometheus-aggregate-exporter

FROM alpine:latest
WORKDIR /app

ARG USER=nobody
USER nobody

COPY --from=build-env --chown=nobody /src/cmd/prometheus-aggregate-exporter /app/
ENTRYPOINT ["./prometheus-aggregate-exporter"]
