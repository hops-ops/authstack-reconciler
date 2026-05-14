# authstack-reconciler

In-cluster reconciler for [`hops-ops/auth-stack`](https://github.com/hops-ops/auth-stack)'s
operational Personal Access Tokens (PATs). Runs as a CronJob in the
AuthStack install namespace; reads the iam-admin machine key,
authenticates against Zitadel via JWT bearer, and ensures each managed
PAT Secret carries a valid token — minting fresh ones via the
management API when an existing value is missing or has been rejected.

See [`hops-ops/auth-stack` spec `specs/authstack-reconciler`](https://github.com/hops-ops/auth-stack)
for the full design (durable vs operational secret split, how this
fits with ESO `ExternalSecret` + `PushSecret` for capture-and-restore,
and the loss-scenario recovery paths).

## Configuration

The reconciler is configured entirely via environment variables — set
by the CronJob spec composed by the AuthStack XRD. No flags, no config
files.

| Env var | Required | Description |
|---|---|---|
| `ZITADEL_BASE_URL` | yes | In-cluster URL of the Zitadel API (e.g. `http://<release>-zitadel.<ns>.svc.cluster.local:8080`). |
| `TARGET_NAMESPACE` | yes | Namespace holding the managed Secrets. |
| `MACHINE_KEY_SECRET` | one-of | K8s Secret name holding the iam-admin machine key JSON (the chart writes this on first install). The reconciler reads, signs a JWT, and exchanges for an access token. |
| `ZITADEL_ADMIN_PASSWORD_SECRET` | one-of | (Fallback, not yet implemented) K8s Secret holding the human `zitadel-admin` password — used when the machine key isn't available. |
| `ZITADEL_ADMIN_USERNAME` | no | Human admin username for the password fallback. Defaults to `zitadel-admin`. |
| `IAM_ADMIN_PAT_SECRET` | optional | Secret name for the iam-admin PAT. |
| `IAM_ADMIN_USERNAME` | with above | Login name of the iam-admin machine user. |
| `LOGIN_CLIENT_PAT_SECRET` | optional | Secret name for the login-client PAT (required for the login UI pod's volume mount). |
| `LOGIN_CLIENT_USERNAME` | with above | Login name of the login-client machine user. |

At least one of `MACHINE_KEY_SECRET` / `ZITADEL_ADMIN_PASSWORD_SECRET`
must be set. At least one managed PAT pair
(`*_PAT_SECRET` + `*_USERNAME`) must be set.

## Behavior

```
1. Wait up to 5m for /debug/ready
2. Authenticate (machine-key JWT exchange)
3. For each managed PAT Secret:
   a. Read the K8s Secret
   b. If `pat` key is present, validate via GET /auth/v1/users/me
   c. If absent or 401, look up the user by username and mint a fresh PAT
      via POST /management/v1/users/{userId}/pats
   d. Write the K8s Secret with `pat=<value>`
4. Exit 0
```

PushSecret (composed by the AuthStack XRD alongside this CronJob)
mirrors any Secret write back to AWS Secrets Manager, so subsequent
installs against a fresh K8s namespace recover via `ExternalSecret`
projection without needing a re-mint.

## RBAC

The reconciler's ServiceAccount needs `get` on the machine-key Secret
and `get`/`update`/`patch`/`create` on the operational PAT Secrets in
its own namespace. The AuthStack composition renders the
ServiceAccount, Role, and RoleBinding alongside the CronJob.

## Build

```sh
go build ./...
docker build -t authstack-reconciler:dev .
```

Published images: `ghcr.io/hops-ops/authstack-reconciler` (built by
[`unbounded-tech/workflows-containers`](https://github.com/unbounded-tech/workflows-containers)
on push to main / version tag).
