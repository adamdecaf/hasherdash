// Package axetemp holds Bitaxe/Nerdaxe helpers used by the poller.
// Kept separate so unit tests do not require cgo / asic-rs.
package axetemp

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/adamdecaf/hasherdash/internal/models"
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

// SystemInfo is the AxeOS /api/system/info subset used for temp fallback.
type SystemInfo struct {
	Chip *float64
	VR   *float64
}

// FetchSystemInfo reads Bitaxe/Nerdaxe /api/system/info for chip/VR temps.
// Returns ok=false on any transport/parse failure.
func FetchSystemInfo(ip string) (SystemInfo, bool) {
	var out SystemInfo
	if ip == "" {
		return out, false
	}
	client := &http.Client{Timeout: 2 * time.Second}
	url := "http://" + ip + "/api/system/info"
	resp, err := client.Get(url)
	if err != nil {
		return out, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return out, false
	}
	var body struct {
		Temp   *float64 `json:"temp"`
		VRTemp *float64 `json:"vrTemp"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return out, false
	}
	out.Chip = body.Temp
	out.VR = body.VRTemp
	return out, true
}

// FetchSystemTemps reads chip ("temp") and VR ("vrTemp") from /api/system/info.
// Returns ok=false when the request fails or chip temp is missing.
func FetchSystemTemps(ip string) (chip float64, vr *float64, ok bool) {
	info, ok := FetchSystemInfo(ip)
	if !ok || info.Chip == nil {
		return 0, nil, false
	}
	return *info.Chip, info.VR, true
}

// ParseDiff turns an AxeOS difficulty string ("483k", "1.2M", "12") into a number.
func ParseDiff(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	mul := 1.0
	if n := len(s); n > 0 {
		switch s[n-1] {
		case 'k', 'K':
			mul = 1e3
			s = s[:n-1]
		case 'm', 'M':
			mul = 1e6
			s = s[:n-1]
		case 'g', 'G':
			mul = 1e9
			s = s[:n-1]
		case 't', 'T':
			mul = 1e12
			s = s[:n-1]
		case 'p', 'P':
			mul = 1e15
			s = s[:n-1]
		case 'e', 'E':
			mul = 1e18
			s = s[:n-1]
		}
		s = strings.TrimSpace(s)
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v * mul, true
}

// FormatDiff renders a difficulty with AxeOS-style k/M/G/T suffixes.
func FormatDiff(v float64) string {
	if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return ""
	}
	type suf struct {
		div float64
		s   string
	}
	for _, p := range []suf{
		{1e18, "E"}, {1e15, "P"}, {1e12, "T"},
		{1e9, "G"}, {1e6, "M"}, {1e3, "k"},
	} {
		if v >= p.div {
			n := v / p.div
			prec := 2
			if n >= 100 {
				prec = 0
			} else if n >= 10 {
				prec = 1
			}
			return strconv.FormatFloat(n, 'f', prec, 64) + p.s
		}
	}
	if v == math.Trunc(v) {
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
	return strconv.FormatFloat(v, 'f', 2, 64)
}

func parseDiffJSON(raw json.RawMessage) (float64, string, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, "", false
	}
	var n float64
	if err := json.Unmarshal(raw, &n); err == nil {
		if n < 0 || math.IsNaN(n) || math.IsInf(n, 0) {
			return 0, "", false
		}
		return n, FormatDiff(n), true
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return 0, "", false
	}
	v, ok := ParseDiff(s)
	if !ok {
		return 0, "", false
	}
	text := strings.TrimSpace(s)
	if text == "" {
		text = FormatDiff(v)
	}
	return v, text, true
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
