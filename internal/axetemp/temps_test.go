package axetemp

import (
	"testing"

	"github.com/adamdecaf/minerdash/internal/models"
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
