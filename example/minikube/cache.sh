#!/bin/bash

set -e

for bin in xds echo proxy; do
	minikube cache delete rueian/zenvoy-$bin:latest
  minikube cache add rueian/zenvoy-$bin:latest
done

minikube cache reload