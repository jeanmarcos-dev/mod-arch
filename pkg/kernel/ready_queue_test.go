package kernel

import (
	"reflect"
	"sync"
	"testing"
)

func TestReadyQueue_PushPop(t *testing.T) {
	q := NewReadyQueue()
	if q.Len() != 0 {
		t.Fatalf("fresh queue len = %d, want 0", q.Len())
	}
	q.Push(1)
	q.Push(2)
	q.Push(3)
	if q.Len() != 3 {
		t.Fatalf("len after 3 pushes = %d, want 3", q.Len())
	}
	for _, want := range []int{1, 2, 3} {
		got, ok := q.Pop()
		if !ok || got != want {
			t.Fatalf("Pop = (%d,%v), want (%d,true)", got, ok, want)
		}
	}
	if _, ok := q.Pop(); ok {
		t.Fatal("Pop on empty should return ok=false")
	}
}

func TestReadyQueue_Snapshot(t *testing.T) {
	q := NewReadyQueue()
	q.Push(7)
	q.Push(8)
	snap := q.Snapshot()
	if !reflect.DeepEqual(snap, []int{7, 8}) {
		t.Fatalf("Snapshot = %v, want [7 8]", snap)
	}
	// Mutating the snapshot must not affect the queue.
	snap[0] = 999
	again := q.Snapshot()
	if !reflect.DeepEqual(again, []int{7, 8}) {
		t.Fatalf("queue mutated via snapshot: %v", again)
	}
}

func TestReadyQueue_Remove(t *testing.T) {
	q := NewReadyQueue()
	q.Push(1)
	q.Push(2)
	q.Push(3)
	if !q.Remove(2) {
		t.Fatal("Remove(2) returned false")
	}
	if got := q.Snapshot(); !reflect.DeepEqual(got, []int{1, 3}) {
		t.Fatalf("after Remove(2), queue = %v, want [1 3]", got)
	}
	if q.Remove(42) {
		t.Fatal("Remove(missing) returned true")
	}
}

func TestReadyQueue_ConcurrentPushPop(t *testing.T) {
	// Stress: many goroutines pushing/popping concurrently. The test does not
	// assert ordering across goroutines (impossible to predict) — only that no
	// data race or panic occurs and counts balance.
	q := NewReadyQueue()
	const N = 1000
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range N {
			q.Push(i)
		}
	}()
	popped := 0
	go func() {
		defer wg.Done()
		for popped < N {
			if _, ok := q.Pop(); ok {
				popped++
			}
		}
	}()
	wg.Wait()
	if q.Len() != 0 {
		t.Fatalf("after stress, len = %d, want 0", q.Len())
	}
}
