package tutor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jeanmarcos/modos-cpu/internal/llm"
	"github.com/jeanmarcos/modos-cpu/internal/rag"
	"github.com/jeanmarcos/modos-cpu/internal/sessions"
)

// citationTokenRe matches every label format declared in prompts.go:
//   [Glosario: id]   [Demo: id]   [ISA: id]   [Docs: id]   [Silberschatz p. N]
// The leading \s* eats spaces and newlines that precede the token so trailing
// citation blocks collapse cleanly when stripped from the answer.
var citationTokenRe = regexp.MustCompile(`\s*\[(?:Glosario|Demo|ISA|Docs|Silberschatz)[^\]]*\]`)

// stripCitationTokens removes inline citation labels from the LLM's reply.
// buildCitationsFromText still needs the raw text to filter the citation
// array, so callers should keep a copy of the original before invoking this.
func stripCitationTokens(text string) string {
	cleaned := citationTokenRe.ReplaceAllString(text, "")
	cleaned = strings.TrimRight(cleaned, " ,;\n\t")
	return strings.TrimSpace(cleaned)
}

// Citation is a structured reference the UI renders as a chip. Glossary
// citations open the GlossaryModal on ID; book citations are informative
// only (the student looks the page up in their own copy of the book).
type Citation struct {
	Source  string `json:"source"`
	ID      string `json:"id"`
	Label   string `json:"label"`
	Page    int    `json:"page,omitempty"`
	Excerpt string `json:"excerpt,omitempty"`
}

// Answer is the orchestrator's response to a question.
type Answer struct {
	Text           string     `json:"text"`
	Citations      []Citation `json:"citations"`
	CorpusVersion  int        `json:"corpus_version"`
	Provider       string     `json:"provider"`
	Model          string     `json:"model"`
	LatencyMS      int64      `json:"latency_ms"`
	CacheHit       bool       `json:"cache_hit"`
	Degraded       bool       `json:"degraded"` // true when the LLM call failed and we fell back to retriever-only
	UsageTokens    int        `json:"usage_tokens,omitempty"`
	ToolsUsedNames []string   `json:"tools_used,omitempty"`
	// LastError carries the underlying LLM error when Degraded=true. Kept out
	// of the wire payload (json:"-") because the message can include request
	// dumps useful only for server-side metrics / debugging.
	LastError string `json:"-"`
}

// Options configures Service behavior. Zero values get reasonable defaults.
type Options struct {
	TopK            int
	MaxAnswerTokens int
	Temperature     float64
	RateLimitMax    int
	RateLimitWindow time.Duration
	CorpusVersion   int
	MaxToolIters    int // safety cap on the tool-calling loop; default 3
	// HistoryWindowTurns caps how many prior student/tutor turns are forwarded
	// to the LLM on each Ask. 0 keeps the legacy stateless behavior; the
	// builder is responsible for picking the default (currently 3) before
	// reaching here. Clamped to [0,10] in withDefaults.
	HistoryWindowTurns int
}

// historyHardCap mirrors the validation in the admin API: even if the builder
// passes something out of range, we never replay more than this many turns.
const historyHardCap = 10

// defaultMaxAnswerTokens caps the per-reply token budget. Modern Gemini and
// other "thinking" models burn part of that budget on internal reasoning
// before any visible text, and our prompt now carries eager-tool payloads
// (process list JSON, current program source + ASM, scheduler config) on
// top of the retrieved chunks. With that amount of context the model can
// easily spend 1500+ tokens on reasoning alone, leaving the visible reply
// truncated. 3000 is the smallest cap that consistently produces a complete
// answer across our hardest demos without inviting wall-of-text replies.
// Shared between Options.withDefaults and Build so both code paths agree.
const defaultMaxAnswerTokens = 3000

