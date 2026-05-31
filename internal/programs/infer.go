package programs

import (
	"regexp"
	"strconv"
	"strings"
)

// InferVars reads a Proc's C source plus its SourceMap and produces the
// variable metadata the VARS panel renders. Best-effort: anything that
// cannot be mapped confidently is dropped silently rather than guessed.
//
// Why this exists: the simulator has no real compiler. The C source and the
// machine bytes are two parallel transcriptions kept consistent by the demo
// author. To avoid forcing each demo to duplicate variable declarations in
// Go (".Global(name, type, addr)" etc.), we extract them from the source
// itself and pin each one to a runtime location by walking the SourceMap.
//
// Heuristics covered:
//
//   - Functions: `<retType> <name>(...)` at column zero or with leading
//     whitespace. The body range is [first SourceLine with CLine >= header,
//     start of next function or totalBytes).
//   - Globals: `(?:volatile )?(int|char) *<name> = (?:(<type> *))?0xADDR`
//     declared outside any function. Address is parsed from the literal.
//   - Locals: `(?:register|volatile)? (int|char) <name> [= ...]` inside a
//     function body. The location is the destination register of the FIRST
//     `MOV <REG>, ...` instruction emitted on the same CLine — that is the
//     register the author moved the init value into. Lines whose
//     right-hand side calls a function (clone(), fork(), read(), …) are
//     skipped because the return value lives in EAX only momentarily.
//
// Types beyond `int` and `char` and storage classes beyond register-based
// locals are intentionally out of scope for the first pass. Demos that need
// them will surface as gaps in the VARS panel, signalling a missing case
// to extend here.
func InferVars(source string, sm []SourceLine, totalBytes uint32) (globals []Global, locals []Local, funcs []Func) {
	lines := strings.Split(source, "\n")

	funcs = inferFuncs(lines, sm, totalBytes)
	globals = inferGlobals(lines, funcs)

	// Names already claimed as globals are excluded from local detection so
	// `volatile int *counter` at file scope does not also surface as a local
	// of the first function.
	usedGlobal := map[string]bool{}
	for _, g := range globals {
		usedGlobal[g.Name] = true
	}
	locals = inferLocals(lines, funcs, sm, usedGlobal)

	return
}

// ─── Regexes ──────────────────────────────────────────────────────────────

// funcDeclRe matches lines like `int main(void) {`, `void thread_fn(void) {`.
// Storage-class keywords (`static`, `inline`) are accepted but the C source
// of the demos rarely uses them.
var funcDeclRe = regexp.MustCompile(`^\s*(?:static\s+|inline\s+)*(?:void|int|char|long|short|unsigned\s+\w+)\s+(\w+)\s*\(`)

// globalPtrRe matches a file-scope pointer-with-init like
// `volatile int *counter = (int *)0x3F0000;`. The capture order is
// (1) type, (2) name, (3) numeric literal.
var globalPtrRe = regexp.MustCompile(`(?:^|\s)(?:volatile\s+)?(int|char|long|short)\s*\*\s*(\w+)\s*=\s*(?:\(\s*\w+\s*\*\s*\))?\s*(0x[0-9A-Fa-f]+|\d+)`)

// localDeclRe matches local declarations like `register int v`, `int t1 = ...`,
// `int i = 0`, `int v = *counter;`. Pointer locals (`int *p`) are out of scope
// for the first pass.
var localDeclRe = regexp.MustCompile(`\b(register\s+|volatile\s+)?(int|char|long|short)\s+(\w+)\s*(?:[=;,)])`)

// movDstRe captures the destination register of a `MOV <REG>, …` mnemonic.
// The DSL builder always uppercases register names so we match the canonical
// set without case folding.
var movDstRe = regexp.MustCompile(`^MOV\s+(EAX|EBX|ECX|EDX|ESI|EDI|EBP|ESP)\s*,`)

// movImmRe captures the destination register AND the immediate of a
// `MOV <REG>, <imm>` mnemonic. Used to validate that a C initialiser like
// `int a = 10` lines up with the actual value loaded into the register,
// rejecting cases where the bytecode loads something unrelated (e.g. a
// down-counting loop seeds ECX with 200 even though the C says `i = 0`).
// The trailing `(?:\s*;.*)?` keeps the regex tolerant of the DSL's optional
// inline comment (e.g. `MOV EBX, 0x0 ; i = 0`).
var movImmRe = regexp.MustCompile(`^MOV\s+(EAX|EBX|ECX|EDX|ESI|EDI|EBP|ESP)\s*,\s*(0x[0-9A-Fa-f]+|-?\d+)\s*(?:;.*)?$`)

