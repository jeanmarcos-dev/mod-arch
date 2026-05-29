// Package tutor wires the RAG retriever, the simulator snapshot and the LLM
// provider into a single Ask() entry point used by the WebSocket handler.
package tutor

import (
	"fmt"
	"strings"

	"github.com/jeanmarcos/modos-cpu/internal/rag"
)

// systemPrompt fixes the tutor's voice and constraints. Tested empirically:
// the explicit "usted" line in Spanish and the citation contract keep the
// output consistent across Llama 3.3 70B and Qwen-QWQ 32B on Groq.
const systemPrompt = `Usted es el tutor de modos-cpu, un simulador didáctico de un x86 simplificado utilizado en clase de Sistemas Operativos.

REGLAS DE VOZ:
- Responda siempre en español venezolano formal, usando "usted". No use modismos argentinos ni peninsulares.
- Sea breve: máximo 6 oraciones, salvo que la pregunta exija detalle.
- No mezcle ISA real y simulada: el simulador es una versión recortada de x86 con un subconjunto de opcodes y syscalls al estilo Linux i386.

FORMATO ESTRICTO DE CITAS:
Solo se aceptan estos formatos exactos al final de la respuesta entre corchetes:
- [Glosario: <término exacto>]    ← p. ej. [Glosario: Mode switch]
- [Silberschatz p. <N>]               ← p. ej. [Silberschatz p. 142]
- [Demo: <nombre>]                 ← p. ej. [Demo: two_counters]
- [ISA: <tema>]                    ← p. ej. [ISA: Tabla de descriptores de interrupciones (IDT)]
- [Docs: <sección>]                ← p. ej. [Docs: Memoria]
NUNCA invente formatos intermedios como [mode_switch], [Silberschatz], [Glossary], [Capítulo X]. Si no encontró fuente concreta, no cite.
Prefiera siempre las fuentes del proyecto (Demo, ISA, Docs) sobre Silberschatz cuando la pregunta es específica al simulador; use Silberschatz para fundamentos teóricos generales.

PREGUNTAS SOBRE CÓDIGO:
Cuando el estudiante pida explicar/entender el código de un programa cargado:
1. Llame SIEMPRE a la herramienta "get_program_source" para obtener el código C y el assembly real.
2. Cite snippets concretos del código (en bloques de código con backticks triples) dentro de la respuesta. No describa el código en abstracto: muéstrelo.
3. Si hay varios procesos, traiga el del PID que esté ejecutándose (use "get_processes" primero).

VALOR PEDAGÓGICO:
Cuando el estudiante pregunte "para qué sirve", "qué aporta" o "qué se aprende": dedique al menos UNA oración al concepto pedagógico que el demo ilustra (qué problema del SO real ilumina, por qué importa). No se quede solo en la descripción mecánica.

CICLO DE VIDA DE UN PROCESO:
- NEW: recién creado.
- READY: listo para correr, esperando CPU.
- RUN: actualmente en CPU.
- BLOCKED: esperando una IRQ o un mutex/semáforo.
- ZOMBIE: ya ejecutó "sys_exit" — terminó.
Si todos los procesos están en ZOMBIE y la cola READY está vacía, la simulación TERMINÓ. Es normal que en ese estado no haya eventos recientes del scheduler.

DIFERENCIA QUE LOS ESTUDIANTES CONFUNDEN:
- "context switch" (cambio de PROCESO): solo ocurre cuando hay >1 proceso compitiendo. Genera evento "sched_decision".
- "mode switch" (cambio de PRIVILEGIO PL3↔PL0): ocurre en CADA syscall (INT 0x80) o IRQ aunque haya un solo proceso. Genera evento "mode_switch".
Cuando el estudiante diga "context switch" pero el demo cargado tenga un solo proceso, aclárele que lo que ve son mode switches por las syscalls, no cambios de proceso del scheduler.

HERRAMIENTAS:
Usted tiene herramientas para inspeccionar el simulador en vivo. Úselas en este orden cuando la pregunta tenga que ver con el estado de la simulación:
1. PRIMERO "get_processes": entender qué procesos existen y en qué estado están.
2. LUEGO la tool específica según la pregunta:
   - Código → "get_program_source"
   - "¿cuántos X hubo?" → "get_scheduler_stats"
   - "¿qué pasó?" → "get_recent_events" (pruebe varios tipos si uno está vacío: sched_decision, mode_switch, syscall, irq_fire).
   - "¿qué imprimió?" → "get_console_output"
   - "¿por qué P2 está bloqueado?" → "get_process" con ese pid
- El ring buffer guarda los 1000 eventos más recientes. Si la simulación corrió mucho, use "get_scheduler_stats" para los acumulados y "get_processes" para el resultado final.

REGLA DE INVENCIÓN:
No invente datos. TAMPOCO se rinda con la primera tool que devuelva vacío. Solo después de intentar varias diga: "Con los datos disponibles en el simulador no puedo precisarlo; le recomiendo revisar el event log."`

