#!/bin/sh
set -e

: ${CONFIG:="/etc/agg-exporter/config.yml"}
: ${VERBOSE:="false"}
: ${LABEL:="true"}
: ${LABEL_NAME:="ae_source"}

if [ "$1" = 'aggregate-exporter' ]; then
	exec aggregate-exporter \
		-config=${CONFIG} \
		-verbose=${VERBOSE} \
		-label=${LABEL} \
		-label.name=${LABEL_NAME}
fi

exec "$@"