// movLoadRe matches a memory-load form `MOV <REG>, [...]`. A C initialiser
// like `int n = *p` legitimately compiles to a load, so this pattern
// authorises the inference even when the immediate validation fails.
var movLoadRe = regexp.MustCompile(`^MOV\s+(EAX|EBX|ECX|EDX|ESI|EDI|EBP|ESP)\s*,\s*\[`)

// callRe catches a likely C function call on the RHS of an initialiser:
// `clone(...)`, `read(...)`, etc. We use it to suppress locals whose value
// only sits in EAX for one instruction.
var callRe = regexp.MustCompile(`\b\w+\s*\(`)

// ─── Helpers ──────────────────────────────────────────────────────────────

// inferFuncs walks the C source line by line, picks up function headers and
// resolves their byte range against the SourceMap.
func inferFuncs(lines []string, sm []SourceLine, totalBytes uint32) []Func {
	type header struct {
		name  string
		cLine int
	}
	var headers []header
	for i, line := range lines {
		// Skip function-pointer declarations: only "real" C declarations
		// start with a type keyword and are followed by `(`. funcDeclRe
		// already enforces that pattern.
		m := funcDeclRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		headers = append(headers, header{name: m[1], cLine: i + 1})
	}
	if len(headers) == 0 {
		return nil
	}

	out := make([]Func, 0, len(headers))
	for i, h := range headers {
		start := firstOffsetAtOrAfter(sm, h.cLine)
		end := totalBytes
		if i+1 < len(headers) {
			next := firstOffsetAtOrAfter(sm, headers[i+1].cLine)
			if next > start {
				end = next
			}
		}
		out = append(out, Func{Name: h.name, StartOffset: start, EndOffset: end})
	}
	return out
}

// firstOffsetAtOrAfter returns the byte offset of the first instruction whose
// CLine is >= the given line. Used to pin a function header to its bytecode.
func firstOffsetAtOrAfter(sm []SourceLine, cLine int) uint32 {
	for _, sl := range sm {
		if sl.CLine >= cLine {
			return sl.Offset
		}
	}
	return 0
}

// inferGlobals scans every line that does NOT fall inside any inferred
// function for pointer-with-init declarations of the form
// `volatile int *counter = (int *)0x3F0000;` and emits one Global per hit.
// The function list is unused at the moment but kept in the signature so
// the helper can grow scope-awareness later without changing callers.
func inferGlobals(lines []string, _ []Func) []Global {
	// Build a quick "is this line inside a function" predicate. Since we
	// only have offsets (not lines) for function ranges, the heuristic is:
	// a line is "inside" a function if there is a header at or before it
	// and no closing brace at column zero has appeared in between. Brace
	// counting is overkill for the simple demos in scope; we use a coarse
	// approximation that treats `}` at column 0 as the end of any enclosing
	// scope. False positives only hide globals — they never introduce
	// phantom ones — which is the safe direction.
	inFunc := make([]bool, len(lines))
	depth := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		// A closing brace at depth 1 returns us to file scope.
		if depth > 0 && (strings.HasPrefix(trimmed, "}") || trimmed == "}") {
			depth--
		}
		inFunc[i] = depth > 0
		// Function header opens a new scope on the same line.
		if funcDeclRe.MatchString(line) {
			depth++
			inFunc[i] = true
			continue
		}
		// Track stray braces (e.g. nested blocks) so we balance correctly.
		opens := strings.Count(line, "{")
		closes := strings.Count(line, "}")
		// Skip the open we already counted via the header detection.
		if depth > 0 {
			depth += opens - closes
			if depth < 0 {
				depth = 0
			}
		}
	}

	var out []Global
	for i, line := range lines {
		if inFunc[i] {
			continue
		}
		m := globalPtrRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		typ := normaliseType(m[1])
		name := m[2]
		addr, ok := parseUint(m[3])
		if !ok {
			continue
		}
		out = append(out, Global{
			Name: name,
			Type: typ,
			Addr: uint32(addr),
			Len:  sizeOfType(typ),
		})
	}
	return out
}

