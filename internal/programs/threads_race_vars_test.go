package programs

import "testing"

// TestThreadsRaceDeclaresVariableMetadata pins the variable metadata the demo
// builder attaches to thread_race. The frontend's VARS panel and identifier
// popover depend on these names/locations being present; a refactor of the
// builder that silently drops them would degrade the panel to "no metadata".
func TestThreadsRaceDeclaresVariableMetadata(t *testing.T) {
	demos := Demos()
	var race *Demo
	for i := range demos {
		if demos[i].Name == "threads_race" {
			race = &demos[i]
			break
		}
	}
	if race == nil {
		t.Fatal("threads_race demo missing from catalogue")
	}
	if len(race.Procs) != 1 {
		t.Fatalf("threads_race has %d procs, want 1", len(race.Procs))
	}
	p := race.Procs[0]

	if len(p.Globals) != 1 || p.Globals[0].Name != "counter" {
		t.Fatalf("expected one global named counter, got %+v", p.Globals)
	}
	if p.Globals[0].Addr != ThreadCounterAddr() {
		t.Errorf("counter addr = 0x%X, want 0x%X", p.Globals[0].Addr, ThreadCounterAddr())
	}

	gotFuncs := map[string]Func{}
	for _, f := range p.Funcs {
		gotFuncs[f.Name] = f
	}
	main, ok := gotFuncs["main"]
	if !ok || main.StartOffset != 0 || main.EndOffset == 0 {
		t.Errorf("main func = %+v (need StartOffset=0 and non-zero EndOffset)", main)
	}
	threadFn, ok := gotFuncs["thread_fn"]
	if !ok || threadFn.StartOffset != 0x80 || threadFn.EndOffset <= threadFn.StartOffset {
		t.Errorf("thread_fn func = %+v (need StartOffset=0x80, EndOffset > StartOffset)", threadFn)
	}

	wantLocals := map[string]string{"v": "EAX", "i": "EBX"}
	got := map[string]string{}
	for _, l := range p.Locals {
		if l.Func != "thread_fn" {
			continue
		}
		if l.Loc.Kind != VarLocReg {
			t.Errorf("local %q kind = %v, want VarLocReg", l.Name, l.Loc.Kind)
		}
		got[l.Name] = l.Loc.Reg
	}
	for name, reg := range wantLocals {
		if got[name] != reg {
			t.Errorf("local %q reg = %q, want %q", name, got[name], reg)
		}
	}
}
