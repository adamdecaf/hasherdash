package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds runtime settings for hasherdash.
type Config struct {
	HTTPAddr         string
	PollInterval     time.Duration
	HistoryPoints    int
	HistoryRetention time.Duration // how long metric samples are kept
	// SQLitePath is where metric samples are persisted.
	// Default "hasherdash.db". Set to "off" for in-memory rings only.
	SQLitePath     string
	MinerIPs       []string
	MinerSubnets   []string // CIDR ranges, e.g. 192.168.1.0/24
	MinerRanges    []string // asic-rs range strings, e.g. 192.168.1.1-50
	ScanTimeoutSec int
	Concurrent     int
	// RescanInterval is how often to re-scan subnets/ranges for new miners.
	// Zero or negative disables periodic rescans (startup scan still runs).
	RescanInterval time.Duration
	// MinerTTL is how long to keep a miner after last successful sighting.
	MinerTTL time.Duration

	// ConfigPath is the file that was loaded, if any (for logging).
	ConfigPath string
}

// fileConfig is the on-disk representation (YAML or JSON).
type fileConfig struct {
	HTTPAddr         string `yaml:"http_addr" json:"http_addr"`
	PollInterval     string `yaml:"poll_interval" json:"poll_interval"`
	HistoryPoints    *int   `yaml:"history_points" json:"history_points"`
	HistoryRetention string `yaml:"history_retention" json:"history_retention"`
	SQLitePath       string `yaml:"sqlite_path" json:"sqlite_path"`
	ScanTimeoutSec   *int   `yaml:"scan_timeout_sec" json:"scan_timeout_sec"`
	Concurrent       *int   `yaml:"scan_concurrent" json:"scan_concurrent"`
	RescanInterval   string `yaml:"rescan_interval" json:"rescan_interval"`
	MinerTTL          string `yaml:"miner_ttl" json:"miner_ttl"`

	IPs     []string `yaml:"ips" json:"ips"`
	Subnets []string `yaml:"subnets" json:"subnets"`
	Ranges  []string `yaml:"ranges" json:"ranges"`

	// Singular aliases
	IP     string `yaml:"ip" json:"ip"`
	Subnet string `yaml:"subnet" json:"subnet"`
	Range  string `yaml:"range" json:"range"`

	// Legacy names
	MinerIPs     []string `yaml:"miner_ips" json:"miner_ips"`
	MinerSubnet  string   `yaml:"miner_subnet" json:"miner_subnet"`
	MinerSubnets []string `yaml:"miner_subnets" json:"miner_subnets"`
	MinerRanges  []string `yaml:"miner_ranges" json:"miner_ranges"`
}

// Load reads configuration from an optional config file, then environment
// overrides (env wins when set).
//
// Path resolution:
//  1. path argument (from -config), if non-empty
//  2. CONFIG_FILE environment variable
//  3. first existing of: hasherdash.yaml, hasherdash.yml, config.yaml,
//     config.yml, hasherdash.json, config.json (current working directory)
//
// Explicit paths that cannot be read return an error. Auto-discovery is optional.
func Load(path string) (Config, error) {
	cfg := Config{
		HTTPAddr:         ":8080",
		PollInterval:     30 * time.Second,
		HistoryPoints:    0, // sized from HistoryRetention in clamp
		HistoryRetention: 7 * 24 * time.Hour,
		SQLitePath:       "hasherdash.db",
		ScanTimeoutSec:   8,
		Concurrent:       200,
		RescanInterval:   30 * time.Minute,
		MinerTTL:          7 * 24 * time.Hour,
	}

	resolved, explicit := resolvePath(path)
	if resolved != "" {
		fc, err := readFile(resolved)
		if err != nil {
			if explicit {
				return cfg, fmt.Errorf("config file %s: %w", resolved, err)
			}
			return cfg, fmt.Errorf("config file %s: %w", resolved, err)
		}
		applyFile(&cfg, fc)
		cfg.ConfigPath = resolved
	}

	applyEnv(&cfg)
	clamp(&cfg)
	return cfg, nil
}

