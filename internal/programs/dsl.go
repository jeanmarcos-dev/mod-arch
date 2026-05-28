package programs

import "github.com/jeanmarcos/modos-cpu/internal/programs/asm"

// dslProc adapts an asm.Builder into the local Proc type. The conversion
// copies SourceMap entries one-for-one. Demos use this to keep the literal
// in Demos() short while the heavy lifting (instructions, SourceMap) lives
// in a dedicated builder function.
func dslProc(b *asm.Builder) Proc {
	p := b.Build()
	sm := make([]SourceLine, len(p.SourceMap))
	for i, l := range p.SourceMap {
		sm[i] = SourceLine{Offset: l.Offset, Mnemonic: l.Mnemonic, CLine: l.CLine}
	}
	return Proc{Name: p.Name, Bytes: p.Bytes, Source: p.Source, SourceMap: sm}
}