func (o Options) withDefaults() Options {
	if o.TopK <= 0 {
		o.TopK = 5
	}
	if o.MaxAnswerTokens <= 0 {
		o.MaxAnswerTokens = defaultMaxAnswerTokens
	}
	if o.Temperature == 0 {
		o.Temperature = 0.3
	}
	if o.RateLimitMax <= 0 {
		o.RateLimitMax = 20
	}
	if o.RateLimitWindow <= 0 {
		o.RateLimitWindow = time.Hour
	}
	if o.MaxToolIters <= 0 {
		o.MaxToolIters = 3
	}
	if o.HistoryWindowTurns < 0 {
		o.HistoryWindowTurns = 0
	}
	if o.HistoryWindowTurns > historyHardCap {
		o.HistoryWindowTurns = historyHardCap
	}
	return o
}

// Service is the tutor's single dependency the WebSocket handler talks to.
type Service struct {
	retriever *rag.Retriever
	store     *rag.Store
	llm       llm.Provider
	limiter   *RateLimiter
	bucket    *llm.TokenBucket
	logger    *slog.Logger
	opts      Options
	metrics   *MetricsStore

	toolDefs    []llm.ToolDef
	toolByName  map[string]ToolHandler
	tools       []Tool // full registry; needed to iterate eager tools generically
	defaultMode string // applied when AskWithOptions receives Mode == ""
}

// SetMetricsStore enables persistence of one row per Ask() into the
// tutor_metrics table. Optional; nil leaves metrics disabled.
func (s *Service) SetMetricsStore(m *MetricsStore) { s.metrics = m }

// SetRateBucket enables process-wide rate limiting for outbound LLM calls.
// Each iteration (initial + per-tool round-trip) acquires one token before
// hitting the provider. Pass nil to disable.
func (s *Service) SetRateBucket(b *llm.TokenBucket) { s.bucket = b }

// SetDefaultMode controls which system prompt is used when AskWithOptions
// is called without an explicit Mode. "" or "direct" use the regular tutor;
// "socratic" swaps in the question-driven prompt.
func (s *Service) SetDefaultMode(mode string) {
	s.defaultMode = strings.ToLower(strings.TrimSpace(mode))
}

// ProgressCallback receives intermediate events during Ask so the WS layer
// can stream "consultando: get_recent_events..." hints to the UI. Stage is
// one of: "tool_call", "tool_result". Detail carries the relevant payload.
type ProgressCallback func(stage string, detail map[string]any)

// New builds a Service. retriever must be Reload()-ed before the first call.
func New(retriever *rag.Retriever, store *rag.Store, provider llm.Provider, logger *slog.Logger, opts Options) *Service {
	opts = opts.withDefaults()
	if logger == nil {
		logger = slog.Default()
	}
	tools := DefaultTools()
	defs := make([]llm.ToolDef, len(tools))
	handlers := make(map[string]ToolHandler, len(tools))
	for i, t := range tools {
		defs[i] = t.Def
		handlers[t.Def.Name] = t.Handler
	}
	return &Service{
		retriever:  retriever,
		store:      store,
		llm:        provider,
		limiter:    NewRateLimiter(opts.RateLimitMax, opts.RateLimitWindow),
		logger:     logger,
		opts:       opts,
		toolDefs:   defs,
		toolByName: handlers,
		tools:      tools,
	}
}

// Close releases background resources.
func (s *Service) Close() {
	if s.limiter != nil {
		s.limiter.Close()
	}
}

// ErrRateLimited is returned by Ask when the session has exceeded its quota.
// The error carries a Duration via errors.As for callers that want to show a
// "retry in N seconds" message.
var ErrRateLimited = errors.New("tutor: rate limited")

// RateLimitError is the concrete error wrapping ErrRateLimited; it exposes
// the retry-after window so handlers can pass it to the UI.
type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("tutor: rate limited, vuelva a intentar en %s", e.RetryAfter.Round(time.Second))
}
func (e *RateLimitError) Unwrap() error { return ErrRateLimited }

// HistoryTurn is one prior message in the conversation forwarded by the
// client. Role must be either "user" (the student) or "assistant" (the tutor);
// anything else is dropped during normalization so a malformed payload cannot
// smuggle "system" turns past the system prompt.
type HistoryTurn struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

