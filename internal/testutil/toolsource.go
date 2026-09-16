package testutil

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"rmarquespaixao/ia-harness/harness"
)

// MemToolSource é um ToolSource em memória: catálogo fixo e resultados/erros/progresso roteirizados.
type MemToolSource struct {
	Tools    []harness.Tool
	Results  map[string]harness.ToolResult
	Errors   map[string]error
	Progress map[string][]harness.ProgressUpdate
	Calls    []harness.ToolCall
	Closed   bool

	mu sync.Mutex
}

// List devolve o catálogo configurado.
func (s *MemToolSource) List(_ context.Context) ([]harness.Tool, error) {
	return s.Tools, nil
}

// Call registra a chamada, emite o progresso roteirizado e devolve o resultado ou erro da tool.
func (s *MemToolSource) Call(_ context.Context, name string, args json.RawMessage, onProgress func(harness.ProgressUpdate)) (harness.ToolResult, error) {
	s.mu.Lock()
	s.Calls = append(s.Calls, harness.ToolCall{Name: name, Args: args})
	s.mu.Unlock()
	if onProgress != nil {
		for _, update := range s.Progress[name] {
			onProgress(update)
		}
	}
	if err, ok := s.Errors[name]; ok && err != nil {
		return harness.ToolResult{}, err
	}
	if result, ok := s.Results[name]; ok {
		return result, nil
	}
	return harness.ToolResult{}, fmt.Errorf("testutil: tool %q não registrada", name)
}

// Close marca a fonte como fechada.
func (s *MemToolSource) Close() error {
	s.Closed = true
	return nil
}

var _ harness.ToolSource = (*MemToolSource)(nil)
