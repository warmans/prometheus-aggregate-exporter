# build stage
FROM golang:alpine AS build-env
RUN apk --no-cache add build-base git gcc
ADD . /src
RUN cd /src/cmd && go build -o prometheus-aggregate-exporter

FROM alpine:latest
WORKDIR /app
COPY --from=build-env /src/cmd/prometheus-aggregate-exporter /app/
ENTRYPOINT ["./prometheus-aggregate-exporter"]