// AskOptions tunes per-question behavior. Mode controls the system prompt;
// "" or "direct" use the default tutor, "socratic" swaps in a question-only
// prompt to push the student to reason out the answer. History carries the
// recent multi-turn context (oldest first); the service truncates it to the
// window configured by Options.HistoryWindowTurns.
type AskOptions struct {
	Mode    string
	History []HistoryTurn
}

// Ask runs the full pipeline for a single question from a session. progress
// is optional; pass nil when the caller does not care about intermediate
// tool-call events.
func (s *Service) Ask(ctx context.Context, sess *sessions.Session, question string, progress ProgressCallback) (*Answer, error) {
	return s.AskWithOptions(ctx, sess, question, AskOptions{}, progress)
}

// AskWithOptions is Ask plus per-question overrides.
func (s *Service) AskWithOptions(ctx context.Context, sess *sessions.Session, question string, opts AskOptions, progress ProgressCallback) (*Answer, error) {
	// Apply the admin-configured default if the caller did not pick a mode.
	if opts.Mode == "" {
		opts.Mode = s.defaultMode
	}
	ans, err := s.askInternal(ctx, sess, question, opts, progress)
	s.recordMetric(ctx, sess, question, ans, err)
	return ans, err
}

// recordMetric persists one row to tutor_metrics. Never returns an error to
// the caller — observability must not break answer delivery.
func (s *Service) recordMetric(ctx context.Context, sess *sessions.Session, question string, ans *Answer, callErr error) {
	if s.metrics == nil {
		return
	}
	sessID, userID := "", ""
	if sess != nil {
		sessID = sess.ID
		userID = sess.UserID()
	}
	row := MetricRow{
		Timestamp:      time.Now(),
		SessionID:      sessID,
		UserID:         userID,
		QuestionDigest: DigestQuestion(question),
		QuestionChars:  len(question),
	}
	if ans != nil {
		row.LatencyMS = ans.LatencyMS
		row.CacheHit = ans.CacheHit
		row.Degraded = ans.Degraded
		row.ToolsUsed = ans.ToolsUsedNames
		row.TokensTotal = ans.UsageTokens
		row.CitationsCount = len(ans.Citations)
	}
	if callErr != nil {
		row.Error = callErr.Error()
	} else if ans != nil && ans.LastError != "" {
		row.Error = ans.LastError
	}
	if err := s.metrics.Record(ctx, row); err != nil {
		s.logger.Warn("tutor: metrics record failed", "err", err)
	}
}

