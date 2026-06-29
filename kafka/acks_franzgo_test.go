//go:build franzgo

package kafka

import (
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"
)

func TestKgoAcks(t *testing.T) {
	cases := []struct {
		in      string
		want    kgo.Acks
		wantDis bool // idempotency must be disabled?
	}{
		{"", kgo.LeaderAck(), true},         // unset → leader (unified default) + disable idempotency
		{AcksAll, kgo.AllISRAcks(), false},  // explicit all → idempotent stays on
		{AcksLeader, kgo.LeaderAck(), true}, // explicit leader → disable
		{AcksNone, kgo.NoAck(), true},       // none → disable
		{"bogus", kgo.LeaderAck(), true},    // unknown → leader (safe default) + disable
	}
	for _, c := range cases {
		if got := kgoAcks(c.in); got != c.want {
			t.Errorf("kgoAcks(%q)=%v want %v", c.in, got, c.want)
		}
		if got := kgoNeedsIdempotencyDisabled(c.in); got != c.wantDis {
			t.Errorf("kgoNeedsIdempotencyDisabled(%q)=%v want %v", c.in, got, c.wantDis)
		}
	}
}
