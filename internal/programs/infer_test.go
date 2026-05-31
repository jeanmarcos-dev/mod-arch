package programs

import "testing"

// TestInferFuncs pins the function-detection heuristic against a minimal C
// source. Each header should generate one Func whose EndOffset closes either
// at the next header or at totalBytes.
func TestInferFuncs(t *testing.T) {
	src := `// header comment
int main(void) {
    return 0;
}

void thread_fn(void) {
    exit(0);
}
`
	sm := []SourceLine{
		{Offset: 0x00, Mnemonic: "MOV EAX, 0x0", CLine: 3},
		{Offset: 0x05, Mnemonic: "INT 0x80", CLine: 3},
		{Offset: 0x10, Mnemonic: "MOV EAX, 0x1", CLine: 7},
		{Offset: 0x15, Mnemonic: "INT 0x80", CLine: 7},
	}
	funcs := inferFuncs(splitSourceForTest(src), sm, 0x20)

	if len(funcs) != 2 {
		t.Fatalf("len(funcs) = %d, want 2 (%+v)", len(funcs), funcs)
	}
	if funcs[0].Name != "main" || funcs[0].StartOffset != 0x00 || funcs[0].EndOffset != 0x10 {
		t.Errorf("main = %+v, want {main 0x00 0x10}", funcs[0])
	}
	if funcs[1].Name != "thread_fn" || funcs[1].StartOffset != 0x10 || funcs[1].EndOffset != 0x20 {
		t.Errorf("thread_fn = %+v, want {thread_fn 0x10 0x20}", funcs[1])
	}
}

// TestInferGlobalsPtrLiteral covers the only global form the parser
// supports for now: `volatile int *<name> = (int *)0xADDR;` at file scope.
func TestInferGlobalsPtrLiteral(t *testing.T) {
	src := `volatile int *counter = (int *)0x3F0000;

int main(void) {
    return 0;
}
`
	g := inferGlobals(splitSourceForTest(src), nil)
	if len(g) != 1 {
		t.Fatalf("len(g) = %d, want 1 (%+v)", len(g), g)
	}
	want := Global{Name: "counter", Type: VarInt, Addr: 0x3F0000, Len: 4}
	if g[0] != want {
		t.Errorf("g[0] = %+v, want %+v", g[0], want)
	}
}

// TestInferGlobalsSkippedInsideFunc — the same pointer declaration written
// inside a function body must NOT be picked up as global, because it is a
// local with file-static semantics that we treat differently.
func TestInferGlobalsSkippedInsideFunc(t *testing.T) {
	src := `int main(void) {
    volatile int *p = (int *)0x100000;
    return 0;
}
`
	g := inferGlobals(splitSourceForTest(src), nil)
	if len(g) != 0 {
		t.Errorf("got %d globals from in-func declaration, want 0 (%+v)", len(g), g)
	}
}

// TestInferLocalsAdd — the canonical `add` demo: two register declarations
// resolve to the registers their first MOV writes.
func TestInferLocalsAdd(t *testing.T) {
	src := `int main(void) {
    register int a = 10;
    register int b = 32;
    return a + b;
}
`
	sm := []SourceLine{
		{Offset: 0x00, Mnemonic: "MOV EAX, 0xA", CLine: 2},
		{Offset: 0x05, Mnemonic: "MOV EBX, 0x20", CLine: 3},
		{Offset: 0x0A, Mnemonic: "ADD EAX, EBX", CLine: 4},
	}
	lines := splitSourceForTest(src)
	funcs := inferFuncs(lines, sm, 0x10)
	locals := inferLocals(lines, funcs, sm, nil)
	if len(locals) != 2 {
		t.Fatalf("len(locals) = %d, want 2 (%+v)", len(locals), locals)
	}
	wantA := Local{Name: "a", Type: VarInt, Loc: VarLoc{Kind: VarLocReg, Reg: "EAX"}, Func: "main"}
	wantB := Local{Name: "b", Type: VarInt, Loc: VarLoc{Kind: VarLocReg, Reg: "EBX"}, Func: "main"}
	if locals[0] != wantA {
		t.Errorf("locals[0] = %+v, want %+v", locals[0], wantA)
	}
	if locals[1] != wantB {
		t.Errorf("locals[1] = %+v, want %+v", locals[1], wantB)
	}
}

