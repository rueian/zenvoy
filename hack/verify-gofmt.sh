#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

find_files() {
  find . -not \( \
      \( \
        -wholename '*/vendor/*' \
      \) -prune \
    \) -name '*.go'
}

GOFMT="gofmt -w -s"
bad_files=$(find_files | xargs $GOFMT -l)
if [[ -n "${bad_files}" ]]; then

  for bad_file in ${bad_files};
  do
    `$GOFMT ${bad_file}`
  done

  echo "** Needs to add following file changes due to gofmt **"
  echo "${bad_file}"

  exit 1
fi
