// Package log implementa o AuditSink que materializa cada AuditEvent como
// registro JSON estruturado no logger injetado pelo host (R13/FR-023). O
// resumo sai em nível Info; os payloads redigidos saem apenas em nível Debug,
// para que a trilha operacional não carregue conteúdo sensível por padrão.
// O slog não devolve erro de escrita, então Emit sempre devolve nil e falhas
// de log nunca derrubam o turno.
package log

import (
	"context"
	"log/slog"

	"github.com/rmarquespaixao-eng/ia-harness/harness"
	"github.com/rmarquespaixao-eng/ia-harness/internal/platform/trace"
)

// Mensagens estáveis dos dois registros emitidos por evento.
const (
	msgResumo  = "audit"
	msgPayload = "audit_payload"
)

// Sink escreve eventos de auditoria como JSON estruturado via log/slog.
type Sink struct {
	logger *slog.Logger
}

// New cria o sink sobre o logger do host; logger nulo cai no default do slog.
func New(logger *slog.Logger) *Sink {
	if logger == nil {
		logger = slog.Default()
	}
	return &Sink{logger: logger}
}

// Emit registra o evento: resumo em Info e, em Debug, os payloads já
// redigidos/truncados pelo núcleo (FR-024). O trace_id vem do contexto do host
// (FR-026) e fica vazio quando ausente.
func (s *Sink) Emit(ctx context.Context, ev harness.AuditEvent) error {
	traceID, _ := trace.IDFromContext(ctx)

	s.logger.InfoContext(ctx, msgResumo,
		slog.String("kind", string(ev.Kind)),
		slog.String("status", string(ev.Status)),
		slog.String("provider", texto(ev.Provider)),
		slog.String("model", texto(ev.Model)),
		slog.String("tool", texto(ev.Tool)),
		slog.Int("latency_ms", ev.LatencyMs),
		slog.Int64("input_tokens", inteiro(ev.InputTokens)),
		slog.Int64("output_tokens", inteiro(ev.OutputTokens)),
		slog.Bool("estimated", ev.Estimated),
		slog.Int64("cost_micros", inteiro(ev.CostMicros)),
		slog.String("currency", texto(ev.Currency)),
		slog.String("trace_id", traceID),
	)

	s.logger.DebugContext(ctx, msgPayload,
		slog.String("id", ev.Id),
		slog.String("kind", string(ev.Kind)),
		slog.String("args_redacted", texto(ev.ArgsRedacted)),
		slog.String("result_redacted", texto(ev.ResultRedacted)),
	)

	return nil
}

// texto devolve o valor apontado ou string vazia quando o campo é ausente.
func texto(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// inteiro devolve o valor apontado ou zero quando o campo é ausente.
func inteiro(v *int) int64 {
	if v == nil {
		return 0
	}
	return int64(*v)
}
