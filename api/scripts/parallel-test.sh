#!/usr/bin/env bash
# go test ./..., with one package's tests spread over several processes:
#   scripts/parallel-test.sh <package> <processes> [<part> <parts>]
#   e.g. scripts/parallel-test.sh ./internal/httpapi 4
# With <part> and <parts>, the package's tests are dealt into <parts> and this run takes one, so
# that machines can share them; the other packages run with part 1. More than four processes on
# one four-CPU runner, which also ran the database server, were no faster.
# Each database test creates and drops its own schema, so the processes share only the server, as
# the packages under go test ./... already do. Within a process the tests still run one at a time,
# in the package's directory, and every test runs once.
set -euo pipefail
cd "$(dirname "$0")/.."
package=$(go list "$1")
processes=$2 part=${3:-1} parts=${4:-1}
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

go test -c -o "$work/package.test" "$package"
pids=() labels=() logs=()
if ((part == 1)); then
  mapfile -t others < <(go list ./... | grep -vxF "$package")
  # -count=1 runs them even when the build cache holds a result, as the split package always runs.
  go test -count=1 "${others[@]}" > "$work/others.log" 2>&1 &
  pids+=("$!") labels+=("go test ./... except $package") logs+=("$work/others.log")
fi

dir=$(go list -f '{{.Dir}}' "$package")
mapfile -t all < <("$work/package.test" -test.list '.*')
tests=()
for ((j = part - 1; j < ${#all[@]}; j += parts)); do tests+=("${all[j]}"); done
for ((i = 0; i < processes; i++)); do
  names=()
  for ((j = i; j < ${#tests[@]}; j += processes)); do names+=("${tests[j]}"); done
  ((${#names[@]})) || continue
  pattern="^($(IFS='|'; echo "${names[*]}"))\$"
  (
    cd "$dir"
    start=$(date +%s)
    status=0
    "$work/package.test" -test.run "$pattern" -test.timeout 10m -test.paniconexit0 || status=$?
    echo "$(($(date +%s) - start)) s"
    exit "$status"
  ) > "$work/$i.log" 2>&1 &
  pids+=("$!") labels+=("$package, part $part of $parts, process $((i + 1)) of $processes: ${#names[@]} tests") logs+=("$work/$i.log")
done

status=0
for k in "${!pids[@]}"; do
  if wait "${pids[k]}"; then result=ok; else result=FAIL status=1; fi
  echo "== $result: ${labels[k]}"
  cat "${logs[k]}"
done
exit "$status"
