// Package config reads the reconciler's runtime configuration from
// environment variables set by the AuthStack-composed CronJob.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Config is the resolved runtime configuration.
type Config struct {
	// ZitadelBaseURL is the in-cluster URL of the Zitadel API (e.g.
	// "http://<release>-zitadel.<ns>.svc.cluster.local:8080").
	ZitadelBaseURL string

	// TargetNamespace is the K8s namespace where the operational Secrets
	// (and the machine-key Secret) live.
	TargetNamespace string

	// MachineKeySecret is the K8s Secret name holding the iam-admin
	// machine key JSON (the chart writes this on first install via the
	// FirstInstance.Org.Machine.MachineKey sidecar). The reconciler
	// reads it, signs a JWT, and exchanges it for an access token.
	MachineKeySecret string

	// AdminPasswordSecret is the K8s Secret name holding the human
	// `zitadel-admin` password. Used as a fallback when MachineKeySecret
	// is missing/invalid (e.g. before the first chart install completes).
	AdminPasswordSecret string
	AdminUsername       string

	// ManagedPATs is the list of operational PAT Secrets to reconcile.
	// Each entry pairs a K8s Secret name with the username of the
	// service account whose PAT it carries.
	ManagedPATs []ManagedPAT

	// ReadyTimeout is how long to wait for Zitadel /readyz before
	// giving up. Defaults to 5m.
	ReadyTimeout time.Duration
}

// ManagedPAT describes one operational PAT Secret + its owning user.
type ManagedPAT struct {
	SecretName string
	Username   string
}

// FromEnv reads Config from the standard env vars set by the
// AuthStack composition's CronJob template.
func FromEnv() (*Config, error) {
	cfg := &Config{
		ZitadelBaseURL:      strings.TrimRight(os.Getenv("ZITADEL_BASE_URL"), "/"),
		TargetNamespace:     os.Getenv("TARGET_NAMESPACE"),
		MachineKeySecret:    os.Getenv("MACHINE_KEY_SECRET"),
		AdminPasswordSecret: os.Getenv("ZITADEL_ADMIN_PASSWORD_SECRET"),
		AdminUsername:       getenvDefault("ZITADEL_ADMIN_USERNAME", "zitadel-admin"),
		ReadyTimeout:        5 * time.Minute,
	}

	for _, p := range []ManagedPAT{
		{
			SecretName: os.Getenv("IAM_ADMIN_PAT_SECRET"),
			Username:   os.Getenv("IAM_ADMIN_USERNAME"),
		},
		{
			SecretName: os.Getenv("LOGIN_CLIENT_PAT_SECRET"),
			Username:   os.Getenv("LOGIN_CLIENT_USERNAME"),
		},
	} {
		if p.SecretName != "" && p.Username != "" {
			cfg.ManagedPATs = append(cfg.ManagedPATs, p)
		}
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	if c.ZitadelBaseURL == "" {
		return fmt.Errorf("ZITADEL_BASE_URL is required")
	}
	if c.TargetNamespace == "" {
		return fmt.Errorf("TARGET_NAMESPACE is required")
	}
	if c.MachineKeySecret == "" && c.AdminPasswordSecret == "" {
		return fmt.Errorf("at least one of MACHINE_KEY_SECRET or ZITADEL_ADMIN_PASSWORD_SECRET is required")
	}
	if len(c.ManagedPATs) == 0 {
		return fmt.Errorf("no managed PAT Secrets configured (set IAM_ADMIN_PAT_SECRET/USERNAME and/or LOGIN_CLIENT_PAT_SECRET/USERNAME)")
	}
	return nil
}

func getenvDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
