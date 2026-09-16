// Package trace propaga o identificador de correlação (trace_id) pelo
// context.Context, gerando um UUID v4 quando ausente.
package trace

import (
	"context"

	"github.com/google/uuid"
)

// contextKey é o tipo privado usado como chave do trace_id no contexto.
type contextKey struct{}

// traceIDKey é a chave única do trace_id neste pacote.
var traceIDKey contextKey

// WithID devolve um contexto que carrega o trace_id informado.
func WithID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, traceIDKey, id)
}

// IDFromContext extrai o trace_id do contexto. Devolve ok=false quando não há
// id ou quando ele é vazio.
func IDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(traceIDKey).(string)
	if !ok || id == "" {
		return "", false
	}
	return id, true
}

// EnsureID devolve o trace_id já presente no contexto ou gera um UUID v4 novo.
// O contexto devolvido sempre carrega o identificador resultante.
func EnsureID(ctx context.Context) (context.Context, string) {
	if id, ok := IDFromContext(ctx); ok {
		return ctx, id
	}

	id := uuid.NewString()
	return WithID(ctx, id), id
}
