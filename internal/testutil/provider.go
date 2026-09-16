// Package testutil reúne fakes determinísticos compartilhados pelos testes do harness.
package testutil

import (
	"context"
	"errors"

	"rmarquespaixao/ia-harness/harness"
)

// ErrScriptExhausted indica que o roteiro do ScriptedProvider terminou sem step disponível.
var ErrScriptExhausted = errors.New("testutil: script de provider esgotado")

// ProviderStep é um passo do roteiro: resposta completa ou erro injetado.
type ProviderStep struct {
	Text       string
	ToolCalls  []harness.ToolCall
	Usage      harness.Usage
	StopReason harness.StopReason
	Err        error
}

// ScriptedProvider devolve respostas determinísticas na ordem do roteiro.
type ScriptedProvider struct {
	Steps    []ProviderStep
	Calls    int
	Closed   bool
	Requests []harness.ChatRequest
}

// Chat consome o próximo step, emite o texto em fragmentos e devolve a resposta canônica.
func (p *ScriptedProvider) Chat(_ context.Context, req harness.ChatRequest, onText func(string)) (harness.ChatResponse, error) {
	p.Requests = append(p.Requests, req)
	p.Calls++
	if p.Calls > len(p.Steps) {
		return harness.ChatResponse{}, ErrScriptExhausted
	}
	step := p.Steps[p.Calls-1]
	if step.Err != nil {
		return harness.ChatResponse{}, step.Err
	}
	emitText(step.Text, onText)
	return harness.ChatResponse{
		Message: harness.Message{
			Role:  harness.RoleAssistant,
			Parts: []harness.Part{{Kind: harness.PartText, Text: step.Text}},
		},
		ToolCalls:  step.ToolCalls,
		Usage:      step.Usage,
		StopReason: step.StopReason,
	}, nil
}

// emitText divide o texto em duas metades quando há mais de um caractere; caso contrário emite uma vez.
func emitText(text string, onText func(string)) {
	if onText == nil {
		return
	}
	if len(text) <= 1 {
		onText(text)
		return
	}
	half := len(text) / 2
	onText(text[:half])
	onText(text[half:])
}

// Close marca o provider como fechado.
func (p *ScriptedProvider) Close() error {
	p.Closed = true
	return nil
}

var _ harness.Provider = (*ScriptedProvider)(nil)