func (s *Service) askInternal(ctx context.Context, sess *sessions.Session, question string, opts AskOptions, progress ProgressCallback) (*Answer, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return nil, errors.New("tutor: empty question")
	}
	if utf8RuneCount(question) > 2000 {
		return nil, errors.New("tutor: question too long (max ~2000 chars)")
	}

	sessID := ""
	if sess != nil {
		sessID = sess.ID
	}
	if sessID != "" {
		if ok, retry := s.limiter.Allow(sessID); !ok {
			return nil, &RateLimitError{RetryAfter: retry}
		}
	}

	start := time.Now()
	snapshot := BuildSnapshot(sess)
	snapText := snapshot.Render()

	history := normalizeHistory(opts.History, s.opts.HistoryWindowTurns)

	// Answer cache lookup. Key includes corpus version so a reindex
	// invalidates all hits without us having to drop the table; it also
	// includes the conversation history so the same question after a
	// different follow-up does not collide.
	cacheKey := s.answerCacheKey(question, snapshot, history)
	if cached, err := s.store.GetCachedAnswer(ctx, cacheKey); err == nil && cached != nil {
		var cites []Citation
		_ = json.Unmarshal(cached.Citations, &cites)
		return &Answer{
			Text:          cached.Answer,
			Citations:     cites,
			CorpusVersion: s.opts.CorpusVersion,
			CacheHit:      true,
			LatencyMS:     time.Since(start).Milliseconds(),
		}, nil
	}

	// Pre-execute the inspection tools whose answer almost every question
	// needs. Running them deterministically here removes the dependency on
	// the LLM correctly deciding to call them via function calling; the
	// results land in the prompt directly. See runEagerTools for which tools
	// are eager vs on-demand.
	eager := s.runEagerTools(ctx, sess, snapshot)

	retrieved, err := s.retriever.Retrieve(ctx, question, s.opts.TopK)
	if err != nil {
		s.logger.Warn("tutor: retrieval failed", "err", err)
		// Continue without retrieved context; the LLM still has the snapshot.
	}
	// Hybrid retrieval: when a program is loaded, guarantee its demo chunk
	// reaches the prompt even if cosine similarity buried it under the bigger
	// corpus (Silberschatz alone has ~1500 chunks). A second retrieval keyed
	// on the program name lets us pick the matching demo chunk and merge it
	// in, displacing the weakest result if needed.
	if snapshot.Program != "" {
		demoMatches, dErr := s.retriever.Retrieve(ctx, snapshot.Program, 5)
		if dErr != nil {
			s.logger.Warn("tutor: demo retrieval failed", "err", dErr, "program", snapshot.Program)
		} else {
			for _, m := range demoMatches {
				if m.Chunk.Source == rag.SourceDemos {
					retrieved = mergeDemoIntoTopK(retrieved, m, s.opts.TopK)
					break
				}
			}
		}
	}

	initial := BuildMessages(question, snapText, eager, retrieved, history)
	messages := toLLMMessages(initial)
	// Socratic mode replaces the first system message with the alternate
	// prompt. Tools and snapshot context stay untouched so the model can
	// still inspect the simulator when guiding the student.
	if opts.Mode == "socratic" && len(messages) > 0 && messages[0].Role == llm.RoleSystem {
		messages[0].Content = socraticSystemPrompt
	}

	totalUsage := 0
	toolsUsed := 0
	toolNames := make([]string, 0, 4)
	var lastModel string
	var finalContent string
	var rawFinalContent string
	streamer, canStream := s.llm.(llm.Streamer)

	for iter := 0; iter < s.opts.MaxToolIters+1; iter++ {
		req := llm.ChatRequest{
			Temperature: s.opts.Temperature,
			MaxTokens:   s.opts.MaxAnswerTokens,
			Messages:    messages,
			Tools:       s.toolDefs,
			ToolChoice:  "auto",
		}
		// On the last allowed iteration, force a textual answer instead of
		// another round of tool calls. Otherwise a stubborn model could loop.
		if iter == s.opts.MaxToolIters {
			req.Tools = nil
			req.ToolChoice = ""
		}

		text, rawText, toolCalls, usage, model, err := s.callWithRetry(ctx, progress, func() (string, string, []llm.ToolCall, llm.Usage, string, error) {
			return s.callLLM(ctx, streamer, canStream, req, progress)
		})
		if err != nil {
			s.logger.Warn("tutor: llm call failed, falling back to retriever-only",
				"err", err, "session", sessID, "iter", iter,
			)
			return s.fallbackAnswer(retrieved, start, err), nil
		}
		totalUsage += usage.TotalTokens
		lastModel = model

		if len(toolCalls) == 0 {
			finalContent = strings.TrimSpace(text)
			rawFinalContent = strings.TrimSpace(rawText)
			break
		}

		// Append the assistant's tool-request turn so the next message has
		// the matching role/ids for each tool result. We deliberately drop
		// any "thinking aloud" text the model emitted alongside tool_calls:
		// some providers (Gemini) 400 when an assistant turn carries BOTH
		// content and tool_calls, and replaying that text adds no value to
		// the next iteration's prompt.
		messages = append(messages, llm.Message{
			Role:      llm.RoleAssistant,
			ToolCalls: toolCalls,
		})
		_ = text    // tool-iteration text is intentionally discarded; see comment above.
		_ = rawText // same as text — raw form is only relevant on the final iteration.

		for _, call := range toolCalls {
			toolsUsed++
			toolNames = append(toolNames, call.Name)
			handler, ok := s.toolByName[call.Name]
			result := s.runTool(ctx, sess, call, handler, ok, progress)
			messages = append(messages, llm.Message{
				Role:       llm.RoleTool,
				ToolCallID: call.ID,
				Name:       call.Name,
				Content:    result,
			})
		}
	}

	if finalContent == "" {
		// Defensive: model exhausted iterations without a textual answer.
		finalContent = "El tutor consultó varias herramientas pero no logró formular una respuesta concreta. Por favor reformule la pregunta."
	}

	// Citations are filtered by literal label match against the raw reply —
	// the cleaned finalContent already has those tokens stripped, so use the
	// pre-strip text here. When rawFinalContent is empty (e.g. the defensive
	// fallback above), fall back to finalContent (no citations will match).
	citationSource := rawFinalContent
	if citationSource == "" {
		citationSource = finalContent
	}
	citations := buildCitationsFromText(citationSource, retrieved)
	ans := &Answer{
		Text:           finalContent,
		Citations:      citations,
		CorpusVersion:  s.opts.CorpusVersion,
		Provider:       s.llm.Name(),
		Model:          lastModel,
		LatencyMS:      time.Since(start).Milliseconds(),
		UsageTokens:    totalUsage,
		ToolsUsedNames: toolNames,
	}

	// Best-effort: persist for next hit. Failure here does not break the call.
	citesJSON, _ := json.Marshal(ans.Citations)
	_ = s.store.PutAnswer(ctx, cacheKey, ans.Text, citesJSON)

	s.logger.Info("tutor: answered",
		"session", sessID,
		"latency_ms", ans.LatencyMS,
		"tokens", ans.UsageTokens,
		"retrieved", len(retrieved),
		"tools_used", toolsUsed,
	)
	return ans, nil
}