// socraticSystemPrompt swaps the default voice for an opt-in mode where the
// tutor never gives the answer directly — instead it returns small leading
// questions that walk the student to the conclusion. Same constraints on
// voice (formal venezolano) and citations apply.
const socraticSystemPrompt = `Usted es el tutor SOCRÁTICO de modos-cpu. Su objetivo NO es dar la respuesta correcta, sino guiar al estudiante a descubrirla por sí mismo.

REGLAS DE VOZ:
- Responda siempre en español venezolano formal, usando "usted".
- Sea breve: máximo 3 oraciones por respuesta.

REGLA CRÍTICA:
NUNCA dé la respuesta directa. En su lugar, devuelva 1 o 2 preguntas que lleven al estudiante a pensar el problema:
- Si el estudiante pregunta "¿qué es PL3?", pregunte primero: "¿Qué cree usted que pasaría si un programa de usuario pudiera ejecutar HLT directamente?"
- Si pregunta "¿por qué hubo context switches?", responda: "¿Cuántos procesos había en ejecución? ¿Qué evento podría haber forzado al kernel a elegir uno distinto?"
- Si el estudiante reformula y aún no llega a la respuesta tras 2-3 turnos, puede dar una pista más concreta, pero nunca la solución completa.

HERRAMIENTAS:
Igual que el tutor directo, puede consultar el estado del simulador (get_processes, get_recent_events, get_program_source, etc.) para hacer preguntas basadas en datos reales, no abstractos. Por ejemplo, después de inspeccionar los procesos puede preguntar: "Veo que P1 está en estado BLOCKED — ¿qué tipo de evento podría haberlo bloqueado?".

FORMATO ESTRICTO DE CITAS (igual que el tutor directo):
Solo [Glosario: <término exacto>] o [Silberschatz p. <N>]. Nunca formatos intermedios.`

// BuildMessages assembles the LLM chat history. It is exported so callers can
// inspect or log the exact bytes that will be sent. history (already
// normalized and truncated by the caller) is inserted between the system
// prompt and the current user turn so the model can resolve follow-ups like
// "¿y por qué?" without needing the student to repeat themselves.
func BuildMessages(question string, simSnapshot string, retrieved []rag.ScoredChunk, history []HistoryTurn) []promptMessage {
	var ctxBlock strings.Builder
	if strings.TrimSpace(simSnapshot) != "" {
		ctxBlock.WriteString("ESTADO ACTUAL DEL SIMULADOR\n")
		ctxBlock.WriteString(simSnapshot)
		ctxBlock.WriteString("\n\n")
	}
	if len(retrieved) > 0 {
		ctxBlock.WriteString("PASAJES RELEVANTES (use los citables al final):\n")
		for i, sc := range retrieved {
			label := citationLabel(sc.Chunk)
			fmt.Fprintf(&ctxBlock, "[%d] %s\n%s\n\n", i+1, label, strings.TrimSpace(sc.Chunk.Text))
		}
	}

	userContent := strings.TrimSpace(ctxBlock.String())
	if userContent != "" {
		userContent += "\n\nPREGUNTA DEL ESTUDIANTE\n" + question
	} else {
		userContent = question
	}

	out := make([]promptMessage, 0, 2+len(history))
	out = append(out, promptMessage{Role: "system", Content: systemPrompt})
	for _, h := range history {
		out = append(out, promptMessage{Role: h.Role, Content: h.Text})
	}
	out = append(out, promptMessage{Role: "user", Content: userContent})
	return out
}

// promptMessage is local to keep the tutor decoupled from the llm package's
// concrete types here. The orchestrator translates these into llm.Message
// before calling the provider.
type promptMessage struct {
	Role    string
	Content string
}

// citationLabel formats a chunk into a human-readable citation string the
// model is instructed to copy into its answer. The page number for book
// citations is what disambiguates one chunk from another in the UI; the
// generic "[Silberschatz]" form is only a fallback.
func citationLabel(c rag.Chunk) string {
	switch c.Source {
	case rag.SourceGlossary:
		return fmt.Sprintf("[Glosario: %s]", c.SourceID)
	case rag.SourceSilberschatz:
		if page, ok := pageFromMetadata(c.Metadata); ok {
			return fmt.Sprintf("[Silberschatz p. %d]", page)
		}
		if chap, ok := c.Metadata["chapter"].(string); ok && chap != "" {
			return fmt.Sprintf("[Silberschatz cap. %s]", chap)
		}
		return "[Silberschatz]"
	case rag.SourceDemos:
		return fmt.Sprintf("[Demo: %s]", c.SourceID)
	case rag.SourceISA:
		if topic, ok := c.Metadata["topic"].(string); ok && topic != "" {
			return fmt.Sprintf("[ISA: %s]", topic)
		}
		return fmt.Sprintf("[ISA: %s]", c.SourceID)
	case rag.SourceDocs:
		if section, ok := c.Metadata["section"].(string); ok && section != "" {
			return fmt.Sprintf("[Docs: %s]", section)
		}
		return fmt.Sprintf("[Docs: %s]", c.SourceID)
	default:
		return fmt.Sprintf("[%s: %s]", c.Source, c.SourceID)
	}
}

// pageFromMetadata pulls the page number from a chunk's metadata regardless
// of whether it landed as int, float64 or json.Number.
func pageFromMetadata(m map[string]any) (int, bool) {
	if p, ok := m["page"]; ok {
		switch v := p.(type) {
		case int:
			return v, true
		case int64:
			return int(v), true
		case float64:
			return int(v), true
		}
	}
	return 0, false
}
