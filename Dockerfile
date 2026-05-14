# syntax=docker/dockerfile:1
#
# Reconciler image for the AuthStack XRD. Two-stage build: build with
# the Go toolchain, run on distroless static so the image is tiny and
# has no shell / package manager surface for the in-cluster Job.

FROM golang:1.26 AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOFLAGS=-trimpath

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN go build -ldflags="-s -w" -o /out/reconciler ./

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/reconciler /usr/local/bin/reconciler
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/reconciler"]
