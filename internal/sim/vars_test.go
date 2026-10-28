package sim

import (
	"reflect"
	"testing"

	"github.com/jeanmarcos-dev/mod-arch/pkg/cpu"
)

// TestFormatBytes covers the type-aware formatting the VARS panel renders.
// Int values are signed (negative shows as such); uint/ptr stay unsigned;
// char prints both glyph and decimal.
func TestFormatBytes(t *testing.T) {
	cases := []struct {
		name    string
		bytes   []byte
		typ     VarType
		wantHex string
		wantDec string
	}{
		{"int positive", []byte{0x89, 0x00, 0x00, 0x00}, VarInt, "0x00000089", "137"},
		{"int negative", []byte{0xFF, 0xFF, 0xFF, 0xFF}, VarInt, "0xFFFFFFFF", "-1"},
		{"uint", []byte{0xFF, 0xFF, 0xFF, 0xFF}, VarUint, "0xFFFFFFFF", "4294967295"},
		{"ptr", []byte{0x00, 0x00, 0xF0, 0x3F}, VarPtr, "0x3FF00000", "0x3FF00000"},
		{"char ascii", []byte{'A'}, VarChar, "0x41", "'A' (65)"},
		{"char control", []byte{0x07}, VarChar, "0x07", "7"},
		{"short int", []byte{0x01, 0x02}, VarInt, "0x00000201", "513"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hex, dec := formatBytes(tc.bytes, tc.typ)
			if hex != tc.wantHex {
				t.Errorf("hex = %q, want %q", hex, tc.wantHex)
			}
			if dec != tc.wantDec {
				t.Errorf("dec = %q, want %q", dec, tc.wantDec)
			}
		})
	}
}

// TestFuncFor checks the function-by-EIP lookup. The simulator uses this to
// figure out which Func's locals to surface for a thread.
func TestFuncFor(t *testing.T) {
	funcs := []Func{
		{Name: "main", StartOffset: 0, EndOffset: 0x80},
		{Name: "thread_fn", StartOffset: 0x80, EndOffset: 0xB0},
	}
	const codeBase uint32 = 0x200000
	cases := []struct {
		eip      uint32
		wantName string
		wantOK   bool
	}{
		{codeBase, "main", true},                  // entry of main
		{codeBase + 0x40, "main", true},           // middle of main
		{codeBase + 0x80, "thread_fn", true},      // entry of thread_fn (start is inclusive)
		{codeBase + 0xAF, "thread_fn", true},      // last byte of thread_fn
		{codeBase + 0xB0, "", false},              // exactly EndOffset (exclusive)
		{codeBase + 0x200, "", false},             // past program
		{codeBase - 1, "", false},                 // before codeBase
		{0, "", false},                            // EIP zero, codeBase zero → not in
	}
	for _, tc := range cases {
		fn, ok := funcFor(tc.eip, codeBase, funcs)
		if ok != tc.wantOK || fn.Name != tc.wantName {
			t.Errorf("funcFor(0x%X) = %q,%v; want %q,%v", tc.eip, fn.Name, ok, tc.wantName, tc.wantOK)
		}
	}

	// No funcs declared → never in scope.
	if _, ok := funcFor(codeBase, codeBase, nil); ok {
		t.Error("funcFor with empty funcs returned in_scope")
	}
}

// TestVarsLockedShape covers the high-level invariants of the vars payload:
// it appears in the snapshot when a demo declares any metadata, and it
// contains globals + threads under the leader PID.
func TestVarsLockedShape(t *testing.T) {
	const counterAddr uint32 = 0x3F0000
	s := New(Config{
		NumCores: 1,
		Demos: []Demo{{
			Name: "demo_vars",
			Procs: []Proc{{
				Name: "p1",
				// Single HLT body; we only care about the metadata path here.
				Bytes:  []byte{0xF4},
				Source: "// dummy\n",
				Globals: []Global{
					{Name: "counter", Type: VarInt, Addr: counterAddr, Len: 4},
				},
				Funcs: []Func{
					{Name: "main", StartOffset: 0, EndOffset: 1},
				},
				Locals: []Local{
					{
						Name: "v", Type: VarInt,
						Loc:  VarLoc{Kind: VarLocReg, Reg: "EAX"},
						Func: "main",
					},
				},
			}},
		}},
	})

	if err := s.LoadProgram("demo_vars"); err != nil {
		t.Fatalf("LoadProgram: %v", err)
	}

	snap := s.Snapshot()
	varsAny, ok := snap["vars"]
	if !ok {
		t.Fatal("snapshot missing 'vars' field")
	}
	vars, ok := varsAny.(map[string]any)
	if !ok || len(vars) == 0 {
		t.Fatalf("vars = %T (%v); want non-empty map[string]any", varsAny, varsAny)
	}

	var entry map[string]any
	for _, e := range vars {
		entry = e.(map[string]any)
		break
	}
	globals := entry["globals"].([]map[string]any)
	if len(globals) != 1 || globals[0]["name"] != "counter" {
		t.Fatalf("globals = %+v", globals)
	}
	if globals[0]["addr"] != "0x3F0000" {
		t.Errorf("global addr = %v, want 0x3F0000", globals[0]["addr"])
	}

	threads := entry["threads"].([]map[string]any)
	if len(threads) == 0 {
		t.Fatal("threads empty; want at least the leader thread")
	}
}

