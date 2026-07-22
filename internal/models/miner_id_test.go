package models

import "testing"

func TestNormalizeMAC(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"AA:BB:CC:DD:EE:FF", "aa:bb:cc:dd:ee:ff"},
		{"aa-bb-cc-dd-ee-ff", "aa:bb:cc:dd:ee:ff"},
		{"aabbccddeeff", "aa:bb:cc:dd:ee:ff"},
		{"  AA:BB:CC:DD:EE:FF  ", "aa:bb:cc:dd:ee:ff"},
		{"00:00:00:00:00:00", ""},
		{"", ""},
		{"not-a-mac", ""},
		{"aa:bb:cc", ""},
	}
	for _, tc := range cases {
		if got := NormalizeMAC(tc.in); got != tc.want {
			t.Errorf("NormalizeMAC(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestStableID(t *testing.T) {
	if got := StableID("AA:BB:CC:DD:EE:FF", "rig-a", "SN1", "10.0.0.1"); got != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("prefer mac: %q", got)
	}
	if got := StableID("", "rig-a", "ABC123", "10.0.0.1"); got != "sn:abc123" {
		t.Fatalf("prefer serial: %q", got)
	}
	if got := StableID("", "Rig-A", "", "10.0.0.1"); got != "host:rig-a" {
		t.Fatalf("prefer hostname: %q", got)
	}
	if got := StableID("", "bitaxe", "", "10.0.0.1"); got != "10.0.0.1" {
		t.Fatalf("generic hostname ignored: %q", got)
	}
	if got := StableID("", "", "", "10.0.0.1"); got != "10.0.0.1" {
		t.Fatalf("fallback ip: %q", got)
	}
	if got := StableID("00:00:00:00:00:00", "rig-a", "", "10.0.0.1"); got != "host:rig-a" {
		t.Fatalf("zero mac ignored: %q", got)
	}
}

func TestApplyStableID(t *testing.T) {
	s := Snapshot{MAC: "AA-BB-CC-DD-EE-FF", Hostname: " Rig ", IP: " 10.0.0.1 "}
	ApplyStableID(&s)
	if s.MAC != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("mac %q", s.MAC)
	}
	if s.Hostname != "Rig" {
		t.Fatalf("hostname %q", s.Hostname)
	}
	if s.IP != "10.0.0.1" {
		t.Fatalf("ip %q", s.IP)
	}
	if s.ID != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("id %q", s.ID)
	}
}
