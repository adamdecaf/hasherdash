package models

import "time"

// Snapshot is a compact, filter-friendly view of one miner at poll time.
type Snapshot struct {
	ID          string  `json:"id"` // IP
	IP          string  `json:"ip"`
	MAC         string  `json:"mac,omitempty"`
	Hostname    string  `json:"hostname,omitempty"`
	Make        string  `json:"make"`
	Model       string  `json:"model"`
	Firmware    string  `json:"firmware"`
	FirmwareVer string  `json:"firmware_version,omitempty"`
	Algo        string  `json:"algo"`
	Serial      string  `json:"serial,omitempty"`
	IsMining    bool    `json:"is_mining"`
	HashrateTH  float64 `json:"hashrate_th"`
	ExpectedTH  float64 `json:"expected_th,omitempty"`
	Wattage     float64 `json:"wattage,omitempty"`
	Efficiency  float64 `json:"efficiency,omitempty"` // J/TH
	// AvgTempC is the miner-reported average (legacy / chart default).
	AvgTempC float64 `json:"avg_temp_c,omitempty"`
	// ASIC chip temperatures (°C) — min/max across boards (inlet/outlet sensors).
	// Has* flags distinguish "unknown" from a real 0 °C reading.
	HasASICTemp bool    `json:"has_asic_temp,omitempty"`
	ASICTempMin float64 `json:"asic_temp_min,omitempty"`
	ASICTempMax float64 `json:"asic_temp_max,omitempty"`
	// VR / board (PCB) temperatures (°C) — min/max of board_temperature sensors.
	HasVRTemp     bool      `json:"has_vr_temp,omitempty"`
	VRTempMin     float64   `json:"vr_temp_min,omitempty"`
	VRTempMax     float64   `json:"vr_temp_max,omitempty"`
	FluidTempC    float64   `json:"fluid_temp_c,omitempty"`
	TotalChips    int       `json:"total_chips,omitempty"`
	ExpectedChips int       `json:"expected_chips,omitempty"`
	Boards        int       `json:"boards,omitempty"`
	Fans          int       `json:"fans,omitempty"`
	UptimeSec     int64     `json:"uptime_sec,omitempty"`
	LightFlashing bool      `json:"light_flashing,omitempty"`
	PoolUsers     []string  `json:"pool_users,omitempty"`
	PoolHosts     []string  `json:"pool_hosts,omitempty"`
	Messages      []string  `json:"messages,omitempty"`
	Error         string    `json:"error,omitempty"`
	LastSeen      time.Time `json:"last_seen"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Detail is the full snapshot plus raw-ish board/fan/pool rows for the side panel.
type Detail struct {
	Snapshot
	Hashboards []BoardRow `json:"hashboards,omitempty"`
	FanRPMs    []float64  `json:"fan_rpms,omitempty"`
	Pools      []PoolRow  `json:"pools,omitempty"`
}

// BoardRow is per-hashboard telemetry for the detail panel.
type BoardRow struct {
	Position   int     `json:"position"`
	HashrateTH float64 `json:"hashrate_th,omitempty"`
	// VRTempC is board/PCB temperature (often the VR / board sensor).
	VRTempC   float64 `json:"vr_temp_c,omitempty"`
	HasVRTemp bool    `json:"has_vr_temp,omitempty"`
	// ASIC inlet (coolest) / outlet (hottest) chip temps on this board.
	ASICTempIn  float64 `json:"asic_temp_in,omitempty"`
	ASICTempOut float64 `json:"asic_temp_out,omitempty"`
	HasASICIn   bool    `json:"has_asic_in,omitempty"`
	HasASICOut  bool    `json:"has_asic_out,omitempty"`
	// BoardTempC kept for compatibility (same as VRTempC when present).
	BoardTempC    float64 `json:"board_temp_c,omitempty"`
	WorkingChips  int     `json:"working_chips,omitempty"`
	ExpectedChips int     `json:"expected_chips,omitempty"`
	Frequency     float64 `json:"frequency,omitempty"`
	Voltage       float64 `json:"voltage,omitempty"`
	Active        *bool   `json:"active,omitempty"`
	Tuned         *bool   `json:"tuned,omitempty"`
}

// TempRange is a helper for JSON-friendly min/max display.
type TempRange struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// PoolRow is a flattened pool endpoint for display.
type PoolRow struct {
	Group    string `json:"group,omitempty"`
	URL      string `json:"url,omitempty"`
	User     string `json:"user,omitempty"`
	Active   *bool  `json:"active,omitempty"`
	Alive    *bool  `json:"alive,omitempty"`
	Accepted uint64 `json:"accepted,omitempty"`
	Rejected uint64 `json:"rejected,omitempty"`
}

// HistoryPoint is one metric sample for charting.
type HistoryPoint struct {
	T time.Time `json:"t"`
	V float64   `json:"v"`
}

// Series is a named time series for one miner + metric.
type Series struct {
	ID       string         `json:"id"`
	Label    string         `json:"label"`
	Metric   string         `json:"metric"`
	Make     string         `json:"make,omitempty"`
	Model    string         `json:"model,omitempty"`
	Firmware string         `json:"firmware,omitempty"`
	Algo     string         `json:"algo,omitempty"`
	Points   []HistoryPoint `json:"points"`
}

// Meta is fleet-level status returned to the UI.
type Meta struct {
	PollIntervalSec int       `json:"poll_interval_sec"`
	HistoryPoints   int       `json:"history_points"`
	LastPollAt      time.Time `json:"last_poll_at,omitempty"`
	LastPollErr     string    `json:"last_poll_err,omitempty"`
	MinerCount      int       `json:"miner_count"`
	Polling         bool      `json:"polling"`
	Makes           []string  `json:"makes"`
	Models          []string  `json:"models"`
	Firmwares       []string  `json:"firmwares"`
	Algos           []string  `json:"algos"`
}