// TestVarsLockedAbsentWhenNoMetadata covers the negative path: a demo with no
// declared variables produces a nil vars field (the WS layer drops nil).
func TestVarsLockedAbsentWhenNoMetadata(t *testing.T) {
	s := New(Config{
		NumCores: 1,
		Demos: []Demo{{
			Name:  "no_vars",
			Procs: []Proc{{Name: "p1", Bytes: []byte{0xF4}}},
		}},
	})
	if err := s.LoadProgram("no_vars"); err != nil {
		t.Fatalf("LoadProgram: %v", err)
	}
	snap := s.Snapshot()
	if v, ok := snap["vars"]; ok && !reflect.ValueOf(v).IsNil() {
		t.Errorf("snapshot.vars = %v; want nil for a metadata-less demo", v)
	}
}

// TestVarsLockedReadsLiveRegistersForRunningThread reproduces the "VARS shows
// 0 for a/b after stepping ADD" bug: after MOV EAX, 10 the snapshot must
// resolve the local `a → EAX` to 10, not to the stale 0 the PCB held before
// dispatch. Catches regressions in threadRegsLocked picking the wrong source
// (PCB.Regs vs live core CPU R) for a RUN proc.
func TestVarsLockedReadsLiveRegistersForRunningThread(t *testing.T) {
	bytes := []byte{
		0xB8, 0x0A, 0x00, 0x00, 0x00, // MOV EAX, 10
		0xBB, 0x20, 0x00, 0x00, 0x00, // MOV EBX, 32
		0x01, 0xD8,                   // ADD EAX, EBX
		0x89, 0xC3,                   // MOV EBX, EAX
		0xB8, 0x01, 0x00, 0x00, 0x00, // MOV EAX, 1 (sys_exit)
		0xCD, 0x80,                   // INT 0x80
	}
	s := New(Config{
		NumCores: 1,
		Demos: []Demo{{
			Name: "add_repro",
			Procs: []Proc{{
				Name:   "add",
				Bytes:  bytes,
				Source: "int main(void) {\n    register int a = 10;\n    register int b = 32;\n    return a + b;\n}\n",
				SourceMap: []SourceLine{
					{Offset: 0, Mnemonic: "MOV EAX, 0xA", CLine: 2},
					{Offset: 5, Mnemonic: "MOV EBX, 0x20", CLine: 3},
					{Offset: 10, Mnemonic: "ADD EAX, EBX", CLine: 4},
				},
				Funcs: []Func{{Name: "main", StartOffset: 0, EndOffset: uint32(len(bytes))}},
				Locals: []Local{
					{Name: "a", Type: VarInt, Loc: VarLoc{Kind: VarLocReg, Reg: "EAX"}, Func: "main"},
					{Name: "b", Type: VarInt, Loc: VarLoc{Kind: VarLocReg, Reg: "EBX"}, Func: "main"},
				},
			}},
		}},
	})
	if err := s.LoadProgram("add_repro"); err != nil {
		t.Fatalf("LoadProgram: %v", err)
	}
	// Step through MOV EAX, 10 + MOV EBX, 32. Two steps land us right before
	// ADD EAX, EBX so both regs still hold the literal init values the
	// student declared in the C source.
	for i := 0; i < 2; i++ {
		if err := s.StepOnce(); err != nil {
			t.Fatalf("StepOnce %d: %v", i, err)
		}
	}

	snap := s.Snapshot()
	varsAny, ok := snap["vars"].(map[string]any)
	if !ok || len(varsAny) == 0 {
		t.Fatalf("snapshot.vars = %v; want non-empty map", snap["vars"])
	}
	var entry map[string]any
	for _, v := range varsAny {
		entry = v.(map[string]any)
	}
	threads := entry["threads"].([]map[string]any)
	if len(threads) != 1 {
		t.Fatalf("threads = %d, want 1", len(threads))
	}
	th := threads[0]
	// Bug it catches: label = "T4" for a single-PCB demo. Plain processes
	// must use "P<pid>" so the panel doesn't claim a thread group exists.
	if label, _ := th["label"].(string); label != "" && label[0] != 'P' {
		t.Errorf("label = %q; want P-prefixed for single-process demo", label)
	}
	if inScope, _ := th["in_scope"].(bool); !inScope {
		t.Fatalf("in_scope = false; EIP should be inside main() after steps. entry=%+v", th)
	}
	locals := th["locals"].([]map[string]any)
	got := map[string]string{}
	for _, l := range locals {
		got[l["name"].(string)], _ = l["value_dec"].(string)
	}
	if got["a"] != "10" {
		t.Errorf("a value_dec = %q, want 10 (full entry: %+v)", got["a"], locals)
	}
	if got["b"] != "32" {
		t.Errorf("b value_dec = %q, want 32 (full entry: %+v)", got["b"], locals)
	}
}