// TestInferLocalsSkipsFunctionCall — `int t1 = clone(...)` reads a syscall
// return into EAX. The author rarely cares about t1 (EAX gets clobbered by
// the next syscall number); skipping it avoids surfacing a meaningless
// "T5 t1 = 78" right after the call.
func TestInferLocalsSkipsFunctionCall(t *testing.T) {
	src := `int main(void) {
    int t1 = clone(0x10500, 0, thread_fn);
    return 0;
}
`
	sm := []SourceLine{
		{Offset: 0x00, Mnemonic: "MOV EAX, 0x78", CLine: 2},
		{Offset: 0x05, Mnemonic: "INT 0x80", CLine: 2},
	}
	lines := splitSourceForTest(src)
	funcs := inferFuncs(lines, sm, 0x10)
	locals := inferLocals(lines, funcs, sm, nil)
	if len(locals) != 0 {
		t.Errorf("got %d locals, want 0 (%+v)", len(locals), locals)
	}
}

// TestInferLocalsGlobalNameTakesPrecedence — a name claimed by a global at
// file scope (e.g. `counter`) is not re-emitted as a local even if it
// appears inside a function declaration like `int counter = 0;`.
func TestInferLocalsGlobalNameTakesPrecedence(t *testing.T) {
	src := `volatile int *counter = (int *)0x3F0000;

int main(void) {
    int counter = 0;
    return 0;
}
`
	sm := []SourceLine{{Offset: 0x00, Mnemonic: "MOV EAX, 0x0", CLine: 4}}
	lines := splitSourceForTest(src)
	funcs := inferFuncs(lines, sm, 0x10)
	used := map[string]bool{"counter": true}
	locals := inferLocals(lines, funcs, sm, used)
	if len(locals) != 0 {
		t.Errorf("locals = %+v, want empty (global name shadowed local detection)", locals)
	}
}

// TestInferVarsThreadRaceShape stitches the three passes together against
// a thread_race-flavoured source and SourceMap and verifies the panel
// would receive (counter global, main+thread_fn funcs, v+i locals in
// thread_fn). main intentionally has no locals because all its
// declarations are clone() results.
func TestInferVarsThreadRaceShape(t *testing.T) {
	src := `// race
volatile int *counter = (int *)0x3F0000;

int main(void) {
    *counter = 0;
    int t1 = clone(0x10500, 0, thread_fn);
    int t2 = clone(0x10500, 0, thread_fn);
    return *counter;
}

void thread_fn(void) {
    for (int i = 0; i < 200; i++) {
        int v = *counter;
        v = v + 1;
        *counter = v;
    }
    exit(0);
}
`
	// SourceMap fragments enough for the heuristics: a MOV reg, imm on
	// the loop-counter line (line 12) and a MOV EAX, [addr] on the
	// load-counter line (line 13).
	sm := []SourceLine{
		{Offset: 0x00, Mnemonic: "MOV EAX, 0x0", CLine: 5},
		{Offset: 0x05, Mnemonic: "MOV EAX, 0x78", CLine: 6}, // clone() — suppressed
		{Offset: 0x0A, Mnemonic: "INT 0x80", CLine: 6},
		{Offset: 0x0F, Mnemonic: "MOV EAX, 0x78", CLine: 7}, // clone() — suppressed
		{Offset: 0x14, Mnemonic: "INT 0x80", CLine: 7},
		{Offset: 0x80, Mnemonic: "MOV EBX, 0x0", CLine: 12}, // i = 0
		{Offset: 0x85, Mnemonic: "MOV EAX, [0x3F0000]", CLine: 13},
	}
	g, l, fs := InferVars(src, sm, 0x200)

	if len(g) != 1 || g[0].Name != "counter" || g[0].Addr != 0x3F0000 {
		t.Errorf("globals = %+v", g)
	}
	if len(fs) != 2 || fs[0].Name != "main" || fs[1].Name != "thread_fn" {
		t.Errorf("funcs = %+v", fs)
	}
	byName := map[string]Local{}
	for _, lo := range l {
		byName[lo.Name] = lo
	}
	if v, ok := byName["v"]; !ok || v.Loc.Reg != "EAX" || v.Func != "thread_fn" {
		t.Errorf("v = %+v (want EAX in thread_fn)", v)
	}
	if i, ok := byName["i"]; !ok || i.Loc.Reg != "EBX" || i.Func != "thread_fn" {
		t.Errorf("i = %+v (want EBX in thread_fn)", i)
	}
	if _, ok := byName["t1"]; ok {
		t.Errorf("t1 leaked into locals; clone() initialiser should suppress it (got %+v)", byName["t1"])
	}
}

