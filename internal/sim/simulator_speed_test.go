package sim

import (
	"testing"
	"time"
)

// TestTickInterval_Formula pins down the (clockHz, speedMul) → time.Duration
// math the run loop relies on. Effective rate = clockHz × speedMul; tick =
// 1s / rate. The low-end clamp keeps the duration finite when callers pass a
// zero clockHz from a stale snapshot.
func TestTickInterval_Formula(t *testing.T) {
	cases := []struct {
		name string
		hz   int
		mul  float64
		want time.Duration
	}{
		{"1Hz x 1", 1, 1.0, time.Second},
		{"4Hz x 1 default", 4, 1.0, 250 * time.Millisecond},
		{"2Hz x 0.25 plan example", 2, 0.25, 2 * time.Second},
		{"10Hz x 4 plan example", 10, 4.0, 25 * time.Millisecond},
		{"60Hz x 5 upper bound", 60, 5.0, time.Second / 300},
		{"clamps zero to 0.1Hz", 0, 0, 10 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tickInterval(tc.hz, tc.mul)
			if got != tc.want {
				t.Errorf("tickInterval(%d, %v) = %v, want %v", tc.hz, tc.mul, got, tc.want)
			}
		})
	}
}

// TestClampClockHz_Bounds keeps the integer clock inside the UI-advertised
// range [1, 60]. Anything below clamps up to 1 (preventing div-by-zero in the
// ticker formula); anything above clamps down to 60.
func TestClampClockHz_Bounds(t *testing.T) {
	cases := map[int]int{
		-5:  1,
		0:   1,
		1:   1,
		4:   4,
		60:  60,
		61:  60,
		999: 60,
	}
	for in, want := range cases {
		if got := clampClockHz(in); got != want {
			t.Errorf("clampClockHz(%d) = %d, want %d", in, got, want)
		}
	}
}

// TestClampSpeedMul_Bounds mirrors clampClockHz for the float multiplier. The
// 0.1–5.0 range is the contract the SpeedSlider relies on.
func TestClampSpeedMul_Bounds(t *testing.T) {
	cases := map[float64]float64{
		0.0:   0.1,
		0.05:  0.1,
		0.1:   0.1,
		1.0:   1.0,
		2.5:   2.5,
		5.0:   5.0,
		5.5:   5.0,
		100.0: 5.0,
	}
	for in, want := range cases {
		if got := clampSpeedMul(in); got != want {
			t.Errorf("clampSpeedMul(%v) = %v, want %v", in, got, want)
		}
	}
}

// TestSetClockHz_StoresAndClamps verifies the setter both clamps its input and
// persists the value into the struct so the next StartRun picks it up.
func TestSetClockHz_StoresAndClamps(t *testing.T) {
	s := New(Config{NumCores: 1})
	s.SetClockHz(120) // above range → clamps to 60
	if s.clockHz != 60 {
		t.Errorf("after SetClockHz(120) clockHz = %d, want 60", s.clockHz)
	}
	s.SetClockHz(0) // below range → clamps to 1
	if s.clockHz != 1 {
		t.Errorf("after SetClockHz(0) clockHz = %d, want 1", s.clockHz)
	}
}

// TestSetSpeedMul_StoresAndClamps mirrors the clock test for the multiplier.
func TestSetSpeedMul_StoresAndClamps(t *testing.T) {
	s := New(Config{NumCores: 1})
	s.SetSpeedMul(10.0) // above → 5.0
	if s.speedMul != 5.0 {
		t.Errorf("after SetSpeedMul(10) speedMul = %v, want 5.0", s.speedMul)
	}
	s.SetSpeedMul(0.0) // below → 0.1
	if s.speedMul != 0.1 {
		t.Errorf("after SetSpeedMul(0) speedMul = %v, want 0.1", s.speedMul)
	}
}

// TestSetClockHz_PlusSetSpeedMul_PlanExample reproduces the verification case
// from the design doc: clockHz=2 combined with speedMul=0.25 must yield a
// 2-second tick interval.
func TestSetClockHz_PlusSetSpeedMul_PlanExample(t *testing.T) {
	s := New(Config{NumCores: 1})
	s.SetClockHz(2)
	s.SetSpeedMul(0.25)

	got := tickInterval(s.clockHz, s.speedMul)
	want := 2 * time.Second
	if got != want {
		t.Errorf("tickInterval(2, 0.25) = %v, want %v", got, want)
	}
}

// TestSetSpeed_LegacyForwardsToClockHz makes sure the deprecated entry point
// the WS handler still uses keeps working: SetSpeed must mutate clockHz, not
// some orphan field.
func TestSetSpeed_LegacyForwardsToClockHz(t *testing.T) {
	s := New(Config{NumCores: 1})
	s.SetSpeed(30)
	if s.clockHz != 30 {
		t.Errorf("SetSpeed(30) did not update clockHz; got %d", s.clockHz)
	}
}

// TestSnapshot_ExposesSpeedFields ensures clients receive clockHz, speedMul
// and tickIntervalMs in every snapshot — the contract the new frontend store
// relies on.
func TestSnapshot_ExposesSpeedFields(t *testing.T) {
	s := New(Config{NumCores: 1})
	s.SetClockHz(8)
	s.SetSpeedMul(0.5)

	snap := s.Snapshot()

	if got := snap["clockHz"]; got != 8 {
		t.Errorf("snapshot clockHz = %v, want 8", got)
	}
	if got := snap["speedMul"]; got != 0.5 {
		t.Errorf("snapshot speedMul = %v, want 0.5", got)
	}
	if got := snap["tickIntervalMs"]; got != 250 {
		t.Errorf("snapshot tickIntervalMs = %v, want 250", got)
	}
	// Legacy `hz` exposes the effective rate so old clients see something.
	if got := snap["hz"]; got != 4.0 {
		t.Errorf("snapshot hz (effective) = %v, want 4.0", got)
	}
}

// TestSetClockHz_NonBlockingWhenRunLoopChannelIsFull guards against a deadlock
// regression: if the user spams the slider while the run loop is busy, the
// intervalChange channel can already hold a pending value. The setter must
// drop the new send instead of blocking the caller (which would freeze the WS
// handler goroutine).
func TestSetClockHz_NonBlockingWhenRunLoopChannelIsFull(t *testing.T) {
	s := New(Config{NumCores: 1})
	// Simulate an active run loop without actually starting one: install a
	// non-buffered consumer and a full buffered channel so the select default
	// branch is exercised.
	s.running = true
	s.intervalChange = make(chan time.Duration, 1)
	s.intervalChange <- time.Second // fill the buffer

	done := make(chan struct{})
	go func() {
		s.SetClockHz(10)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("SetClockHz blocked on full intervalChange channel")
	}
	// Cleanup so the simulator doesn't carry over fake state.
	s.running = false
	s.intervalChange = nil
}
