#!/bin/bash

set -e

export DOCKER_BUILDKIT=1
export COMPOSE_DOCKER_CLI_BUILD=1

docker-compose build --force-rm xds
docker-compose build --force-rm echo
docker-compose build --force-rm proxy

docker-compose up
