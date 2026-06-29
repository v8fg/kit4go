//go:build !franzgo

package kafka

import (
	"testing"

	"github.com/IBM/sarama"
)

func TestSaramaAcks(t *testing.T) {
	cases := []struct {
		in   string
		want sarama.RequiredAcks
	}{
		{"", sarama.WaitForLocal}, // unset → native leader
		{AcksLeader, sarama.WaitForLocal},
		{AcksAll, sarama.WaitForAll},
		{AcksNone, sarama.NoResponse},
		{"bogus", sarama.WaitForLocal}, // unknown → leader (safe default)
	}
	for _, c := range cases {
		if got := saramaAcks(c.in); got != c.want {
			t.Errorf("saramaAcks(%q)=%v want %v", c.in, got, c.want)
		}
	}
}
