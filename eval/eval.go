// Package eval roda cenários de avaliação/regressão de um harness: executa um
// turno por caso, coleta os eventos e aplica uma verificação (feature 016). É
// determinístico e usa o que o host injetou (provider/tools fakes nos testes).
package eval

import (
	"context"
	"fmt"

	"rmarquespaixao/ia-harness/harness"
)

// Case é um cenário: entrada do usuário e verificação do resultado.
type Case struct {
	// Name identifica o caso no relatório.
	Name string
	// Input é a mensagem do usuário.
	Input []harness.Part
	// Model opcional; vazio usa o DefaultModel do harness.
	Model string
	// OutputSchema opcional (structured output).
	OutputSchema []byte
	// Check valida o resultado e os eventos; nil = só exige turno sem erro.
	Check func(res harness.TurnResult, rec *Recorder) error
}

// Recorder captura os eventos do turno para as verificações.
type Recorder struct {
	harness.NopHandler
	Deltas      []string
	ToolCalls   []harness.ToolCallEvent
	ToolResults []harness.ToolResultEvent
	Errors      []harness.ErrorEvent
}

func (r *Recorder) TextDelta(_ context.Context, ev harness.TextDelta) {
	r.Deltas = append(r.Deltas, ev.Text)
}
func (r *Recorder) ToolCall(_ context.Context, ev harness.ToolCallEvent) {
	r.ToolCalls = append(r.ToolCalls, ev)
}
func (r *Recorder) ToolResult(_ context.Context, ev harness.ToolResultEvent) {
	r.ToolResults = append(r.ToolResults, ev)
}
func (r *Recorder) Error(_ context.Context, ev harness.ErrorEvent) {
	r.Errors = append(r.Errors, ev)
}

// Text concatena o texto emitido (deltas + saída final).
func (r *Recorder) Text() string {
	out := ""
	for _, d := range r.Deltas {
		out += d
	}
	return out
}

// UsedTool informa se a tool foi executada com sucesso.
func (r *Recorder) UsedTool(name string) bool {
	for _, tr := range r.ToolResults {
		if tr.Tool == name && tr.Status == harness.StatusOK {
			return true
		}
	}
	return false
}

// Result é o desfecho de um caso.
type Result struct {
	Case   string
	Passed bool
	Err    error
}

// Report agrega os resultados.
type Report struct {
	Results []Result
	Passed  int
	Failed  int
}

// OK informa se todos os casos passaram.
func (r Report) OK() bool { return r.Failed == 0 }

// Run executa os casos contra o harness e devolve o relatório. userID/agentID
// identificam o dono das sessões (isolamento — FR-022).
func Run(ctx context.Context, h *harness.Harness, userID, agentID string, cases []Case) Report {
	report := Report{Results: make([]Result, 0, len(cases))}
	for _, c := range cases {
		rec := &Recorder{}
		req := harness.RunRequest{
			UserID:       userID,
			AgentID:      agentID,
			Model:        c.Model,
			Input:        c.Input,
			OutputSchema: c.OutputSchema,
		}
		res, err := h.Run(ctx, req, rec)
		result := Result{Case: c.Name}
		switch {
		case err != nil:
			result.Err = err
		case c.Check != nil:
			result.Err = c.Check(res, rec)
		}
		result.Passed = result.Err == nil
		if result.Passed {
			report.Passed++
		} else {
			report.Failed++
		}
		report.Results = append(report.Results, result)
	}
	return report
}

// ContainsText verifica que o texto (deltas) contém o trecho.
func ContainsText(sub string) func(harness.TurnResult, *Recorder) error {
	return func(_ harness.TurnResult, rec *Recorder) error {
		if !contains(rec.Text(), sub) {
			return fmt.Errorf("texto não contém %q", sub)
		}
		return nil
	}
}

// UsedTool verifica que a tool foi executada com sucesso.
func UsedTool(name string) func(harness.TurnResult, *Recorder) error {
	return func(_ harness.TurnResult, rec *Recorder) error {
		if !rec.UsedTool(name) {
			return fmt.Errorf("tool %q não executou com sucesso", name)
		}
		return nil
	}
}

func contains(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
