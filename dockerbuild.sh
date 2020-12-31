#!/bin/bash

set -e

export DOCKER_BUILDKIT=1
export COMPOSE_DOCKER_CLI_BUILD=1

for bin in xds echo proxy; do
  docker build --rm -t rueian/zenvoy-$bin:latest --target $bin .
done