// retryDelays controls the wait between attempts when the provider rate-limits
// us. Three retries (2s, 5s, 10s) absorb a brief free-tier spike without
// pushing the total Ask latency past the 90s WS deadline.
var retryDelays = []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second}

// callWithRetry wraps a single LLM call with bounded backoff on rate limits.
// Non-rate-limit errors propagate immediately so the orchestrator's fallback
// path kicks in. The progress callback signals retries to the UI.
func (s *Service) callWithRetry(
	ctx context.Context,
	progress ProgressCallback,
	fn func() (text string, rawText string, toolCalls []llm.ToolCall, usage llm.Usage, model string, err error),
) (text string, rawText string, toolCalls []llm.ToolCall, usage llm.Usage, model string, err error) {
	for attempt := 0; ; attempt++ {
		text, rawText, toolCalls, usage, model, err = fn()
		if err == nil || !errors.Is(err, llm.ErrRateLimited) || attempt >= len(retryDelays) {
			return
		}
		wait := retryDelays[attempt]
		s.logger.Warn("tutor: rate limited, retrying", "attempt", attempt+1, "wait", wait, "err", err)
		if progress != nil {
			progress("retry", map[string]any{"attempt": attempt + 1, "wait_seconds": int(wait.Seconds())})
		}
		select {
		case <-ctx.Done():
			return text, rawText, toolCalls, usage, model, ctx.Err()
		case <-time.After(wait):
		}
	}
}

