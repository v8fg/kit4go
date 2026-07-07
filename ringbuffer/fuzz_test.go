package ringbuffer

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// FuzzRingBufferPushPop fuzzes an arbitrary push/pop sequence against a
// fixed-capacity ring buffer. It drives the non-blocking TryPush/TryPop API
// (the blocking Push/Pop would stall the fuzzer when the buffer fills up) and
// asserts three invariants that must hold for every input:
//
//  1. No operation in the sequence ever panics.
//  2. TryPush reports false exactly when the buffer is full; TryPop reports
//     false exactly when it is empty.
//  3. The items returned by TryPop come out in FIFO order — the oldest item
//     not yet popped, which (after wrap-around) is the head of a logical
//     queue of size at most Cap. Len never exceeds Cap.
//
// The fuzz input is a raw byte blob consumed one byte at a time: even bytes
// (0x00..0x7f) drive a TryPush carrying that byte as its value; odd bytes
// drive a TryPop. An independent golden model (a Go slice used as a bounded
// FIFO) mirrors every accepted operation so the ring buffer's output can be
// checked against it byte-for-byte. The seed corpus covers the empty buffer,
// pure-push saturation (full), pure-pop drain, wrap-around, and mixed churn.
func FuzzRingBufferPushPop(f *testing.F) {
	// Seeds: each blob is a stream of ops. Even byte => push that byte; odd => pop.
	f.Add([]byte{})                          // empty op stream
	f.Add([]byte{0x00, 0x02, 0x04, 0x06})    // push only -> saturates capacity 4
	f.Add([]byte{0x01, 0x03, 0x05})          // pop only on empty buffer -> all no-ops
	f.Add([]byte{0x00, 0x01, 0x02, 0x03})    // push, pop, push, pop -> churn, no wrap
	f.Add([]byte{0x00, 0x02, 0x04, 0x06, 0x08, 0x0a, 0x01, 0x03, 0x05, 0x07}) // saturate, then drain (wrap-around)
	f.Add([]byte{0x10, 0x12, 0x14, 0x01, 0x16, 0x03, 0x18, 0x05})             // interleaved push/pop past capacity

	f.Fuzz(func(t *testing.T, ops []byte) {
		const cap = 4
		rb := New[byte](cap)
		require.Equal(t, cap, rb.Cap(), "Cap must match requested capacity")

		// Golden model: a bounded FIFO holding the items currently in the buffer.
		// It accepts a push only when not full and a pop only when not empty,
		// mirroring TryPush/TryPop semantics exactly.
		model := make([]byte, 0, cap)

		for _, op := range ops {
			isPush := op&0x80 == 0 // even (high bit clear) => push; odd => pop
			if isPush {
				val := op & 0x7f
				accepted := rb.TryPush(val)
				wantAccepted := len(model) < cap
				require.Equalf(t, wantAccepted, accepted,
					"TryPush(%#x)=%v but model expected %v (model len=%d, cap=%d)",
					val, accepted, wantAccepted, len(model), cap)
				if accepted {
					model = append(model, val)
				}
			} else {
				val, ok := rb.TryPop()
				wantOk := len(model) > 0
				require.Equalf(t, wantOk, ok,
					"TryPop ok=%v but model expected %v (model len=%d)",
					ok, wantOk, len(model))
				if ok {
					// FIFO: the popped item must be the oldest surviving push.
					require.Equalf(t, model[0], val,
						"FIFO broken: popped %#x, model head %#x (model=%v)",
						val, model[0], model)
					model = model[1:]
				}
			}

			// Invariant: length is consistent with the model and bounded by Cap.
			require.LessOrEqualf(t, rb.Len(), cap,
				"Len=%d exceeded Cap=%d (model len=%d)", rb.Len(), cap, len(model))
			require.Equal(t, len(model), rb.Len(),
				"Len diverged from model (rb=%d, model=%d)", rb.Len(), len(model))
		}

		// Final drain must yield the model contents in FIFO order. Drain is a
		// bulk Pop and must not reorder or drop items.
		out := rb.Drain()
		require.Equal(t, model, out, "Drain did not match model in FIFO order")
		require.True(t, rb.IsEmpty(), "buffer must be empty after Drain")
		require.Equal(t, 0, rb.Len(), "Len must be 0 after Drain")
	})
}
