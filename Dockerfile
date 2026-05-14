# syntax=docker/dockerfile:1
#
# Reconciler image for the AuthStack XRD.
#
# Multi-arch via Go cross-compilation: the `build` stage runs natively on
# the runner's BUILDPLATFORM (amd64 on GitHub-hosted runners) and emits
# the binary for TARGETPLATFORM. Without --platform=$BUILDPLATFORM the
# Go toolchain would itself run under QEMU when targeting arm64, which is
# ~10x slower. With it the compiler runs native and cross-compiles in
# seconds. The final stage uses the target-arch distroless image as
# normal, so the resulting manifest list still carries native images for
# each platform.

FROM --platform=$BUILDPLATFORM golang:1.26 AS build
WORKDIR /src
ARG TARGETOS
ARG TARGETARCH
ENV CGO_ENABLED=0 GOFLAGS=-trimpath

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o /out/reconciler ./

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/reconciler /usr/local/bin/reconciler
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/reconciler"]
