package rag

import (
	"fmt"
	"strings"

	"github.com/jeanmarcos/modos-cpu/internal/programs"
)

// LoadDemosAsRawDocs builds one RawDoc per demo with the prose description,
// the C-like source of each Proc and a curated list of mnemonics. The intent
// is to let the tutor reason about each demo without having to invoke the
// get_program_source tool (the tool stays for live introspection of the
// currently loaded program).
func LoadDemosAsRawDocs(demos []programs.Demo) []RawDoc {
	out := make([]RawDoc, 0, len(demos))
	for _, d := range demos {
		text := renderDemoForRAG(d)
		category := d.Category
		if category == "" {
			category = "general"
		}
		procNames := make([]string, 0, len(d.Procs))
		for _, p := range d.Procs {
			procNames = append(procNames, p.Name)
		}
		out = append(out, RawDoc{
			Source:   SourceDemos,
			SourceID: d.Name,
			Text:     text,
			Metadata: map[string]any{
				"demo":     d.Name,
				"category": category,
				"procs":    procNames,
				"cores":    d.Cores,
			},
		})
	}
	return out
}

// renderDemoForRAG flattens one Demo into the chunk text fed to the embedder
// and retrieved as context. Layout:
//
//	Demo "<name>" (<category>)
//	<description>
//
//	Proceso <i>: <proc.Name>
//	[C source ...]
//	[ASM mnemonics ...]
func renderDemoForRAG(d programs.Demo) string {
	var b strings.Builder
	category := d.Category
	if category == "" {
		category = "general"
	}
	fmt.Fprintf(&b, "Demo \"%s\" (categoría: %s)\n", d.Name, category)
	if d.Description != "" {
		b.WriteString(d.Description)
		b.WriteByte('\n')
	}
	if d.Cores > 1 {
		fmt.Fprintf(&b, "Configuración: %d núcleos activos.\n", d.Cores)
	}
	for i, p := range d.Procs {
		fmt.Fprintf(&b, "\nProceso %d: %s\n", i+1, p.Name)
		if src := strings.TrimSpace(p.Source); src != "" {
			b.WriteString("```c\n")
			b.WriteString(src)
			if !strings.HasSuffix(src, "\n") {
				b.WriteByte('\n')
			}
			b.WriteString("```\n")
		}
		if len(p.SourceMap) > 0 {
			b.WriteString("ASM:\n")
			b.WriteString("```asm\n")
			for _, sl := range p.SourceMap {
				fmt.Fprintf(&b, "%04x  %s\n", sl.Offset, sl.Mnemonic)
			}
			b.WriteString("```\n")
		}
	}
	return b.String()
}