func resolvePath(flagPath string) (path string, explicit bool) {
	if p := strings.TrimSpace(flagPath); p != "" {
		return p, true
	}
	if p := strings.TrimSpace(os.Getenv("CONFIG_FILE")); p != "" {
		return p, true
	}
	for _, c := range []string{
		"hasherdash.yaml", "hasherdash.yml",
		"config.yaml", "config.yml",
		"hasherdash.json", "config.json",
	} {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, false
		}
	}
	return "", false
}

func readFile(path string) (fileConfig, error) {
	var fc fileConfig
	data, err := os.ReadFile(path)
	if err != nil {
		return fc, err
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		err = json.Unmarshal(data, &fc)
	default:
		err = yaml.Unmarshal(data, &fc)
	}
	return fc, err
}

func applyFile(cfg *Config, fc fileConfig) {
	if v := strings.TrimSpace(fc.HTTPAddr); v != "" {
		cfg.HTTPAddr = v
	}
	if v := strings.TrimSpace(fc.PollInterval); v != "" {
		if d, err := parseDuration(v); err == nil {
			cfg.PollInterval = d
		}
	}
	if fc.HistoryPoints != nil {
		cfg.HistoryPoints = *fc.HistoryPoints
	}
	if v := strings.TrimSpace(fc.HistoryRetention); v != "" {
		if d, err := parseDuration(v); err == nil {
			cfg.HistoryRetention = d
		}
	}
	if v := strings.TrimSpace(fc.SQLitePath); v != "" {
		cfg.SQLitePath = v
	}
	if fc.ScanTimeoutSec != nil {
		cfg.ScanTimeoutSec = *fc.ScanTimeoutSec
	}
	if fc.Concurrent != nil {
		cfg.Concurrent = *fc.Concurrent
	}
	if v := strings.TrimSpace(fc.RescanInterval); v != "" {
		if d, err := parseDuration(v); err == nil {
			cfg.RescanInterval = d
		}
	}
	if v := strings.TrimSpace(fc.MinerTTL); v != "" {
		if d, err := parseDuration(v); err == nil {
			cfg.MinerTTL = d
		}
	}

	cfg.MinerIPs = appendUnique(cfg.MinerIPs, fc.IPs...)
	cfg.MinerIPs = appendUnique(cfg.MinerIPs, fc.MinerIPs...)
	if v := strings.TrimSpace(fc.IP); v != "" {
		cfg.MinerIPs = appendUnique(cfg.MinerIPs, v)
	}

	cfg.MinerSubnets = appendUnique(cfg.MinerSubnets, fc.Subnets...)
	cfg.MinerSubnets = appendUnique(cfg.MinerSubnets, fc.MinerSubnets...)
	if v := strings.TrimSpace(fc.Subnet); v != "" {
		cfg.MinerSubnets = appendUnique(cfg.MinerSubnets, v)
	}
	if v := strings.TrimSpace(fc.MinerSubnet); v != "" {
		cfg.MinerSubnets = appendUnique(cfg.MinerSubnets, v)
	}

	cfg.MinerRanges = appendUnique(cfg.MinerRanges, fc.Ranges...)
	cfg.MinerRanges = appendUnique(cfg.MinerRanges, fc.MinerRanges...)
	if v := strings.TrimSpace(fc.Range); v != "" {
		cfg.MinerRanges = appendUnique(cfg.MinerRanges, v)
	}
}