// inferLocals scans each function's source lines for local declarations and
// uses the SourceMap to pin each name to a register.
func inferLocals(lines []string, funcs []Func, sm []SourceLine, usedGlobal map[string]bool) []Local {
	if len(funcs) == 0 || len(lines) == 0 {
		return nil
	}

	// Resolve function name → C-line range by walking the headers again.
	// Cheaper than mixing line/byte spaces inside the byte-range Func.
	type lineRange struct {
		name        string
		startCLine  int
		endCLineExc int // exclusive
	}
	var ranges []lineRange
	for i, line := range lines {
		m := funcDeclRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		ranges = append(ranges, lineRange{name: m[1], startCLine: i + 1, endCLineExc: len(lines) + 1})
	}
	for i := range ranges {
		if i+1 < len(ranges) {
			ranges[i].endCLineExc = ranges[i+1].startCLine
		}
	}

	// Track names already claimed inside this function so duplicate matches
	// (e.g. the declaration line plus a later assignment) don't double-emit.
	var out []Local
	for _, fr := range ranges {
		claimed := map[string]bool{}
		for cLine := fr.startCLine; cLine < fr.endCLineExc; cLine++ {
			line := lines[cLine-1]
			// The header line itself never declares a local in C; skip so
			// patterns like `int main(void)` are not parsed as `int main`.
			if cLine == fr.startCLine {
				continue
			}
			matches := localDeclRe.FindAllStringSubmatchIndex(line, -1)
			for _, m := range matches {
				name := line[m[6]:m[7]]
				typ := normaliseType(line[m[4]:m[5]])
				if usedGlobal[name] || claimed[name] {
					continue
				}
				rhs := line[m[7]:]
				if hasCallOnRHS(rhs) {
					continue
				}
				reg, ok := registerForDeclaration(sm, cLine, rhs)
				if !ok {
					continue
				}
				claimed[name] = true
				out = append(out, Local{
					Name: name,
					Type: typ,
					Loc:  VarLoc{Kind: VarLocReg, Reg: reg},
					Func: fr.name,
				})
			}
		}
	}
	return out
}

// registerForDeclaration picks the register a local lives in by walking the
// SourceMap and only accepting a match when the bytecode's initialisation
// agrees with the C initialiser. Three forms are accepted, in priority order:
//
//  1. C has `= <literal>` and bytecode has `MOV <reg>, <imm>` with imm equal
//     to the literal. Confirms the variable truly lives in that register.
//  2. C has `= *<expr>` (or no init at all) and bytecode has a load
//     `MOV <reg>, [addr]`. The author meant "read memory into a register".
//  3. C has no parseable init (declaration only) — fall back to the first
//     MOV destination as a best-effort, accepting that the variable will be
//     initialised by later code.
//
// Anything else (e.g. a C `i = 0` paired with `MOV ECX, 200` because the
// bytecode chose a down-counting loop instead of a literal up-count) is
// rejected, so the panel does not advertise a stale or misleading mapping.
func registerForDeclaration(sm []SourceLine, cLine int, rhs string) (string, bool) {
	init, hasInit := parseInitExpr(rhs)
	for _, sl := range sm {
		if sl.CLine != cLine {
			continue
		}
		if hasInit && init.literal != nil {
			m := movImmRe.FindStringSubmatch(sl.Mnemonic)
			if m == nil {
				continue
			}
			imm, ok := parseSignedInt(m[2])
			if !ok || imm != *init.literal {
				continue
			}
			return m[1], true
		}
		if hasInit && init.isLoad {
			m := movLoadRe.FindStringSubmatch(sl.Mnemonic)
			if m != nil {
				return m[1], true
			}
			continue
		}
		if !hasInit {
			m := movDstRe.FindStringSubmatch(sl.Mnemonic)
			if m != nil {
				return m[1], true
			}
		}
	}
	return "", false
}

// initExpr describes the right-hand side of a local declaration. Exactly one
// of literal / isLoad is set when the parser recognises a supported form.
type initExpr struct {
	literal *int64 // for `= <number>`
	isLoad  bool   // for `= *<expr>` or `= [addr]`-style
}

