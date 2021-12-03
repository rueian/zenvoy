# syntax=docker/dockerfile:experimental

FROM golang:1.17-alpine3.15 AS build
ENV GOCACHE="/gobuildcache"
ENV GOPATH="/go"
WORKDIR /src
ADD . /src
RUN --mount=type=cache,target=/gobuildcache \
    --mount=type=cache,target=/go/pkg/mod/cache \
    ls cmd | xargs -I {} go build -o /{} cmd/{}/main.go

FROM alpine:3.15 AS proxy
RUN apk add --no-cache nftables iptables
COPY --from=build /proxy /
ENTRYPOINT ["/proxy"]

FROM alpine:3.15 AS echo
COPY --from=build /echo /
ENTRYPOINT ["/echo"]

FROM alpine:3.15 AS xds
RUN apk add --no-cache iptables
COPY --from=build /xds /
ENTRYPOINT ["/xds"]