func applyEnv(cfg *Config) {
	if v := strings.TrimSpace(os.Getenv("HTTP_ADDR")); v != "" {
		cfg.HTTPAddr = v
	}
	if v := strings.TrimSpace(os.Getenv("POLL_INTERVAL")); v != "" {
		if d, err := parseDuration(v); err == nil {
			cfg.PollInterval = d
		}
	}
	if v := strings.TrimSpace(os.Getenv("HISTORY_POINTS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.HistoryPoints = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("HISTORY_RETENTION")); v != "" {
		if d, err := parseDuration(v); err == nil {
			cfg.HistoryRetention = d
		}
	}
	if v, ok := os.LookupEnv("SQLITE_PATH"); ok {
		cfg.SQLitePath = strings.TrimSpace(v)
	}
	if v := strings.TrimSpace(os.Getenv("SCAN_TIMEOUT_SEC")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.ScanTimeoutSec = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("SCAN_CONCURRENT")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Concurrent = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("RESCAN_INTERVAL")); v != "" {
		if d, err := parseDuration(v); err == nil {
			cfg.RescanInterval = d
		}
	}
	if v := strings.TrimSpace(os.Getenv("MINER_TTL")); v != "" {
		if d, err := parseDuration(v); err == nil {
			cfg.MinerTTL = d
		}
	}

	if ips := strings.TrimSpace(os.Getenv("MINER_IPS")); ips != "" {
		cfg.MinerIPs = appendUnique(cfg.MinerIPs, splitCSV(ips)...)
	}
	if sub := strings.TrimSpace(os.Getenv("MINER_SUBNET")); sub != "" {
		cfg.MinerSubnets = appendUnique(cfg.MinerSubnets, splitCSV(sub)...)
	}
	if subs := strings.TrimSpace(os.Getenv("MINER_SUBNETS")); subs != "" {
		cfg.MinerSubnets = appendUnique(cfg.MinerSubnets, splitCSV(subs)...)
	}
	if ranges := strings.TrimSpace(os.Getenv("MINER_RANGES")); ranges != "" {
		cfg.MinerRanges = appendUnique(cfg.MinerRanges, splitCSV(ranges)...)
	}
}

func clamp(cfg *Config) {
	if cfg.PollInterval < 5*time.Second {
		cfg.PollInterval = 5 * time.Second
	}
	if cfg.HistoryRetention < cfg.PollInterval {
		cfg.HistoryRetention = 7 * 24 * time.Hour
	}
	// Ring capacity must cover retention at the poll cadence (plus a small buffer).
	needed := int(cfg.HistoryRetention/cfg.PollInterval) + 2
	if needed < 10 {
		needed = 10
	}
	if cfg.HistoryPoints < needed {
		cfg.HistoryPoints = needed
	}
	if cfg.ScanTimeoutSec < 1 {
		cfg.ScanTimeoutSec = 1
	}
	if cfg.Concurrent < 1 {
		cfg.Concurrent = 1
	}
	// RescanInterval <= 0 means "scan at startup only" (no periodic rescan).
	// MinerTTL <= 0 means "never prune".
	if cfg.MinerTTL < 0 {
		cfg.MinerTTL = 0
	}
}

// HasDiscoveryTargets reports whether any IP/subnet/range is configured.
func (c Config) HasDiscoveryTargets() bool {
	return len(c.MinerIPs) > 0 || len(c.MinerSubnets) > 0 || len(c.MinerRanges) > 0
}

// Summary is a short log line describing discovery settings.
func (c Config) Summary() string {
	var parts []string
	if c.ConfigPath != "" {
		parts = append(parts, "config="+c.ConfigPath)
	}
	if len(c.MinerSubnets) > 0 {
		parts = append(parts, "subnets="+strings.Join(c.MinerSubnets, ","))
	}
	if len(c.MinerRanges) > 0 {
		parts = append(parts, "ranges="+strings.Join(c.MinerRanges, ","))
	}
	if len(c.MinerIPs) > 0 {
		parts = append(parts, fmt.Sprintf("ips=%d", len(c.MinerIPs)))
	}
	if !c.HasDiscoveryTargets() {
		parts = append(parts, "targets=none")
	}
	parts = append(parts, "interval="+c.PollInterval.String())
	if c.RescanInterval > 0 {
		parts = append(parts, "rescan="+c.RescanInterval.String())
	} else {
		parts = append(parts, "rescan=off")
	}
	if c.MinerTTL > 0 {
		parts = append(parts, "miner_ttl="+c.MinerTTL.String())
	} else {
		parts = append(parts, "miner_ttl=forever")
	}
	parts = append(parts, "history="+c.HistoryRetention.String())
	switch {
	case c.SQLitePath == "" || strings.EqualFold(c.SQLitePath, "off"):
		parts = append(parts, "sqlite=off")
	default:
		parts = append(parts, "sqlite="+c.SQLitePath)
	}
	return strings.Join(parts, " ")
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func appendUnique(dst []string, vals ...string) []string {
	seen := make(map[string]struct{}, len(dst))
	for _, v := range dst {
		seen[v] = struct{}{}
	}
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		dst = append(dst, v)
	}
	return dst
}

func parseDuration(v string) (time.Duration, error) {
	v = strings.TrimSpace(v)
	if n, err := strconv.Atoi(v); err == nil {
		return time.Duration(n) * time.Second, nil
	}
	return time.ParseDuration(v)
}
