package kernel

import "sync"

// ReadyQueue is the FIFO of PIDs waiting for a core to become available.
//
// In the single-core kernel this was a plain []int field, but as soon as
// multiple cores can dequeue concurrently the slice needs a mutex around it.
// Even with the simulator's current sequential Tick(), pulling the queue out
// of Kernel into its own type makes the contract explicit and is the natural
// place to hang future events like ready_queue_op.
type ReadyQueue struct {
	mu    sync.Mutex
	items []int
}

// NewReadyQueue builds an empty ready queue.
func NewReadyQueue() *ReadyQueue {
	return &ReadyQueue{}
}

// Push appends a PID to the tail of the queue.
func (q *ReadyQueue) Push(pid int) {
	q.mu.Lock()
	q.items = append(q.items, pid)
	q.mu.Unlock()
}

// Pop removes and returns the PID at the head, plus true on success. Returns
// (0, false) when the queue is empty.
func (q *ReadyQueue) Pop() (int, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return 0, false
	}
	pid := q.items[0]
	q.items = q.items[1:]
	return pid, true
}

// Peek returns the PID at the head without removing it. Returns (0, false) on
// an empty queue.
func (q *ReadyQueue) Peek() (int, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return 0, false
	}
	return q.items[0], true
}

// Len returns the number of waiting PIDs.
func (q *ReadyQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

// Snapshot returns a copy of the queued PIDs in FIFO order. The result is
// safe to embed in events without worrying about later mutations.
//
// Returns a non-nil empty slice for an empty queue so JSON consumers see
// "ready_queue": [] instead of "ready_queue": null — frontend code that
// spreads the value (e.g. [...ev.ready_queue]) would otherwise crash.
func (q *ReadyQueue) Snapshot() []int {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]int, len(q.items))
	copy(out, q.items)
	return out
}

// Remove deletes the first occurrence of pid and returns true if one was
// found. Used when a process exits while sitting in the ready queue.
func (q *ReadyQueue) Remove(pid int) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, p := range q.items {
		if p == pid {
			q.items = append(q.items[:i], q.items[i+1:]...)
			return true
		}
	}
	return false
}

// Reset empties the queue. Used by Kernel.Reset to wipe state between demos.
func (q *ReadyQueue) Reset() {
	q.mu.Lock()
	q.items = nil
	q.mu.Unlock()
}
