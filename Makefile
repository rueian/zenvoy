.DEFAULT_GOAL := check
SHELL := /bin/bash

check: static-check

static-check:
	hack/verify-gofmt.sh
	hack/verify-govet.sh