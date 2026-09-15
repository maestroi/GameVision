# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS build
RUN apk add --no-cache ca-certificates git
WORKDIR /src
COPY . .
# Local development uses ../gomeboy. The container build is hermetic and uses
# the required v1.1.0 module instead, matching CI's pinned sibling checkout.
RUN go mod edit -dropreplace github.com/maestroi/gomeboy && \
    go mod download && \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/gamevision ./cmd/gamevision

FROM alpine:3.20
RUN apk add --no-cache ca-certificates curl && update-ca-certificates
COPY --from=build /out/gamevision /usr/local/bin/gamevision
COPY deploy/entrypoint.sh /usr/local/bin/gamevision-entrypoint
RUN chmod +x /usr/local/bin/gamevision-entrypoint && \
    mkdir -p /data/sessions /rom

ENV GAMEVISION_BASE_URL=http://192.168.50.81:8002/v1 \
    GAMEVISION_MODEL=Qwen3-VL-4B-Instruct \
    GAMEVISION_MODEL_MATCH=Qwen3-VL-4B \
    GAMEVISION_REQUIRE_VISION=1 \
    GAMEVISION_WATCH_VISION=1

EXPOSE 8099
ENTRYPOINT ["/usr/local/bin/gamevision-entrypoint"]