// TestVarsLockedTracksStepByStep walks the ADD demo bytes step by step and
// captures the snapshot after each StepOnce so we can verify the panel sees
// the right (EAX, EBX) pair as the program advances. Reproduces the "VARS
// is one step behind the cursor" bug if it exists in the backend; if all
// steps line up here, the discrepancy must be in the frontend's event
// ordering.
func TestVarsLockedTracksStepByStep(t *testing.T) {
	bytes := []byte{
		0xB8, 0x0A, 0x00, 0x00, 0x00, // 0x00  MOV EAX, 10
		0xBB, 0x20, 0x00, 0x00, 0x00, // 0x05  MOV EBX, 32
		0x01, 0xD8,                   // 0x0A  ADD EAX, EBX  → EAX=42
		0x89, 0xC3,                   // 0x0C  MOV EBX, EAX  → EBX=42
		0xB8, 0x01, 0x00, 0x00, 0x00, // 0x0E  MOV EAX, 1
		0xCD, 0x80,                   // 0x13  INT 0x80
	}
	s := New(Config{
		NumCores: 1,
		Demos: []Demo{{
			Name: "add_trace",
			Procs: []Proc{{
				Name:   "add",
				Bytes:  bytes,
				Source: "int main(void) {\n    register int a = 10;\n    register int b = 32;\n    return a + b;\n}\n",
				SourceMap: []SourceLine{
					{Offset: 0, Mnemonic: "MOV EAX, 0xA", CLine: 2},
					{Offset: 5, Mnemonic: "MOV EBX, 0x20", CLine: 3},
				},
				Funcs: []Func{{Name: "main", StartOffset: 0, EndOffset: uint32(len(bytes))}},
				Locals: []Local{
					{Name: "a", Type: VarInt, Loc: VarLoc{Kind: VarLocReg, Reg: "EAX"}, Func: "main"},
					{Name: "b", Type: VarInt, Loc: VarLoc{Kind: VarLocReg, Reg: "EBX"}, Func: "main"},
				},
			}},
		}},
	})
	if err := s.LoadProgram("add_trace"); err != nil {
		t.Fatalf("LoadProgram: %v", err)
	}

	type expect struct{ a, b string }
	want := []expect{
		// After step 1: MOV EAX, 10 executed. EIP advances to 0x05.
		{"10", "0"},
		// After step 2: MOV EBX, 32 executed.
		{"10", "32"},
		// After step 3: ADD EAX, EBX executed → EAX = 42.
		{"42", "32"},
		// After step 4: MOV EBX, EAX executed → EBX = 42.
		{"42", "42"},
		// After step 5: MOV EAX, 1 executed (preparing sys_exit).
		{"1", "42"},
	}

	for i, exp := range want {
		if err := s.StepOnce(); err != nil {
			t.Fatalf("StepOnce %d: %v", i+1, err)
		}
		snap := s.Snapshot()
		varsAny, ok := snap["vars"].(map[string]any)
		if !ok || len(varsAny) == 0 {
			t.Fatalf("step %d: vars missing or empty", i+1)
		}
		var entry map[string]any
		for _, v := range varsAny {
			entry = v.(map[string]any)
		}
		threads := entry["threads"].([]map[string]any)
		th := threads[0]
		got := map[string]string{}
		for _, l := range th["locals"].([]map[string]any) {
			got[l["name"].(string)], _ = l["value_dec"].(string)
		}
		if got["a"] != exp.a || got["b"] != exp.b {
			t.Errorf("after step %d: a=%q b=%q; want a=%q b=%q (EIP=0x%X)",
				i+1, got["a"], got["b"], exp.a, exp.b,
				uint32(th["regs"].(map[string]uint32)["EIP"]))
		}
	}
}

