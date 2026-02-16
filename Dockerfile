# Multi-stage Dockerfile for Fleet with Kencove premium license
# Based on fleet-v4.80.2 with modified LoadLicense()

# Stage 1: Build frontend assets
FROM node:24-bookworm AS frontend
WORKDIR /build
COPY package.json yarn.lock ./
RUN yarn install --frozen-lockfile --network-timeout 600000
COPY . .
RUN NODE_ENV=production yarn run webpack --progress

# Stage 2: Build Go binary
FROM golang:1.25.7-bookworm AS backend
RUN apt-get update && apt-get install -y --no-install-recommends gcc
WORKDIR /build
COPY --from=frontend /build .
RUN go run github.com/kevinburke/go-bindata/go-bindata -pkg=bindata -tags full \
    -o=server/bindata/generated.go \
    frontend/templates/ assets/... server/mail/templates
ARG FLEET_VERSION=dev
RUN CGO_ENABLED=1 go build -tags full,fts5,netgo -trimpath \
    -ldflags "-extldflags '-static' \
    -X github.com/fleetdm/fleet/v4/server/version.version=${FLEET_VERSION}-kencove \
    -X github.com/fleetdm/fleet/v4/server/version.branch=kencove" \
    -o fleet ./cmd/fleet

# Stage 3: Runtime image
FROM alpine:3.21
RUN apk --no-cache add ca-certificates tini
RUN addgroup -S fleet && adduser -S fleet -G fleet
USER fleet
COPY --from=backend /build/fleet /usr/bin/fleet
ENTRYPOINT ["/sbin/tini", "--"]
CMD ["fleet", "serve"]
