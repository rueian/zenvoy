# syntax=docker/dockerfile:experimental

FROM golang:1.15 AS build
ARG bin
ENV GOCACHE="/gobuildcache"
ENV GOPATH="/go"
WORKDIR /src
ADD . /src
RUN --mount=type=cache,target=/gobuildcache \
    --mount=type=cache,target=/go/pkg/mod/cache \
    go build -o /$bin cmd/$bin/main.go

FROM gcr.io/distroless/base-debian10
ARG bin
COPY --from=build /$bin /zenvoy
ENTRYPOINT ["/zenvoy"]
