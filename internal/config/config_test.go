package config

import (
	"os"
	"testing"
)

func TestFromEnv_RequiresBaseURL(t *testing.T) {
	withEnv(t, map[string]string{
		"TARGET_NAMESPACE":        "zitadel",
		"MACHINE_KEY_SECRET":      "iam-admin",
		"LOGIN_CLIENT_PAT_SECRET": "login-client",
		"LOGIN_CLIENT_USERNAME":   "login-client",
	})
	if _, err := FromEnv(); err == nil {
		t.Fatal("expected error when ZITADEL_BASE_URL is missing")
	}
}

func TestFromEnv_RequiresNamespace(t *testing.T) {
	withEnv(t, map[string]string{
		"ZITADEL_BASE_URL":        "http://zitadel.example",
		"MACHINE_KEY_SECRET":      "iam-admin",
		"LOGIN_CLIENT_PAT_SECRET": "login-client",
		"LOGIN_CLIENT_USERNAME":   "login-client",
	})
	if _, err := FromEnv(); err == nil {
		t.Fatal("expected error when TARGET_NAMESPACE is missing")
	}
}

func TestFromEnv_RequiresOneCredentialSource(t *testing.T) {
	withEnv(t, map[string]string{
		"ZITADEL_BASE_URL":        "http://zitadel.example",
		"TARGET_NAMESPACE":        "zitadel",
		"LOGIN_CLIENT_PAT_SECRET": "login-client",
		"LOGIN_CLIENT_USERNAME":   "login-client",
	})
	if _, err := FromEnv(); err == nil {
		t.Fatal("expected error when neither MACHINE_KEY_SECRET nor ZITADEL_ADMIN_PASSWORD_SECRET is set")
	}
}

func TestFromEnv_RequiresAtLeastOnePATPair(t *testing.T) {
	withEnv(t, map[string]string{
		"ZITADEL_BASE_URL":   "http://zitadel.example",
		"TARGET_NAMESPACE":   "zitadel",
		"MACHINE_KEY_SECRET": "iam-admin",
	})
	if _, err := FromEnv(); err == nil {
		t.Fatal("expected error when no managed PAT pairs are configured")
	}
}

func TestFromEnv_PicksUpBothPATPairs(t *testing.T) {
	withEnv(t, map[string]string{
		"ZITADEL_BASE_URL":        "http://zitadel.example/",
		"TARGET_NAMESPACE":        "zitadel",
		"MACHINE_KEY_SECRET":      "iam-admin",
		"IAM_ADMIN_PAT_SECRET":    "iam-admin-pat",
		"IAM_ADMIN_USERNAME":      "iam-admin",
		"LOGIN_CLIENT_PAT_SECRET": "login-client",
		"LOGIN_CLIENT_USERNAME":   "login-client",
	})
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.ZitadelBaseURL != "http://zitadel.example" {
		t.Fatalf("trailing slash not trimmed: %q", cfg.ZitadelBaseURL)
	}
	if len(cfg.ManagedPATs) != 2 {
		t.Fatalf("want 2 managed PATs, got %d", len(cfg.ManagedPATs))
	}
}

func TestFromEnv_DefaultsAdminUsername(t *testing.T) {
	withEnv(t, map[string]string{
		"ZITADEL_BASE_URL":              "http://zitadel.example",
		"TARGET_NAMESPACE":              "zitadel",
		"ZITADEL_ADMIN_PASSWORD_SECRET": "zitadel-admin-password",
		"LOGIN_CLIENT_PAT_SECRET":       "login-client",
		"LOGIN_CLIENT_USERNAME":         "login-client",
	})
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.AdminUsername != "zitadel-admin" {
		t.Fatalf("default admin username = %q, want zitadel-admin", cfg.AdminUsername)
	}
}

// withEnv replaces the process env for the duration of the test.
func withEnv(t *testing.T, vars map[string]string) {
	t.Helper()
	keys := []string{
		"ZITADEL_BASE_URL", "TARGET_NAMESPACE", "MACHINE_KEY_SECRET",
		"ZITADEL_ADMIN_PASSWORD_SECRET", "ZITADEL_ADMIN_USERNAME",
		"IAM_ADMIN_PAT_SECRET", "IAM_ADMIN_USERNAME",
		"LOGIN_CLIENT_PAT_SECRET", "LOGIN_CLIENT_USERNAME",
	}
	for _, k := range keys {
		k := k // capture
		old, hadOld := os.LookupEnv(k)
		t.Cleanup(func() {
			if hadOld {
				_ = os.Setenv(k, old)
			} else {
				_ = os.Unsetenv(k)
			}
		})
		_ = os.Unsetenv(k)
	}
	for k, v := range vars {
		_ = os.Setenv(k, v)
	}
}
