// Package reconciler is the orchestration loop: wait for Zitadel,
// authenticate, then for each managed PAT Secret either confirm it's
// valid or mint a fresh one and write it.
package reconciler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/hops-ops/authstack-reconciler/internal/config"
	"github.com/hops-ops/authstack-reconciler/internal/k8s"
	"github.com/hops-ops/authstack-reconciler/internal/zitadel"
)

// Reconciler runs one pass of the AuthStack reconciliation loop.
type Reconciler struct {
	cfg *config.Config
	log *slog.Logger
	k8s *k8s.Client
	z   *zitadel.Client
}

// New wires up the dependencies.
func New(_ context.Context, cfg *config.Config, log *slog.Logger) (*Reconciler, error) {
	kc, err := k8s.NewInCluster(cfg.TargetNamespace)
	if err != nil {
		return nil, fmt.Errorf("k8s client: %w", err)
	}
	return &Reconciler{
		cfg: cfg,
		log: log,
		k8s: kc,
		z:   zitadel.New(cfg.ZitadelBaseURL),
	}, nil
}

// Run executes one reconciliation pass. Idempotent: a no-op when all
// managed PATs are valid; mints fresh ones when missing or invalid.
func (r *Reconciler) Run(ctx context.Context) error {
	readyCtx, cancel := context.WithTimeout(ctx, r.cfg.ReadyTimeout)
	defer cancel()
	r.log.Info("waiting for zitadel readiness", "baseURL", r.cfg.ZitadelBaseURL)
	if err := r.z.WaitReady(readyCtx); err != nil {
		return fmt.Errorf("zitadel did not become ready: %w", err)
	}
	r.log.Info("zitadel is ready")

	if err := r.authenticate(ctx); err != nil {
		return fmt.Errorf("authenticate: %w", err)
	}

	var anyErr error
	for _, p := range r.cfg.ManagedPATs {
		if err := r.reconcilePAT(ctx, p); err != nil {
			r.log.Error("reconcile PAT failed", "secret", p.SecretName, "username", p.Username, "err", err)
			anyErr = errors.Join(anyErr, err)
		}
	}
	return anyErr
}

func (r *Reconciler) authenticate(ctx context.Context) error {
	// Phase 2 v1: only the machine-key path is implemented. The
	// admin-password fallback is wired through config but the reconciler
	// uses it only as a "you forgot to seed the machine key" diagnostic
	// hint for now. A future iteration can do OAuth ROPC against the
	// human admin user when machine key is unavailable.
	if r.cfg.MachineKeySecret == "" {
		return fmt.Errorf("MACHINE_KEY_SECRET not configured; admin-password fallback not yet implemented")
	}
	data, err := r.k8s.ReadSecret(ctx, r.cfg.MachineKeySecret)
	if err != nil {
		return fmt.Errorf("read machine-key secret %q: %w", r.cfg.MachineKeySecret, err)
	}
	if data == nil {
		return fmt.Errorf("machine-key secret %q not found — wait for the chart's setup-job sidecar to write it, then re-run", r.cfg.MachineKeySecret)
	}
	// The chart writes the JSON blob under a key that historically has
	// varied across versions; accept the obvious candidates.
	var raw []byte
	for _, k := range []string{"key.json", "iam-admin.json", "machinekey", "key"} {
		if v, ok := data[k]; ok && len(v) > 0 {
			raw = v
			break
		}
	}
	if raw == nil {
		// Single-value Secret with an unknown key: fall back to the
		// first entry that decodes as JSON.
		for _, v := range data {
			if json.Valid(v) {
				raw = v
				break
			}
		}
	}
	if raw == nil {
		return fmt.Errorf("machine-key secret %q has no recognized JSON-encoded key payload", r.cfg.MachineKeySecret)
	}

	var mk zitadel.MachineKey
	if err := json.Unmarshal(raw, &mk); err != nil {
		return fmt.Errorf("parse machine key: %w", err)
	}
	if mk.UserID == "" || mk.Key == "" {
		return fmt.Errorf("machine key missing required fields (userId, key)")
	}

	if err := r.z.AuthWithMachineKey(ctx, &mk, ""); err != nil {
		return fmt.Errorf("token exchange: %w", err)
	}
	r.log.Info("authenticated as iam-admin via machine key")
	return nil
}

func (r *Reconciler) reconcilePAT(ctx context.Context, p config.ManagedPAT) error {
	log := r.log.With("secret", p.SecretName, "username", p.Username)

	data, err := r.k8s.ReadSecret(ctx, p.SecretName)
	if err != nil {
		return fmt.Errorf("read secret: %w", err)
	}
	existing := stringValue(data, "pat", "token")
	if existing != "" {
		if err := r.z.ValidatePAT(ctx, existing); err == nil {
			log.Info("existing PAT is valid; skipping mint")
			return nil
		} else if errors.Is(err, zitadel.ErrPATInvalid) {
			log.Warn("existing PAT was rejected by Zitadel; will mint a fresh one")
		} else {
			return fmt.Errorf("validate PAT: %w", err)
		}
	} else {
		log.Info("no existing PAT; will mint")
	}

	userID, err := r.z.FindUserByLoginName(ctx, p.Username)
	if err != nil {
		return fmt.Errorf("find user %q: %w", p.Username, err)
	}
	newPAT, err := r.z.MintPAT(ctx, userID)
	if err != nil {
		return fmt.Errorf("mint PAT for user %q: %w", p.Username, err)
	}
	if err := r.k8s.UpsertSecretKey(ctx, p.SecretName, "pat", []byte(newPAT)); err != nil {
		return fmt.Errorf("write secret: %w", err)
	}
	log.Info("minted and wrote new PAT")
	return nil
}

func stringValue(m map[string][]byte, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok && len(v) > 0 {
			return string(v)
		}
	}
	return ""
}