// callLLM picks streaming or synchronous depending on provider capability.
// In stream mode we buffer deltas until we see the FinishReason — if no tool
// calls came in, we replay the buffered text to the UI as live tokens; if
// tool calls were emitted instead, we silently discard the buffer (those
// deltas were the model thinking aloud about which tool to use).
//
// rawText holds the LLM's unmodified output (with inline citation labels
// intact); text is the cleaned version that gets streamed to the UI and
// returned as the answer body. The raw form is needed by
// buildCitationsFromText to filter which retrieved chunks the model actually
// referenced.
func (s *Service) callLLM(ctx context.Context, streamer llm.Streamer, canStream bool, req llm.ChatRequest, progress ProgressCallback) (text string, rawText string, toolCalls []llm.ToolCall, usage llm.Usage, model string, err error) {
	// Rate-limit gate. If the bucket is empty, signal the UI before parking
	// the goroutine on Acquire so the student sees "en cola (~Ns)" instead
	// of a frozen spinner.
	if s.bucket != nil {
		if !s.bucket.TryAcquire() {
			eta := s.bucket.NextAvailableIn()
			if progress != nil {
				progress("queued", map[string]any{
					"wait_seconds":   int(eta.Round(time.Second).Seconds()),
					"queue_depth":    s.bucket.QueueDepth(),
				})
			}
			if err := s.bucket.Acquire(ctx); err != nil {
				return "", "", nil, llm.Usage{}, "", err
			}
		}
	}

	if !canStream {
		resp, err := s.llm.Chat(ctx, req)
		if err != nil {
			return "", "", nil, llm.Usage{}, "", err
		}
		return stripCitationTokens(resp.Content), resp.Content, resp.ToolCalls, resp.Usage, resp.Model, nil
	}

	stream, err := streamer.ChatStream(ctx, req)
	if err != nil {
		return "", "", nil, llm.Usage{}, "", err
	}
	defer stream.Close()

	var buf strings.Builder
	for ev := range stream.Events {
		if ev.Delta != "" {
			buf.WriteString(ev.Delta)
		}
		if len(ev.ToolCalls) > 0 {
			toolCalls = ev.ToolCalls
		}
		if ev.Usage.TotalTokens > 0 {
			usage = ev.Usage
		}
	}
	if e := stream.Err(); e != nil {
		return "", "", nil, llm.Usage{}, "", e
	}
	rawText = buf.String()
	text = stripCitationTokens(rawText)

	// Final-answer iteration: stream the accumulated text to the UI now so
	// the student sees a typewriter effect. We do this AFTER we know there
	// are no tool calls so intermediate "thinking" tokens never leak. The
	// cleaned text is replayed so inline citation labels never reach the UI.
	if len(toolCalls) == 0 && progress != nil && text != "" {
		s.replayTokens(text, progress)
	}
	return text, rawText, toolCalls, usage, req.Model, nil
}

// replayTokens emits the model's textual answer as a series of progress
// events so the UI can render it incrementally. Chunk size is small enough
// to feel like real streaming but large enough to keep WS chatter sane.
func (s *Service) replayTokens(text string, progress ProgressCallback) {
	const chunkRunes = 12
	runes := []rune(text)
	for i := 0; i < len(runes); i += chunkRunes {
		end := i + chunkRunes
		if end > len(runes) {
			end = len(runes)
		}
		progress("token", map[string]any{"delta": string(runes[i:end])})
	}
}

// runTool executes one tool call, notifies the progress callback before and
// after, and returns a JSON string ready to feed back to the LLM. Errors are
// serialized as JSON too so the model sees them and can recover.
func (s *Service) runTool(ctx context.Context, sess *sessions.Session, call llm.ToolCall, handler ToolHandler, ok bool, progress ProgressCallback) string {
	if progress != nil {
		progress("tool_call", map[string]any{"tool": call.Name, "arguments": call.Arguments, "call_id": call.ID})
	}
	if !ok {
		errPayload, _ := json.Marshal(map[string]any{"error": "tool desconocida: " + call.Name})
		if progress != nil {
			progress("tool_result", map[string]any{"call_id": call.ID, "ok": false, "error": "unknown tool"})
		}
		return string(errPayload)
	}
	result, err := handler(ctx, sess, json.RawMessage(call.Arguments))
	if err != nil {
		errPayload, _ := json.Marshal(map[string]any{"error": err.Error()})
		s.logger.Warn("tutor: tool failed", "tool", call.Name, "err", err)
		if progress != nil {
			progress("tool_result", map[string]any{"call_id": call.ID, "ok": false, "error": err.Error()})
		}
		return string(errPayload)
	}
	out, err := json.Marshal(result)
	if err != nil {
		errPayload, _ := json.Marshal(map[string]any{"error": "encode result: " + err.Error()})
		return string(errPayload)
	}
	if progress != nil {
		progress("tool_result", map[string]any{"call_id": call.ID, "ok": true, "bytes": len(out)})
	}
	return string(out)
}