// initLiteralRe matches a clean integer literal initialiser like `= 10;` or
// `= 0x3F0000)`. The trailing terminator is part of the match so the regex
// rejects expressions that continue past the literal (e.g. `= a + b;`).
var initLiteralRe = regexp.MustCompile(`^\s*=\s*(-?0x[0-9A-Fa-f]+|-?\d+)\s*[;,)]`)

// parseInitExpr extracts the form of an initialiser from the slice of source
// after the declared name. Returns (zero, false) when there is no `=` at all.
func parseInitExpr(rhs string) (initExpr, bool) {
	idx := strings.IndexByte(rhs, '=')
	if idx < 0 {
		return initExpr{}, false
	}
	tail := rhs[idx:]
	if m := initLiteralRe.FindStringSubmatch(tail); m != nil {
		v, ok := parseSignedInt(m[1])
		if ok {
			return initExpr{literal: &v}, true
		}
	}
	// Strip the leading `=` and whitespace and look for a load expression:
	// `*<ident>`, `*(<cast>)<expr>` or a bare `[addr]`.
	expr := strings.TrimSpace(tail[1:])
	expr = stripLeadingCast(expr)
	if strings.HasPrefix(expr, "*") || strings.HasPrefix(expr, "[") {
		return initExpr{isLoad: true}, true
	}
	return initExpr{}, true
}

// parseSignedInt accepts decimal and 0x-prefixed integers, optionally signed,
// and returns the value as a 64-bit int. Used for both the C-source literal
// and the immediate field of a MOV mnemonic so both sides go through the
// same parser and a comparison is unambiguous.
func parseSignedInt(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	}
	var n uint64
	var err error
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		n, err = strconv.ParseUint(s[2:], 16, 64)
	} else {
		n, err = strconv.ParseUint(s, 10, 64)
	}
	if err != nil {
		return 0, false
	}
	v := int64(n)
	if neg {
		v = -v
	}
	return v, true
}

// hasCallOnRHS reports whether the right-hand side of an init contains a
// likely C function call. The check is intentionally loose: we just look for
// `<identifier>(` after the `=`. Suppresses false positives for casts because
// type-name casts in the demos use parentheses without a preceding identifier
// in the value position (e.g. `(int *)0x3F0000`).
func hasCallOnRHS(rhs string) bool {
	eq := strings.IndexByte(rhs, '=')
	if eq < 0 {
		return false
	}
	tail := rhs[eq+1:]
	// Strip a leading cast like `(int *)` so the `int(` inside it isn't
	// counted as a call. The cast always has the form (KEYWORD [*]).
	tail = stripLeadingCast(tail)
	return callRe.MatchString(tail)
}

// stripLeadingCast removes a leading C cast expression of the form
// `(type)` or `(type *)` from the input so the call detector below doesn't
// fire on it. Returns the input unchanged when no cast is present.
func stripLeadingCast(s string) string {
	s = strings.TrimLeft(s, " \t")
	if !strings.HasPrefix(s, "(") {
		return s
	}
	end := strings.Index(s, ")")
	if end < 0 {
		return s
	}
	inside := s[1:end]
	// A cast contains only a type keyword and optional `*`. If anything
	// else shows up, it's not a cast (probably an expression).
	for _, tok := range strings.Fields(inside) {
		if tok == "*" || tok == "int" || tok == "char" || tok == "long" || tok == "short" || tok == "void" || tok == "unsigned" || tok == "signed" {
			continue
		}
		return s
	}
	return s[end+1:]
}

// normaliseType folds C type aliases into the small set the simulator can
// read at snapshot time. Unknown types default to int so the value still
// renders (and the panel signals "unknown type" via its hex column).
func normaliseType(t string) VarType {
	switch t {
	case "int", "long", "short":
		return VarInt
	case "char":
		return VarChar
	}
	return VarInt
}

func sizeOfType(t VarType) uint32 {
	if t == VarChar {
		return 1
	}
	return 4
}

func parseUint(s string) (uint64, bool) {
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		v, err := strconv.ParseUint(s[2:], 16, 64)
		return v, err == nil
	}
	v, err := strconv.ParseUint(s, 10, 64)
	return v, err == nil
}
