package axetemp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adamdecaf/hasherdash/internal/models"
)

func f64(v float64) *float64 { return &v }

func TestSameTemp(t *testing.T) {
	if !SameTemp(f64(78), f64(78)) {
		t.Fatal("equal should match")
	}
	if !SameTemp(f64(78.01), f64(78.04)) {
		t.Fatal("within eps should match")
	}
	if SameTemp(f64(59.875), f64(78)) {
		t.Fatal("distinct chip vs VR should not match")
	}
	if SameTemp(f64(70), nil) {
		t.Fatal("nil should not match")
	}
}

func TestIsAxeFamily(t *testing.T) {
	for _, make := range []string{"Bitaxe", "Nerdaxe", "NerdQAxe", "bitaxe"} {
		if !IsAxeFamily(make) {
			t.Fatalf("%q should be axe family", make)
		}
	}
	if IsAxeFamily("Antminer") {
		t.Fatal("Antminer is not axe family")
	}
}

func TestNeedsFallback(t *testing.T) {
	// Missing ASIC on Bitaxe → need fallback.
	if !NeedsFallback(models.Snapshot{Make: "Bitaxe", HasVRTemp: true, VRTempMax: 78}) {
		t.Fatal("missing ASIC should need fallback")
	}
	// Conflated ASIC==VR → need fallback.
	if !NeedsFallback(models.Snapshot{
		Make: "Bitaxe", HasASICTemp: true, ASICTempMin: 78, ASICTempMax: 78,
		HasVRTemp: true, VRTempMin: 78, VRTempMax: 78,
	}) {
		t.Fatal("ASIC==VR should need fallback")
	}
	// Distinct readings → no fallback.
	if NeedsFallback(models.Snapshot{
		Make: "Bitaxe", HasASICTemp: true, ASICTempMin: 59.9, ASICTempMax: 59.9,
		HasVRTemp: true, VRTempMin: 78, VRTempMax: 78,
	}) {
		t.Fatal("distinct ASIC/VR should not need fallback")
	}
	// Non-axe with only VR is fine.
	if NeedsFallback(models.Snapshot{Make: "Antminer", HasVRTemp: true, VRTempMax: 70}) {
		t.Fatal("non-axe should not fallback")
	}
}

func TestMinMax(t *testing.T) {
	min, max, ok := MinMax([]float64{60, 55, 72})
	if !ok || min != 55 || max != 72 {
		t.Fatalf("got %v %v %v", min, max, ok)
	}
	if _, _, ok := MinMax(nil); ok {
		t.Fatal("empty should not be ok")
	}
}

func TestParseDiff(t *testing.T) {
	cases := []struct {
		in   string
		want float64
		ok   bool
	}{
		{"", 0, false},
		{"0", 0, true},
		{"12", 12, true},
		{"483k", 483e3, true},
		{"1.2M", 1.2e6, true},
		{"1.23G", 1.23e9, true},
		{"4.5T", 4.5e12, true},
		{"  2.0k ", 2e3, true},
		{"nope", 0, false},
		{"-1", 0, false},
	}
	for _, tc := range cases {
		got, ok := ParseDiff(tc.in)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("ParseDiff(%q) = %v, %v; want %v, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestFormatDiff(t *testing.T) {
	if got := FormatDiff(483000); got != "483k" {
		t.Fatalf("483000 → %q", got)
	}
	if got := FormatDiff(1.2e6); got != "1.20M" {
		t.Fatalf("1.2e6 → %q", got)
	}
	if got := FormatDiff(12); got != "12" {
		t.Fatalf("12 → %q", got)
	}
}

func TestFetchSystemInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/system/info" {
			t.Errorf("path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"temp": 59.9,
			"vrTemp": 78.1,
			"bestDiff": "483k",
			"bestSessionDiff": "12.5M"
		}`))
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	info, ok := FetchSystemInfo(host)
	if !ok {
		t.Fatal("FetchSystemInfo failed")
	}
	if info.Chip == nil || *info.Chip != 59.9 {
		t.Fatalf("chip %#v", info.Chip)
	}
	if info.VR == nil || *info.VR != 78.1 {
		t.Fatalf("vr %#v", info.VR)
	}
	if !info.HasBestDiff || info.BestDiff != 483e3 || info.BestDiffText != "483k" {
		t.Fatalf("best %#v", info)
	}
	if !info.HasSessionDiff || info.SessionDiff != 12.5e6 || info.SessionDiffText != "12.5M" {
		t.Fatalf("session %#v", info)
	}
	chip, vr, ok := FetchSystemTemps(host)
	if !ok || chip != 59.9 || vr == nil || *vr != 78.1 {
		t.Fatalf("FetchSystemTemps %v %v %v", chip, vr, ok)
	}
}