// TestInferLocalsRejectsLiteralMismatch — when the C says `int i = 0` but the
// bytecode seeds the register with a different literal (typical down-counting
// loop optimisation: `MOV ECX, 200` for `for (i = 0; i < 200; i++)`), the
// register does NOT actually hold the variable, it holds "iterations left".
// The inferenciador must skip rather than show ECX as `i = 200`.
func TestInferLocalsRejectsLiteralMismatch(t *testing.T) {
	src := `int main(void) {
    for (int i = 0; i < 200; i++) {
        i++;
    }
    return 0;
}
`
	sm := []SourceLine{
		{Offset: 0x00, Mnemonic: "MOV ECX, 0xC8", CLine: 2}, // down-counter
		{Offset: 0x05, Mnemonic: "DEC ECX", CLine: 3},
		{Offset: 0x06, Mnemonic: "JNE 0x05", CLine: 3},
	}
	lines := splitSourceForTest(src)
	funcs := inferFuncs(lines, sm, 0x10)
	locals := inferLocals(lines, funcs, sm, nil)
	for _, l := range locals {
		if l.Name == "i" {
			t.Errorf("inferred i = %+v; want it to be skipped because MOV ECX, 200 doesn't match `i = 0`", l)
		}
	}
}

// TestInferLocalsAcceptsLoadInitialiser — `int n = *p;` legitimately compiles
// to a memory load. The inferenciador must accept `MOV <reg>, [addr]` as a
// valid binding even though there is no literal to compare against.
func TestInferLocalsAcceptsLoadInitialiser(t *testing.T) {
	src := `int main(void) {
    int n = *(int *)0x3FFFE0;
    return n;
}
`
	sm := []SourceLine{
		{Offset: 0x00, Mnemonic: "MOV EAX, [0x3FFFE0]", CLine: 2},
	}
	lines := splitSourceForTest(src)
	funcs := inferFuncs(lines, sm, 0x10)
	locals := inferLocals(lines, funcs, sm, nil)
	if len(locals) != 1 || locals[0].Name != "n" || locals[0].Loc.Reg != "EAX" {
		t.Errorf("locals = %+v; want one entry n → EAX", locals)
	}
}

// TestParseInitExpr exercises the small parser that classifies the right-hand
// side of a declaration into "literal", "load" or "other". Pinning the cases
// here makes regressions in the regex obvious without re-running the whole
// inference pipeline.
func TestParseInitExpr(t *testing.T) {
	cases := []struct {
		rhs      string
		wantLit  *int64
		wantLoad bool
		wantOK   bool
	}{
		{"= 10;", ptrInt64(10), false, true},
		{"= 0x3F0000;", ptrInt64(0x3F0000), false, true},
		{"= -5;", ptrInt64(-5), false, true},
		{"= *counter;", nil, true, true},
		{"= *(int *)0x3FFFE0;", nil, true, true},
		{"= a + b;", nil, false, true},
		{";", nil, false, false},
	}
	for _, tc := range cases {
		got, ok := parseInitExpr(tc.rhs)
		if ok != tc.wantOK {
			t.Errorf("parseInitExpr(%q) ok = %v, want %v", tc.rhs, ok, tc.wantOK)
			continue
		}
		gotLit := got.literal
		if (gotLit == nil) != (tc.wantLit == nil) || (gotLit != nil && *gotLit != *tc.wantLit) {
			t.Errorf("parseInitExpr(%q) literal = %v, want %v", tc.rhs, gotLit, tc.wantLit)
		}
		if got.isLoad != tc.wantLoad {
			t.Errorf("parseInitExpr(%q) isLoad = %v, want %v", tc.rhs, got.isLoad, tc.wantLoad)
		}
	}
}

func ptrInt64(v int64) *int64 { return &v }

// TestHasCallOnRHSStripsCast — make sure `(int *)0x100000` does not register
// as a function call. False positives on casts would suppress every global
// with a cast literal.
func TestHasCallOnRHSStripsCast(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{`= 10;`, false},
		{`= (int *)0x3F0000;`, false},
		{`= *counter;`, false},
		{`= clone(0x10500, 0, thread_fn);`, true},
		{`= read(0, &buf, 1);`, true},
		{`= a + b;`, false},
	}
	for _, tc := range cases {
		got := hasCallOnRHS(tc.s)
		if got != tc.want {
			t.Errorf("hasCallOnRHS(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}

// splitSourceForTest mirrors strings.Split(s, "\n") so test code can build a
// []string out of a literal and feed it into the helpers without each test
// repeating the import.
func splitSourceForTest(s string) []string {
	// Hand-rolled split; we avoid pulling strings here to keep imports tight.
	out := []string{""}
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, "")
			continue
		}
		out[len(out)-1] += string(s[i])
	}
	return out
}