// fallbackAnswer surfaces the retrieved passages when the LLM call failed.
// Better than blanking out; the student still gets something to read. The
// underlying llmErr is preserved on Answer.LastError so observability can
// record what went wrong without exposing it on the wire.
func (s *Service) fallbackAnswer(retrieved []rag.ScoredChunk, start time.Time, llmErr error) *Answer {
	var b strings.Builder
	b.WriteString("El tutor no pudo conectarse con el modelo de lenguaje en este momento. ")
	b.WriteString("Le dejamos los pasajes más cercanos a su pregunta para que los revise:\n\n")
	if len(retrieved) == 0 {
		b.WriteString("(no se encontraron pasajes relevantes en el corpus)")
	}
	for i, sc := range retrieved {
		fmt.Fprintf(&b, "%d. %s — %s\n\n", i+1, citationLabel(sc.Chunk), shorten(sc.Chunk.Text, 280))
	}
	errMsg := ""
	if llmErr != nil {
		errMsg = llmErr.Error()
	}
	return &Answer{
		Text:          strings.TrimSpace(b.String()),
		Citations:     buildCitations(retrieved),
		CorpusVersion: s.opts.CorpusVersion,
		Provider:      "",
		Degraded:      true,
		LatencyMS:     time.Since(start).Milliseconds(),
		LastError:     errMsg,
	}
}

// mergeDemoIntoTopK guarantees a demo chunk lands in the retrieved slice
// without growing it. Returns base unchanged when the demo already appears
// in it (matched by chunk ID). When the slice still has spare capacity the
// demo is appended; otherwise it displaces the lowest-scoring entry (the
// last position in a sorted-by-score slice).
func mergeDemoIntoTopK(base []rag.ScoredChunk, demo rag.ScoredChunk, topK int) []rag.ScoredChunk {
	for _, c := range base {
		if c.Chunk.ID == demo.Chunk.ID {
			return base
		}
	}
	if len(base) < topK {
		return append(base, demo)
	}
	out := make([]rag.ScoredChunk, len(base))
	copy(out, base)
	out[len(out)-1] = demo
	return out
}

// EagerResult is one pre-executed tool's payload, ready to be embedded in
// the user prompt under Label. The orchestrator keeps the slice ordered so
// the prompt sections render in a predictable order.
type EagerResult struct {
	ToolName string
	Label    string
	Payload  string
}

// runEagerTools iterates over the registered tools, runs every one marked
// Eager (whose optional EagerWhen predicate passes) in parallel, and returns
// the JSON-encoded payloads in registration order. The set of eager tools is
// data-driven: adding a new tool with Eager=true (including future MCP
// tools) becomes part of the pre-load without touching this function or the
// system prompt. Failures are swallowed silently — eager pre-load is a
// best-effort optimization, the LLM can still invoke the tool on demand.
func (s *Service) runEagerTools(ctx context.Context, sess *sessions.Session, snap SnapshotView) []EagerResult {
	if sess == nil || sess.Sim == nil {
		return nil
	}
	results := make([]EagerResult, len(s.tools))
	var wg sync.WaitGroup
	for i, t := range s.tools {
		if !t.Eager {
			continue
		}
		if t.EagerWhen != nil && !t.EagerWhen(snap) {
			continue
		}
		wg.Add(1)
		go func(idx int, tool Tool) {
			defer wg.Done()
			var args json.RawMessage
			if tool.EagerArgs != nil {
				args = tool.EagerArgs(snap)
			}
			v, err := tool.Handler(ctx, sess, args)
			if err != nil {
				return
			}
			b, mErr := json.Marshal(v)
			if mErr != nil {
				return
			}
			label := tool.EagerLabel
			if label == "" {
				label = strings.ToUpper(tool.Def.Name)
			}
			results[idx] = EagerResult{
				ToolName: tool.Def.Name,
				Label:    label,
				Payload:  string(b),
			}
		}(i, t)
	}
	wg.Wait()
	// Compact zero entries (skipped or failed tools) preserving order.
	out := results[:0]
	for _, r := range results {
		if r.Payload != "" {
			out = append(out, r)
		}
	}
	return out
}

