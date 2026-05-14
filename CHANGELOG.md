### What's changed in v0.0.1

* initial: authstack-reconciler (by @patrickleet)

  In-cluster reconciler for hops-ops/auth-stack's operational PATs.
  Reads the iam-admin machine key from a K8s Secret, signs a JWT, and
  exchanges it for an access token via Zitadel's `urn:ietf:params:oauth:
  grant-type:jwt-bearer` flow. Then for each managed PAT Secret it either
  confirms the existing value is still valid (GET /auth/v1/users/me) or
  mints a fresh one via POST /management/v1/users/{userId}/pats and
  writes it back to K8s. PushSecret (composed by the auth-stack XRD
  alongside this CronJob) handles capture-to-AWS-SM.

  Idempotent — sub-second when no work to do; converges on every tick.
  Implements [[specs/authstack-reconciler]] (in hops-ops/auth-stack).

  Structure:
  - main.go: entry point + signal handling
  - internal/config: env-var configuration
  - internal/k8s: scoped in-cluster Secret client
  - internal/zitadel: JWT bearer auth + minimal API client
  - internal/reconciler: orchestration loop

* test(config): cover required-field validation + defaults (by @patrickleet)

* ci: split workflows — vnext on main, publish on tag, test on PR (by @patrickleet)

  Match the canonical unbounded-tech release pattern that auth-stack and
  the other Crossplane stacks use:

  - on-push-main: runs `go vet`/`go test`, then `workflow-vnext-tag`
    computes the next SemVer and pushes a v* tag (via DEPLOY_KEY).
  - on-version-tagged: triggers on `v*.*.*` tags — `workflows-containers`
    publishes the image at the tag + SHA, `workflow-simple-release`
    cuts a GitHub Release with the auto-generated changelog.
  - on-pr: tests + a PR-tagged image build for preview.

  Replaces the prior 'build on every push' workflow that left an
  amd64-only :main tag in GHCR. Going forward only v* tags publish images.


