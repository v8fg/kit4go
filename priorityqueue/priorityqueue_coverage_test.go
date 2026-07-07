package priorityqueue

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmptyPop(t *testing.T) {
	q := New[int]()
	v, prio, ok := q.Pop()
	require.False(t, ok)
	require.Equal(t, 0, v)
	require.Equal(t, 0, prio)
	require.Equal(t, 0, q.Len())
}

func TestEmptyPeek(t *testing.T) {
	q := New[string]()
	v, prio, ok := q.Peek()
	require.False(t, ok)
	require.Equal(t, "", v)
	require.Equal(t, 0, prio)
}

func TestLen(t *testing.T) {
	q := New[int]()
	require.Equal(t, 0, q.Len())
	q.Push(1, 1)
	require.Equal(t, 1, q.Len())
	q.Push(2, 2)
	q.Push(3, 3)
	require.Equal(t, 3, q.Len())
}

func TestPushPopOrdering(t *testing.T) {
	q := New[int]()
	q.Push(1, 3)
	q.Push(2, 1)
	q.Push(3, 5)
	q.Push(4, 4)
	q.Push(5, 2)

	var got []int
	var prios []int
	for q.Len() > 0 {
		v, prio, ok := q.Pop()
		require.True(t, ok)
		got = append(got, v)
		prios = append(prios, prio)
	}
	// Highest priority first: 5,4,3,2,1 → values 3,4,1,5,2.
	require.Equal(t, []int{3, 4, 1, 5, 2}, got)
	require.Equal(t, []int{5, 4, 3, 2, 1}, prios) // monotonically descending
}

func TestPeekDoesNotRemove(t *testing.T) {
	q := New[string]()
	q.Push("a", 1)
	q.Push("b", 5)

	v, prio, ok := q.Peek()
	require.True(t, ok)
	require.Equal(t, "b", v)
	require.Equal(t, 5, prio)
	require.Equal(t, 2, q.Len()) // peek did not remove

	// Pop should still return the same top.
	v2, prio2, _ := q.Pop()
	require.Equal(t, v, v2)
	require.Equal(t, prio, prio2)
	require.Equal(t, 1, q.Len())
}

func TestUpdatePromotes(t *testing.T) {
	q := New[string]()
	low := q.Push("low", 1)
	q.Push("mid", 5)
	q.Push("high", 9)

	// Promote the lowest item past everything.
	q.Update(low, 20)

	v, prio, ok := q.Pop()
	require.True(t, ok)
	require.Equal(t, "low", v)
	require.Equal(t, 20, prio)
}

func TestUpdateDemotes(t *testing.T) {
	q := New[string]()
	high := q.Push("high", 9)
	q.Push("mid", 5)
	q.Push("low", 1)

	// Demote the top below everything.
	q.Update(high, 0)

	v, prio, ok := q.Pop()
	require.True(t, ok)
	require.Equal(t, "mid", v)
	require.Equal(t, 5, prio)
}

func TestUpdateToSamePriority(t *testing.T) {
	// Updating to the same priority is a no-op re-heapify; the queue must stay
	// a valid max-heap. With both items tied at 5 either may be on top.
	q := New[int]()
	it := q.Push(1, 5)
	q.Push(2, 5)
	q.Update(it, 5)
	require.Equal(t, 2, q.Len())

	v1, p1, ok1 := q.Pop()
	require.True(t, ok1)
	require.Equal(t, 5, p1)
	require.Contains(t, []int{1, 2}, v1)

	v2, p2, ok2 := q.Pop()
	require.True(t, ok2)
	require.Equal(t, 5, p2)
	require.Contains(t, []int{1, 2}, v2)
	require.NotEqual(t, v1, v2)
}

func TestUpdateNoOpHeapConsistent(t *testing.T) {
	q := New[int]()
	a := q.Push(1, 3)
	q.Push(2, 7)
	q.Update(a, 3) // unchanged priority
	// Heap must remain valid: pop yields highest first.
	v, prio, ok := q.Pop()
	require.True(t, ok)
	require.Equal(t, 2, v)
	require.Equal(t, 7, prio)
}

func TestTieBehavior(t *testing.T) {
	// Equal priorities: heap order among ties is unspecified but the queue
	// must remain a valid max-heap (all ties share the top priority, and the
	// heap invariant holds after every pop).
	q := New[int]()
	for i := range 5 {
		q.Push(i, 10) // all same priority
	}
	require.Equal(t, 5, q.Len())

	prevPrio := 11
	for q.Len() > 0 {
		_, prio, ok := q.Pop()
		require.True(t, ok)
		require.Equal(t, 10, prio)
		require.LessOrEqual(t, prio, prevPrio) // monotonic
		prevPrio = prio
	}
}

func TestMixedPrioritiesWithNegatives(t *testing.T) {
	// Max-heap must handle negative priorities correctly (higher = dequeues
	// first, so -1 outranks -5 which outranks -10).
	q := New[int]()
	q.Push(1, -10)
	q.Push(2, -1)
	q.Push(3, -5)

	var got []int
	var prios []int
	for q.Len() > 0 {
		v, prio, ok := q.Pop()
		require.True(t, ok)
		got = append(got, v)
		prios = append(prios, prio)
	}
	require.Equal(t, []int{2, 3, 1}, got)
	require.Equal(t, []int{-1, -5, -10}, prios) // descending priority
}

func TestSingleItemLifecycle(t *testing.T) {
	q := New[int]()
	q.Push(42, 7)

	v, prio, ok := q.Peek()
	require.True(t, ok)
	require.Equal(t, 42, v)
	require.Equal(t, 7, prio)
	require.Equal(t, 1, q.Len())

	rv, rprio, rok := q.Pop()
	require.True(t, rok)
	require.Equal(t, 42, rv)
	require.Equal(t, 7, rprio)
	require.Equal(t, 0, q.Len())

	// Now empty.
	_, _, eok := q.Pop()
	require.False(t, eok)
}

func TestDrainAll(t *testing.T) {
	q := New[int]()
	for i, p := range []int{3, 1, 4, 1, 5, 9, 2, 6} {
		q.Push(i, p)
	}
	priorities := make([]int, 0, q.Len())
	for q.Len() > 0 {
		_, p, _ := q.Pop()
		priorities = append(priorities, p)
	}
	// Must be non-increasing (max-heap output).
	for i := 1; i < len(priorities); i++ {
		require.GreaterOrEqual(t, priorities[i-1], priorities[i],
			"priority out of order at %d: %v", i, priorities)
	}
	require.Equal(t, 9, priorities[0])
	require.Equal(t, 1, priorities[len(priorities)-1])
}

func TestNewReturnsUsableQueue(t *testing.T) {
	q := New[struct{}]()
	require.NotNil(t, q)
	require.Equal(t, 0, q.Len())
}

// TestUpdate_StaleItemIsNoOp covers the Update guard: updating a popped item
// (index < 0), a nil item, or an out-of-range index is a silent no-op, not a
// panic (heap.Fix on a stale index would otherwise index out of bounds).
func TestUpdate_StaleItemIsNoOp(t *testing.T) {
	q := New[int]()
	it := q.Push(1, 5)
	v, _, ok := q.Pop()
	require.True(t, ok)
	require.Equal(t, 1, v)

	// it is now popped (index == -1): Update must be a no-op, not panic.
	require.NotPanics(t, func() { q.Update(it, 99) })
	require.NotPanics(t, func() { q.Update(nil, 1) })
	require.Equal(t, 0, q.Len())
}
