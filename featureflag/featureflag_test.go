package featureflag

import (
	"testing"
	"time"
)

func TestFlag_DisabledByDefault(t *testing.T) {
	f := New()
	if f.Enabled("user1") {
		t.Fatal("flag should be off by default")
	}
}

func TestFlag_Enable(t *testing.T) {
	f := New(WithEnabled(true))
	if !f.Enabled("anyone") {
		t.Fatal("enabled flag should be on for everyone")
	}
}

func TestFlag_Percentage(t *testing.T) {
	f := New(WithEnabled(true), WithPercentage(50))
	enabled := 0
	total := 1000
	for i := 0; i < total; i++ {
		if f.Enabled("user-" + itoa(i)) {
			enabled++
		}
	}
	// ~50% with some variance; allow 40-60%.
	if enabled < total*40/100 || enabled > total*60/100 {
		t.Fatalf("percentage rollout: %d/%d enabled, expected ~50%%", enabled, total)
	}
}

func TestFlag_PercentageConsistent(t *testing.T) {
	f := New(WithEnabled(true), WithPercentage(30))
	for i := 0; i < 100; i++ {
		key := "user-" + itoa(i)
		first := f.Enabled(key)
		second := f.Enabled(key)
		if first != second {
			t.Fatalf("inconsistent result for key %q: %v then %v", key, first, second)
		}
	}
}

func TestFlag_Allowlist(t *testing.T) {
	f := New(WithEnabled(true), WithPercentage(0), WithAllowlist("vip-user"))
	if !f.Enabled("vip-user") {
		t.Fatal("allowlisted key should be enabled even at 0%")
	}
	if f.Enabled("regular-user") {
		t.Fatal("non-allowlisted key should be off at 0%")
	}
}

func TestFlag_TimeGate(t *testing.T) {
	future := time.Now().Add(time.Hour)
	f := New(WithEnabled(true), WithPercentage(100), WithStartTime(future))
	if f.Enabled("user1") {
		t.Fatal("flag should be off before start time")
	}
}

func TestFlag_RuntimeChanges(t *testing.T) {
	f := New()
	f.Enable()
	f.SetPercentage(0)
	if f.Enabled("user1") {
		t.Fatal("0% should be off for non-allowlisted")
	}
	f.Allow("user1")
	if !f.Enabled("user1") {
		t.Fatal("allowlisted user should be on")
	}
	f.Revoke("user1")
	if f.Enabled("user1") {
		t.Fatal("revoked user should be off at 0%")
	}
	f.Disable()
	if f.Enabled("vip") {
		t.Fatal("disabled flag should be off for everyone")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
