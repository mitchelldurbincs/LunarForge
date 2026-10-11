#!/bin/sh
# Pinned tool and transient reports stay outside the repository.
set -eu
scratch=$(mktemp -d /tmp/lunarforge-gocognit.XXXXXX)
trap 'rm -rf "$scratch"' EXIT HUP INT TERM
TMPDIR=/tmp GOBIN="$scratch" go install github.com/uudashr/gocognit/cmd/gocognit@v1.2.0
packages=$(go list -f '{{.Dir}}' ./...)
status=0
"$scratch/gocognit" -over 60 -test=false $packages > "$scratch/report" || status=$?
# gocognit returns 1 for a nonempty threshold report, and also for parse errors.
# Parse errors have no report; never silently accept a failed analysis.
if [ "$status" -ne 0 ] && { [ "$status" -ne 1 ] || [ ! -s "$scratch/report" ]; }; then
    exit "$status"
fi
awk '
    NR == FNR {
        if ($0 !~ /^#/ && NF) limits[$2 " " $3 " " $4] = $1
        next
    }
    {
        file = $4
        sub(/:[0-9]+:[0-9]+$/, "", file)
        key = $2 " " $3 " " file
        if (!(key in limits) || $1 > limits[key]) {
            print
            failed = 1
        }
    }
    END { exit failed }
' .gocognit-baseline "$scratch/report"
