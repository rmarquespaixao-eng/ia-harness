package budget

import "time"

// DefaultMaxIterations é o teto de iterações de um turno quando a config não
// define outro (FR-008).
const DefaultMaxIterations = 8

// TurnBudget limita iterações, tokens e tempo de parede de um turno.
type TurnBudget struct {
	maxIterations int
	iterations    int
	maxTokens     int64
	tokens        int64
	deadline      time.Time
	hasDeadline   bool
}

// New monta o orçamento do turno aplicando o default de iterações.
func New(b Budget, defaultIterations int, now time.Time) *TurnBudget {
	if defaultIterations <= 0 {
		defaultIterations = DefaultMaxIterations
	}
	return &TurnBudget{
		maxIterations: defaultIterations,
		maxTokens:     b.MaxTokens,
		hasDeadline:   b.MaxWall > 0,
		deadline:      now.Add(b.MaxWall),
	}
}

// CanIterate informa se ainda há iteração disponível (conta a próxima).
func (b *TurnBudget) CanIterate() bool { return b.iterations < b.maxIterations }

// StartIteration consome uma iteração.
func (b *TurnBudget) StartIteration() { b.iterations++ }

// AddUsage acumula o consumo da chamada de modelo no orçamento de tokens.
func (b *TurnBudget) AddUsage(u Usage) { b.tokens += u.InputTokens + u.OutputTokens }

// Exceeded devolve o motivo quando tokens ou tempo estouraram.
func (b *TurnBudget) Exceeded(now time.Time) (StopReason, bool) {
	if b.hasDeadline && !now.Before(b.deadline) {
		return StopBudget, true
	}
	if b.maxTokens > 0 && b.tokens >= b.maxTokens {
		return StopBudget, true
	}
	return "", false
}
