#!/usr/bin/env bash
# go test ./..., with one package's tests spread over several processes:
#   scripts/parallel-test.sh <package> <processes>, e.g. scripts/parallel-test.sh ./internal/httpapi 4
# Each database test creates and drops its own schema, so the processes share only the server, as
# the packages under go test ./... already do. Within a process the tests still run one at a time,
# in the package's directory, and every test runs once.
set -euo pipefail
cd "$(dirname "$0")/.."
package=$(go list "$1")
processes=$2
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

go test -c -o "$work/package.test" "$package"
mapfile -t others < <(go list ./... | grep -vxF "$package")
go test "${others[@]}" > "$work/others.log" 2>&1 &
pids=("$!") labels=("go test ./... except $package") logs=("$work/others.log")

dir=$(go list -f '{{.Dir}}' "$package")
mapfile -t tests < <("$work/package.test" -test.list '.*')
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
  pids+=("$!") labels+=("$package, process $((i + 1)) of $processes: ${#names[@]} tests") logs+=("$work/$i.log")
done

status=0
for k in "${!pids[@]}"; do
  if wait "${pids[k]}"; then result=ok; else result=FAIL status=1; fi
  echo "== $result: ${labels[k]}"
  cat "${logs[k]}"
done
exit "$status"
