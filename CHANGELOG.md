### What's changed in v0.0.3

* fix(docker): cross-compile Go for multi-arch builds (fast path) (by @patrickleet)

  Pinning the build stage to --platform=\$BUILDPLATFORM keeps the Go
  compiler running natively on the runner (amd64) and cross-compiles to
  TARGETARCH via GOOS/GOARCH. Without this, buildx runs the entire
  golang:1.26 build stage under QEMU when targeting arm64, which makes
  the Go toolchain itself emulated — every \`go build\` invocation crawls
  and a small binary takes 10+ minutes instead of seconds.

  The final distroless stage is left at the default (TARGETPLATFORM) so
  each platform's manifest still has a native binary.


See full diff: [v0.0.2...v0.0.3](https://github.com/hops-ops/authstack-reconciler/compare/v0.0.2...v0.0.3)
