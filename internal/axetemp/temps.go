// Package axetemp holds Bitaxe/Nerdaxe temperature helpers used by the poller.
// Kept separate so unit tests do not require cgo / asic-rs-go.
package axetemp

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/adamdecaf/minerdash/internal/models"
)

// SameTemp reports whether two optional °C readings match within a small
// tolerance (sensor noise / float noise).
func SameTemp(a, b *float64) bool {
	if a == nil || b == nil {
		return false
	}
	const eps = 0.05
	d := *a - *b
	if d < 0 {
		d = -d
	}
	return d < eps
}

// IsAxeFamily reports whether make is Bitaxe / Nerdaxe / NerdQAxe.
func IsAxeFamily(make string) bool {
	m := strings.ToLower(strings.TrimSpace(make))
	return strings.Contains(m, "bitaxe") ||
		strings.Contains(m, "nerdaxe") ||
		strings.Contains(m, "nerdqaxe")
}

// NeedsFallback is true when asic-rs left us without a real ASIC reading
// (or only VR-mirrored values) on an Axe-family device.
func NeedsFallback(snap models.Snapshot) bool {
	if !IsAxeFamily(snap.Make) {
		return false
	}
	if !snap.HasASICTemp {
		return true
	}
	// ASIC equal to VR is the asic-rs conflation signature on single-chip axes.
	if snap.HasVRTemp && SameTemp(&snap.ASICTempMax, &snap.VRTempMax) &&
		SameTemp(&snap.ASICTempMin, &snap.VRTempMin) {
		return true
	}
	return false
}

// FetchSystemTemps reads Bitaxe/Nerdaxe /api/system/info for the real chip
// temp ("temp") and VR temp ("vrTemp"). Returns ok=false on any failure.
func FetchSystemTemps(ip string) (chip float64, vr *float64, ok bool) {
	if ip == "" {
		return 0, nil, false
	}
	client := &http.Client{Timeout: 2 * time.Second}
	url := "http://" + ip + "/api/system/info"
	resp, err := client.Get(url)
	if err != nil {
		return 0, nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, nil, false
	}
	var body struct {
		Temp   *float64 `json:"temp"`
		VRTemp *float64 `json:"vrTemp"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, nil, false
	}
	if body.Temp == nil {
		return 0, nil, false
	}
	return *body.Temp, body.VRTemp, true
}

// MinMax returns the min and max of vals.
func MinMax(vals []float64) (min, max float64, ok bool) {
	if len(vals) == 0 {
		return 0, 0, false
	}
	min, max = vals[0], vals[0]
	for _, v := range vals[1:] {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return min, max, true
}
