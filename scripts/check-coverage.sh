#!/bin/sh
# Fail if go tool cover total is below MIN (default 70).
set -eu

PROFILE="${1:-coverage.out}"
MIN="${2:-70}"

if [ ! -f "$PROFILE" ]; then
	echo "missing $PROFILE" >&2
	exit 1
fi

TOTAL=$(go tool cover -func="$PROFILE" | awk '/^total:/{print $3}' | tr -d '%')
if [ -z "$TOTAL" ]; then
	echo "could not parse total coverage" >&2
	exit 1
fi

awk -v t="$TOTAL" -v m="$MIN" 'BEGIN {
	if ((t + 0) < (m + 0)) {
		printf "coverage %s%% is below %s%%\n", t, m
		exit 1
	}
	printf "coverage %s%% (min %s%%)\n", t, m
}'
