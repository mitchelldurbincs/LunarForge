#!/bin/sh
# Pinned tool and transient reports stay outside the repository.
set -eu
# staticcheck@v0.8.1 needs a Go toolchain newer than some CI runners ship;
# auto lets go run fetch exactly what it needs (pinned module versions unchanged).
export GOTOOLCHAIN=auto
scratch=$(mktemp -d /tmp/lunarforge-gocognit.XXXXXX)
trap 'rm -rf "$scratch"' EXIT HUP INT TERM
TMPDIR=/tmp GOBIN="$scratch" go install github.com/uudashr/gocognit/cmd/gocognit@v1.2.0
packages=$(go list -f "{{.Dir}}" ./...)

status=0
"$scratch/gocognit" -over 60 -test=false $packages > "$scratch/report" || status=$?

# Normalize report file keys to module-relative paths so .gocognit-baseline
# rows (written as paths relative to the repo root) can match even though
# gocognit receives absolute package dirs.
pwdhere=$(pwd)
sed -e "s|$pwdhere/||g" "$scratch/report" > "$scratch/report.relative"
mv "$scratch/report.relative" "$scratch/report"
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
# Staticcheck bug classes only (style checks live out of scope), type-checked
# with the repo's own toolchain so export data always matches.
TMPDIR=/tmp GOFLAGS=-mod=mod go run honnef.co/go/tools/cmd/staticcheck@v0.8.1 -checks='SA*' -tests=false ./...

# Curated go-critic diagnostic checks (style checkers excluded; rangeValCopy
# calibrated to 256 bytes so structural-gate PRs stay mechanical).
TMPDIR=/tmp GOFLAGS=-mod=mod go run github.com/go-critic/go-critic/cmd/gocritic@v0.15.0 \
    check -enable=appendAssign,appendCombine,boolExprSimplify,deferInLoop,exitAfterDefer,filepathJoin,ifElseChain,rangeValCopy,shift,singleCaseSwitch,sloppyLen,sloppyTypeAssign,underef,unlabelStmt,unslice \
    -@rangeValCopy.sizeThreshold=256 ./... >/dev/null
