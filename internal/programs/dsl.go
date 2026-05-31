package programs

import "github.com/jeanmarcos/modos-cpu/internal/programs/asm"

// dslProc adapts an asm.Builder into the local Proc type. The conversion
// copies SourceMap and variable-metadata entries one-for-one. Demos use this
// to keep the literal in Demos() short while the heavy lifting (instructions,
// SourceMap, declared variables) lives in a dedicated builder function.
func dslProc(b *asm.Builder) Proc {
	p := b.Build()

	sm := make([]SourceLine, len(p.SourceMap))
	for i, l := range p.SourceMap {
		sm[i] = SourceLine{Offset: l.Offset, Mnemonic: l.Mnemonic, CLine: l.CLine}
	}

	globals := make([]Global, len(p.Globals))
	for i, g := range p.Globals {
		globals[i] = Global{Name: g.Name, Type: VarType(g.Type), Addr: g.Addr, Len: g.Len}
	}

	locals := make([]Local, len(p.Locals))
	for i, l := range p.Locals {
		locals[i] = Local{
			Name: l.Name,
			Type: VarType(l.Type),
			Loc: VarLoc{
				Kind:   VarLocKind(l.Loc.Kind),
				Addr:   l.Loc.Addr,
				Reg:    l.Loc.Reg,
				Offset: l.Loc.Offset,
			},
			Func: l.Func,
		}
	}

	funcs := make([]Func, len(p.Funcs))
	for i, f := range p.Funcs {
		funcs[i] = Func{Name: f.Name, StartOffset: f.StartOffset, EndOffset: f.EndOffset}
	}

	// If the builder did not declare any variable metadata, infer it from
	// the C source + SourceMap. This is the default path: demos write the C
	// once and the panel resolves everything automatically. Authors can
	// still call .Global/.Func/.LocalReg on the builder to override.
	if len(globals) == 0 && len(locals) == 0 && len(funcs) == 0 && len(p.Source) > 0 {
		globals, locals, funcs = InferVars(p.Source, sm, uint32(len(p.Bytes)))
	}

	return Proc{
		Name:      p.Name,
		Bytes:     p.Bytes,
		Source:    p.Source,
		SourceMap: sm,
		Globals:   globals,
		Locals:    locals,
		Funcs:     funcs,
	}
}
