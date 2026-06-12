package sim

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jeanmarcos/modos-cpu/pkg/kernel"
)

// TestSnapshot_IncludesMutexesAndSemaphores ensures a late joiner receives the
// full sync-object state in the snapshot, not just whatever mutex_op/sem_op
// events happen to arrive afterwards.
func TestSnapshot_IncludesMutexesAndSemaphores(t *testing.T) {
	s := New(Config{NumCores: 1})
	s.kernel.Mutexes = []*kernel.Mutex{
		{ID: 1, Holder: 3, Waiters: []int{4, 5}},
		{ID: 2, Holder: -1},
	}
	s.kernel.Semaphores = []*kernel.Semaphore{
		{ID: 1, Count: 2, Waiters: []int{6}},
	}

	snap := s.Snapshot()

	mutexes, ok := snap["mutexes"].([]MutexSnapshot)
	if !ok {
		t.Fatalf("snapshot mutexes has type %T, want []MutexSnapshot", snap["mutexes"])
	}
	if len(mutexes) != 2 {
		t.Fatalf("len(mutexes) = %d, want 2", len(mutexes))
	}
	if mutexes[0].ID != 1 || mutexes[0].Holder != 3 {
		t.Errorf("mutexes[0] = %+v, want ID=1 Holder=3", mutexes[0])
	}
	if len(mutexes[0].Waiters) != 2 || mutexes[0].Waiters[0] != 4 || mutexes[0].Waiters[1] != 5 {
		t.Errorf("mutexes[0].Waiters = %v, want [4 5]", mutexes[0].Waiters)
	}
	if mutexes[1].ID != 2 || mutexes[1].Holder != -1 {
		t.Errorf("mutexes[1] = %+v, want ID=2 Holder=-1", mutexes[1])
	}

	sems, ok := snap["semaphores"].([]SemaphoreSnapshot)
	if !ok {
		t.Fatalf("snapshot semaphores has type %T, want []SemaphoreSnapshot", snap["semaphores"])
	}
	if len(sems) != 1 {
		t.Fatalf("len(semaphores) = %d, want 1", len(sems))
	}
	if sems[0].ID != 1 || sems[0].Count != 2 {
		t.Errorf("semaphores[0] = %+v, want ID=1 Count=2", sems[0])
	}
	if len(sems[0].Waiters) != 1 || sems[0].Waiters[0] != 6 {
		t.Errorf("semaphores[0].Waiters = %v, want [6]", sems[0].Waiters)
	}
}

// TestSnapshot_SyncWaitersAreCopies guards against the snapshot sharing the
// live kernel slices: a later kernel mutation must not leak into a snapshot
// that was already handed to the encoder.
func TestSnapshot_SyncWaitersAreCopies(t *testing.T) {
	s := New(Config{NumCores: 1})
	s.kernel.Mutexes = []*kernel.Mutex{{ID: 1, Holder: 3, Waiters: []int{4, 5}}}
	s.kernel.Semaphores = []*kernel.Semaphore{{ID: 1, Count: 0, Waiters: []int{7}}}

	snap := s.Snapshot()
	s.kernel.Mutexes[0].Waiters[0] = 99
	s.kernel.Semaphores[0].Waiters[0] = 99

	mutexes := snap["mutexes"].([]MutexSnapshot)
	if mutexes[0].Waiters[0] != 4 {
		t.Errorf("mutex waiters aliased kernel slice: got %v, want [4 5]", mutexes[0].Waiters)
	}
	sems := snap["semaphores"].([]SemaphoreSnapshot)
	if sems[0].Waiters[0] != 7 {
		t.Errorf("semaphore waiters aliased kernel slice: got %v, want [7]", sems[0].Waiters)
	}
}

// TestSnapshot_EmptySyncStateSerializesAsArrays pins the wire format: with no
// mutexes or semaphores the JSON must carry [] (and waiters too), never null,
// so the frontend can assign the payload without null checks.
func TestSnapshot_EmptySyncStateSerializesAsArrays(t *testing.T) {
	s := New(Config{NumCores: 1})

	b, err := json.Marshal(s.Snapshot())
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	js := string(b)
	if !strings.Contains(js, `"mutexes":[]`) {
		t.Errorf("empty mutexes did not serialize as []: %s", js)
	}
	if !strings.Contains(js, `"semaphores":[]`) {
		t.Errorf("empty semaphores did not serialize as []: %s", js)
	}

	s.kernel.Mutexes = []*kernel.Mutex{{ID: 1, Holder: -1}}
	b, err = json.Marshal(s.Snapshot())
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if !strings.Contains(string(b), `"waiters":[]`) {
		t.Errorf("empty waiters did not serialize as []: %s", b)
	}
}
