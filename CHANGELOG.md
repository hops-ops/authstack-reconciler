### What's changed in v0.0.2

* chore: Add renovate.json (#1) (by @renovate[bot])

  Co-authored-by: renovate[bot] <29139614+renovate[bot]@users.noreply.github.com>

* ci: pin workflows-containers to v1.8.0 + build multi-arch (by @patrickleet)

  v1.8.0 ships the new `platforms` input. Set linux/amd64,linux/arm64 so
  the image runs on both EKS amd64 nodes and arm64 dev clusters (colima,
  M-series). Also pins the workflow ref away from @main for reproducibility.


See full diff: [v0.0.1...v0.0.2](https://github.com/hops-ops/authstack-reconciler/compare/v0.0.1...v0.0.2)
