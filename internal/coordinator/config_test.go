package coordinator

import (
	"errors"
	"testing"
)

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "standalone OK without URL",
			cfg: Config{
				Mode:              ModeStandalone,
				InstanceTTL:       60e9,
				HeartbeatInterval: 20e9,
				ReconnectInterval: 5e9,
				SweepInterval:     45e9,
			},
		},
		{
			name: "fail-fast needs URL",
			cfg: Config{
				Mode:              ModeFailFast,
				InstanceTTL:       60e9,
				HeartbeatInterval: 20e9,
				ReconnectInterval: 5e9,
				SweepInterval:     45e9,
			},
			wantErr: true,
		},
		{
			name: "degraded with URL OK",
			cfg: Config{
				Mode:              ModeDegraded,
				RedisURL:          "redis://localhost:6379/0",
				InstanceTTL:       60e9,
				HeartbeatInterval: 20e9,
				ReconnectInterval: 5e9,
				SweepInterval:     45e9,
			},
		},
		{
			name: "heartbeat shorter than TTL is required",
			cfg: Config{
				Mode:              ModeStandalone,
				InstanceTTL:       20e9,
				HeartbeatInterval: 60e9,
				ReconnectInterval: 5e9,
				SweepInterval:     45e9,
			},
			wantErr: true,
		},
		{
			name: "unknown mode rejected",
			cfg: Config{
				Mode:              "weird",
				InstanceTTL:       60e9,
				HeartbeatInterval: 20e9,
				ReconnectInterval: 5e9,
				SweepInterval:     45e9,
			},
			wantErr: true,
		},
		{
			name: "ReconnectInterval must be > 0",
			cfg: Config{
				Mode:              ModeStandalone,
				InstanceTTL:       60e9,
				HeartbeatInterval: 20e9,
				ReconnectInterval: 0,
				SweepInterval:     45e9,
			},
			wantErr: true,
		},
		{
			name: "SweepInterval must be > 0",
			cfg: Config{
				Mode:              ModeStandalone,
				InstanceTTL:       60e9,
				HeartbeatInterval: 20e9,
				ReconnectInterval: 5e9,
				SweepInterval:     0,
			},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate err = %v, wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestConfigFromEnvDefaultsToStandalone(t *testing.T) {
	t.Setenv(EnvMode, "")
	t.Setenv(EnvRedisURLPrimary, "")
	t.Setenv(EnvRedisURLLegacy, "")

	c := ConfigFromEnv()
	if c.Mode != ModeStandalone {
		t.Fatalf("Mode = %s, want standalone", c.Mode)
	}
	if c.RedisURL != "" {
		t.Fatalf("unexpected RedisURL %q", c.RedisURL)
	}
}

func TestConfigFromEnvLegacyURLImpliesDegraded(t *testing.T) {
	t.Setenv(EnvMode, "")
	t.Setenv(EnvRedisURLPrimary, "")
	t.Setenv(EnvRedisURLLegacy, "redis://localhost:6379/0")

	c := ConfigFromEnv()
	if c.Mode != ModeDegraded {
		t.Fatalf("Mode = %s, want degraded (back-compat)", c.Mode)
	}
}

func TestConfigFromEnvExplicitFailFast(t *testing.T) {
	t.Setenv(EnvMode, "fail-fast")
	t.Setenv(EnvRedisURLPrimary, "redis://localhost:6379/0")

	c := ConfigFromEnv()
	if c.Mode != ModeFailFast {
		t.Fatalf("Mode = %s, want fail-fast", c.Mode)
	}
}

func TestBuildFailFastWithoutURLReturnsFatal(t *testing.T) {
	cfg := Config{
		Mode:              ModeFailFast,
		InstanceTTL:       60e9,
		HeartbeatInterval: 20e9,
	}
	_, _, err := Build(nil, cfg, "id")
	if err == nil {
		t.Fatal("expected error for fail-fast without URL")
	}
	if !errors.Is(err, ErrFatalConfig) {
		t.Fatalf("expected ErrFatalConfig, got %v", err)
	}
}