// TestVarsUpdateBroadcastInOrder pins the *order* and *content* of the
// frames StepOnce emits, since the visual lag the user reported looks like
// the WS frame for vars carries stale data. We register a fake client,
// drive a step, and assert that the vars_update frame on the wire carries
// the post-step register values.
func TestVarsUpdateBroadcastInOrder(t *testing.T) {
	bytes := []byte{
		0xB8, 0x0A, 0x00, 0x00, 0x00, // MOV EAX, 10
		0xBB, 0x20, 0x00, 0x00, 0x00, // MOV EBX, 32
	}
	s := New(Config{
		NumCores: 1,
		Demos: []Demo{{
			Name: "add_wire",
			Procs: []Proc{{
				Name:  "add",
				Bytes: bytes,
				Source: "int main(void) {\n    register int a = 10;\n    register int b = 32;\n    return a + b;\n}\n",
				SourceMap: []SourceLine{
					{Offset: 0, Mnemonic: "MOV EAX, 0xA", CLine: 2},
					{Offset: 5, Mnemonic: "MOV EBX, 0x20", CLine: 3},
				},
				Funcs: []Func{{Name: "main", StartOffset: 0, EndOffset: uint32(len(bytes))}},
				Locals: []Local{
					{Name: "a", Type: VarInt, Loc: VarLoc{Kind: VarLocReg, Reg: "EAX"}, Func: "main"},
					{Name: "b", Type: VarInt, Loc: VarLoc{Kind: VarLocReg, Reg: "EBX"}, Func: "main"},
				},
			}},
		}},
	})
	if err := s.LoadProgram("add_wire"); err != nil {
		t.Fatalf("LoadProgram: %v", err)
	}

	// Attach a generous-buffer client AFTER LoadProgram so its load-time
	// snapshot/vars_update do not crowd the channel for the step assertions.
	cli := NewClient(RoleOwner)
	cli.Send = make(chan any, 1024)
	s.clientMu.Lock()
	s.clients[cli] = struct{}{}
	s.clientMu.Unlock()

	if err := s.StepOnce(); err != nil {
		t.Fatalf("StepOnce: %v", err)
	}

	// Drain the channel briefly and find the tick_events frame carrying
	// the vars piggyback. The whole point of the refactor that introduced
	// this test was making vars ride the same atomic frame as the events
	// instead of a separate message that the Send buffer could drop while
	// keeping the events.
	var varsFrame TickEventsEvent
	varsFrame.Tick = 99999
	for done := false; !done; {
		select {
		case msg := <-cli.Send:
			if ev, ok := msg.(TickEventsEvent); ok && ev.Vars != nil {
				varsFrame = ev
			}
		default:
			done = true
		}
	}
	if varsFrame.Tick == 99999 {
		t.Fatal("no tick_events frame with vars piggyback received after StepOnce")
	}
	vars := varsFrame.Vars
	var entry map[string]any
	for _, v := range vars {
		entry = v.(map[string]any)
	}
	threads := entry["threads"].([]map[string]any)
	got := map[string]string{}
	for _, l := range threads[0]["locals"].([]map[string]any) {
		got[l["name"].(string)], _ = l["value_dec"].(string)
	}
	// After step 1 the only executed instruction is MOV EAX, 10. The wire
	// must carry a=10, b=0 — anything else (a=0 in particular) means the
	// broadcast captured a stale snapshot.
	if got["a"] != "10" {
		t.Errorf("vars_update on wire: a=%q, want 10 (full payload: %+v)", got["a"], threads[0])
	}
	if got["b"] != "0" {
		t.Errorf("vars_update on wire: b=%q, want 0 (full payload: %+v)", got["b"], threads[0])
	}
}

// TestReadRegByName ensures every register name the DSL accepts via LocalReg
// is resolvable. Adding a new register requires updating both ends.
func TestReadRegByName(t *testing.T) {
	r := cpu.Registers{EAX: 1, EBX: 2, ECX: 3, EDX: 4, ESI: 5, EDI: 6, EBP: 7, ESP: 8}
	for _, name := range []string{"EAX", "EBX", "ECX", "EDX", "ESI", "EDI", "EBP", "ESP"} {
		v, ok := readRegByName(r, name)
		if !ok {
			t.Errorf("readRegByName(%q) returned !ok", name)
		}
		if v == 0 {
			t.Errorf("readRegByName(%q) = 0; want stub value", name)
		}
	}
	if _, ok := readRegByName(r, "ZZZ"); ok {
		t.Error("unknown register name was accepted")
	}
}