// answerCacheKey hashes question + snapshot digest + corpus version + the
// truncated history. Including the history prevents two different follow-ups
// to the same base question from returning the same cached answer.
func (s *Service) answerCacheKey(question string, snap SnapshotView, history []HistoryTurn) string {
	h := sha256.New()
	fmt.Fprintf(h, "v=%d|q=%s|snap=%s",
		s.opts.CorpusVersion,
		strings.ToLower(strings.TrimSpace(question)),
		snap.Digest(),
	)
	for i, t := range history {
		fmt.Fprintf(h, "|h%d=%s:%s", i, t.Role, t.Text)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// normalizeHistory drops malformed entries (unknown roles, empty text) and
// keeps only the most recent 2*maxTurns messages — one student + one tutor
// per "turn". maxTurns=0 returns nil so the legacy stateless behavior is
// preserved when the admin explicitly opts out.
func normalizeHistory(in []HistoryTurn, maxTurns int) []HistoryTurn {
	if maxTurns <= 0 || len(in) == 0 {
		return nil
	}
	out := make([]HistoryTurn, 0, len(in))
	for _, h := range in {
		role := strings.ToLower(strings.TrimSpace(h.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		text := strings.TrimSpace(h.Text)
		if text == "" {
			continue
		}
		out = append(out, HistoryTurn{Role: role, Text: text})
	}
	limit := maxTurns * 2
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

func buildCitations(retrieved []rag.ScoredChunk) []Citation {
	out := make([]Citation, 0, len(retrieved))
	for _, sc := range retrieved {
		page := 0
		if p, ok := sc.Chunk.Metadata["page"]; ok {
			if n, ok := anyToInt(p); ok {
				page = n
			}
		}
		out = append(out, Citation{
			Source:  string(sc.Chunk.Source),
			ID:      sc.Chunk.SourceID,
			Label:   citationLabel(sc.Chunk),
			Page:    page,
			Excerpt: buildExcerpt(sc.Chunk.Text, 280),
		})
	}
	return out
}

// buildCitationsFromText narrows buildCitations to the chunks whose label
// (e.g. "[Demo: two_counters]") appears literally in the LLM's reply. Without
// this filter the UI shows every retrieved chunk as a citation, even when the
// model never referenced it — which hides which sources actually grounded the
// answer and turns retrieval noise into apparent evidence.
func buildCitationsFromText(text string, retrieved []rag.ScoredChunk) []Citation {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	all := buildCitations(retrieved)
	out := make([]Citation, 0, len(all))
	seen := make(map[string]bool)
	for _, c := range all {
		if seen[c.Label] {
			continue
		}
		if strings.Contains(text, c.Label) {
			out = append(out, c)
			seen[c.Label] = true
		}
	}
	return out
}

func toLLMMessages(in []promptMessage) []llm.Message {
	out := make([]llm.Message, len(in))
	for i, m := range in {
		out[i] = llm.Message{Role: llm.Role(m.Role), Content: m.Content}
	}
	return out
}

func shorten(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// buildExcerpt returns a UI-friendly snippet of text capped at n runes. It
// collapses internal whitespace and tries to end at a word boundary so the
// student sees a clean fragment rather than a mid-word cut.
func buildExcerpt(text string, n int) string {
	if n <= 0 {
		return ""
	}
	clean := strings.Join(strings.Fields(text), " ")
	r := []rune(clean)
	if len(r) <= n {
		return clean
	}
	cut := r[:n]
	for i := len(cut) - 1; i > n/2; i-- {
		if cut[i] == ' ' {
			cut = cut[:i]
			break
		}
	}
	return strings.TrimRight(string(cut), " ,.;:") + "…"
}

// utf8RuneCount avoids importing unicode/utf8 just for one call.
func utf8RuneCount(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}


