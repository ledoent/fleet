# Multi-stage Dockerfile for Fleet (Ledo build)
# Builds from source with frontend assets embedded.

ARG FLEET_VERSION=dev

# Stage 1: Build frontend assets
# Pinned by digest — bump together with the tag when refreshing base images.
FROM node:24-bookworm@sha256:33cf7f057918860b043c307751ef621d74ac96f875b79b6724dcebf2dfd0db6d AS frontend
WORKDIR /build
COPY package.json yarn.lock ./
RUN yarn install --frozen-lockfile --network-timeout 600000
COPY . .
RUN NODE_OPTIONS=--openssl-legacy-provider NODE_ENV=production yarn run webpack --progress

# Stage 2: Build Go binary
FROM golang:1.26-bookworm@sha256:e8c859f5632dcfde7b32d2012b4351728f6437930887c2f6a91ea242459e5514 AS backend
RUN apt-get update && apt-get install -y --no-install-recommends gcc
WORKDIR /build
ARG FLEET_VERSION
COPY --from=frontend /build .
RUN go run github.com/kevinburke/go-bindata/go-bindata -pkg=bindata -tags full \
    -o=server/bindata/generated.go \
    frontend/templates/ assets/... server/mail/templates
RUN CGO_ENABLED=1 go build -tags full,fts5,netgo -trimpath \
    -ldflags "-extldflags '-static' \
    -X github.com/fleetdm/fleet/v4/server/version.version=${FLEET_VERSION}-ledo \
    -X github.com/fleetdm/fleet/v4/server/version.branch=aggregated" \
    -o fleet ./cmd/fleet

# Stage 3: Runtime image
FROM alpine:3.21@sha256:48b0309ca019d89d40f670aa1bc06e426dc0931948452e8491e3d65087abc07d
RUN apk --no-cache add ca-certificates tini
RUN addgroup -S fleet && adduser -S fleet -G fleet
USER fleet
COPY --from=backend /build/fleet /usr/bin/fleet
ENTRYPOINT ["/sbin/tini", "--"]
CMD ["fleet", "serve"]
