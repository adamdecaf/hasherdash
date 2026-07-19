package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadYAMLSubnets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hasherdash.yaml")
	content := `
poll_interval: 15s
subnets:
  - 192.168.1.0/24
  - 10.0.0.0/24
ips:
  - 192.168.1.50
ranges:
  - 192.168.2.1-20
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_ = os.Unsetenv("MINER_SUBNET")
	_ = os.Unsetenv("MINER_IPS")
	_ = os.Unsetenv("CONFIG_FILE")

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PollInterval != 15*time.Second {
		t.Fatalf("interval %v", cfg.PollInterval)
	}
	if len(cfg.MinerSubnets) != 2 {
		t.Fatalf("subnets %#v", cfg.MinerSubnets)
	}
	if len(cfg.MinerIPs) != 1 || cfg.MinerIPs[0] != "192.168.1.50" {
		t.Fatalf("ips %#v", cfg.MinerIPs)
	}
	if len(cfg.MinerRanges) != 1 {
		t.Fatalf("ranges %#v", cfg.MinerRanges)
	}
	if cfg.ConfigPath != path {
		t.Fatalf("path %q", cfg.ConfigPath)
	}
}

func TestLoadSingularSubnetAlias(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("subnet: 10.1.2.0/24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.MinerSubnets) != 1 || cfg.MinerSubnets[0] != "10.1.2.0/24" {
		t.Fatalf("%#v", cfg.MinerSubnets)
	}
}

func TestEnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hasherdash.yaml")
	if err := os.WriteFile(path, []byte("subnet: 192.168.0.0/24\npoll_interval: 60s\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("POLL_INTERVAL", "10s")
	t.Setenv("MINER_SUBNET", "10.0.0.0/24")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PollInterval != 10*time.Second {
		t.Fatalf("interval %v", cfg.PollInterval)
	}
	// env appends / merges unique
	found := false
	for _, s := range cfg.MinerSubnets {
		if s == "10.0.0.0/24" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected env subnet merged: %#v", cfg.MinerSubnets)
	}
}

func TestMissingExplicitConfig(t *testing.T) {
	_ = os.Unsetenv("MINER_SUBNET")
	_ = os.Unsetenv("MINER_IPS")
	_ = os.Unsetenv("MINER_SUBNETS")
	_ = os.Unsetenv("MINER_RANGES")
	_ = os.Unsetenv("CONFIG_FILE")

	missing := filepath.Join(t.TempDir(), "missing.yaml")
	if _, err := Load(missing); err == nil {
		t.Fatal("expected error for missing explicit config path")
	}
}

func TestJSONConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hasherdash.json")
	if err := os.WriteFile(path, []byte(`{"subnets":["172.16.0.0/24"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.MinerSubnets) != 1 {
		t.Fatalf("%#v", cfg)
	}
}

func TestHasDiscoveryTargets(t *testing.T) {
	var cfg Config
	if cfg.HasDiscoveryTargets() {
		t.Fatal("empty config has no targets")
	}
	cfg.MinerSubnets = []string{"192.168.1.0/24"}
	if !cfg.HasDiscoveryTargets() {
		t.Fatal("expected targets")
	}
}

func TestDefaultsRescanAndRetention(t *testing.T) {
	_ = os.Unsetenv("CONFIG_FILE")
	_ = os.Unsetenv("RESCAN_INTERVAL")
	_ = os.Unsetenv("MINER_TTL")
	_ = os.Unsetenv("HISTORY_RETENTION")
	_ = os.Unsetenv("HISTORY_POINTS")
	_ = os.Unsetenv("SQLITE_PATH")
	_ = os.Unsetenv("MINER_SUBNET")
	_ = os.Unsetenv("MINER_IPS")
	_ = os.Unsetenv("MINER_SUBNETS")
	_ = os.Unsetenv("MINER_RANGES")

	// Work from a temp dir so auto-discovery finds no config file.
	t.Chdir(t.TempDir())
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RescanInterval != 30*time.Minute {
		t.Fatalf("rescan %v", cfg.RescanInterval)
	}
	if cfg.MinerTTL != 7*24*time.Hour {
		t.Fatalf("ttl %v", cfg.MinerTTL)
	}
	if cfg.HistoryRetention != 7*24*time.Hour {
		t.Fatalf("retention %v", cfg.HistoryRetention)
	}
	if cfg.SQLitePath != "hasherdash.db" {
		t.Fatalf("sqlite_path %q", cfg.SQLitePath)
	}
	// history points sized for 7d @ 30s
	wantPts := int((7*24*time.Hour)/cfg.PollInterval) + 2
	if cfg.HistoryPoints != wantPts {
		t.Fatalf("history_points %d want %d", cfg.HistoryPoints, wantPts)
	}
}

func TestLoadRescanAndTTL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hasherdash.yaml")
	content := `
rescan_interval: 15m
miner_ttl: 48h
history_retention: 72h
poll_interval: 60s
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RescanInterval != 15*time.Minute {
		t.Fatalf("rescan %v", cfg.RescanInterval)
	}
	if cfg.MinerTTL != 48*time.Hour {
		t.Fatalf("ttl %v", cfg.MinerTTL)
	}
	if cfg.HistoryRetention != 72*time.Hour {
		t.Fatalf("retention %v", cfg.HistoryRetention)
	}
	wantPts := int(72*time.Hour/cfg.PollInterval) + 2
	if cfg.HistoryPoints < wantPts {
		t.Fatalf("points %d want >= %d", cfg.HistoryPoints, wantPts)
	}
}
