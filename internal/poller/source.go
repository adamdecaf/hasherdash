package poller

import "github.com/adamdecaf/minerdash/internal/config"

// NewSource returns the asic-rs poller for real miner discovery and telemetry.
func NewSource(cfg config.Config) Source {
	return NewAsicSource(cfg)
}
