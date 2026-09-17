package agents

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	core "github.com/rmarquespaixao-eng/ia-harness/internal/core"
)

// fakeRunner é um AgentRunner de teste que registra a chamada e devolve um
// resultado/erro pré-programados.
type fakeRunner struct {
	calls  int
	gotCtx context.Context
	gotReq core.SubAgentRequest
	result core.SubAgentResult
	err    error
}

func (f *fakeRunner) RunSubAgent(ctx context.Context, req core.SubAgentRequest) (core.SubAgentResult, error) {
	f.calls++
	f.gotCtx = ctx
	f.gotReq = req
	return f.result, f.err
}

// firstText devolve o primeiro conteúdo textual do resultado.
func firstText(res core.ToolResult) string {
	if len(res.Content) == 0 {
		return ""
	}
	return res.Content[0].Text
}

// TestSourceList cobre a fonte sem agentes e a fonte com agentes publicando a
// única tool de delegação.
func TestSourceList(t *testing.T) {
	tests := []struct {
		name    string
		agents  map[string]core.AgentSpec
		wantNil bool
		wantLen int
	}{
		{
			name:    "sem agentes devolve nil",
			agents:  nil,
			wantNil: true,
		},
		{
			name:    "com agentes devolve a tool de delegação",
			agents:  map[string]core.AgentSpec{"pesquisa": {}},
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			src := NewSource(nil, tt.agents, 0)

			// Act
			tools, err := src.List(context.Background())

			// Assert
			require.NoError(t, err)
			if tt.wantNil {
				assert.Nil(t, tools)
				return
			}
			require.Len(t, tools, tt.wantLen)
			assert.Equal(t, DelegateName, tools[0].Name)
		})
	}
}

// TestSourceCall_ErrosDeUso cobre os caminhos em que a delegação é recusada
// como resultado de erro, sem chamar o runner.
func TestSourceCall_ErrosDeUso(t *testing.T) {
	tests := []struct {
		name       string
		agents     map[string]core.AgentSpec
		callCtx    CallContext
		args       json.RawMessage
		wantInText string
	}{
		{
			name:       "agente desconhecido",
			agents:     map[string]core.AgentSpec{"conhecido": {}},
			args:       json.RawMessage(`{"agent":"x","task":"y"}`),
			wantInText: "não configurado",
		},
		{
			name:       "task vazia é erro de uso",
			agents:     map[string]core.AgentSpec{"x": {}},
			args:       json.RawMessage(`{"agent":"x","task":""}`),
			wantInText: "task é obrigatório",
		},
		{
			name:       "profundidade máxima estourada",
			agents:     map[string]core.AgentSpec{"x": {}},
			callCtx:    CallContext{Depth: 1},
			args:       json.RawMessage(`{"agent":"x","task":"y"}`),
			wantInText: "profundidade máxima",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			runner := &fakeRunner{}
			src := NewSource(runner, tt.agents, 1)
			ctx := WithCallContext(context.Background(), tt.callCtx)

			// Act
			res, err := src.Call(ctx, DelegateName, tt.args, nil)

			// Assert
			require.NoError(t, err, "erro de uso nunca é erro de transporte")
			assert.True(t, res.IsError)
			assert.Contains(t, firstText(res), tt.wantInText)
			assert.Zero(t, runner.calls, "o runner não deve ser chamado")
		})
	}
}

// TestSourceCall_Valido cobre a delegação bem-sucedida repassando o pedido ao
// runner e convertendo o texto do sub-turno no resultado.
func TestSourceCall_Valido(t *testing.T) {
	// Arrange
	runner := &fakeRunner{result: core.SubAgentResult{
		SessionID: "child-session",
		Output:    []core.Part{{Kind: core.PartText, Text: "resposta do sub-agente"}},
		State:     core.SessionActive,
	}}
	src := NewSource(runner, map[string]core.AgentSpec{"pesquisa": {}}, 1)
	ctx := WithCallContext(context.Background(), CallContext{
		Depth:           0,
		UserID:          "u1",
		ParentSessionID: "s1",
	})

	// Act
	res, err := src.Call(ctx, DelegateName, json.RawMessage(`{"agent":"pesquisa","task":"resuma o mês"}`), nil)

	// Assert
	require.NoError(t, err)
	assert.False(t, res.IsError)
	assert.Equal(t, core.ResultText, res.Content[0].Kind)
	assert.Equal(t, "resposta do sub-agente", firstText(res))

	require.Equal(t, 1, runner.calls)
	assert.Equal(t, "pesquisa", runner.gotReq.AgentID)
	assert.Equal(t, "u1", runner.gotReq.UserID)
	assert.Equal(t, "s1", runner.gotReq.ParentSessionID)
	require.Len(t, runner.gotReq.Input, 1)
	assert.Equal(t, core.PartText, runner.gotReq.Input[0].Kind)
	assert.Equal(t, "resuma o mês", runner.gotReq.Input[0].Text)

	child := FromCallContext(runner.gotCtx)
	assert.Equal(t, 1, child.Depth)
	assert.Equal(t, "u1", child.UserID)
	assert.Equal(t, "s1", child.ParentSessionID)
}

// TestSourceCall_RunnerComErro garante que a falha do sub-agente vira resultado
// de erro, não erro de transporte.
func TestSourceCall_RunnerComErro(t *testing.T) {
	// Arrange
	runner := &fakeRunner{err: errors.New("boom")}
	src := NewSource(runner, map[string]core.AgentSpec{"x": {}}, 1)

	// Act
	res, err := src.Call(context.Background(), DelegateName, json.RawMessage(`{"agent":"x","task":"y"}`), nil)

	// Assert
	require.NoError(t, err)
	assert.True(t, res.IsError)
	assert.Contains(t, firstText(res), "falha no sub-agente")
	assert.Contains(t, firstText(res), "boom")
	assert.Equal(t, 1, runner.calls)
}

// TestSourceCall_MaxDepthNormalizado cobre maxDepth ≤ 0 normalizado para 1: o
// topo em Depth 0 delega; quem já está em Depth 1 não delega mais.
func TestSourceCall_MaxDepthNormalizado(t *testing.T) {
	t.Run("topo em Depth 0 pode delegar", func(t *testing.T) {
		// Arrange
		runner := &fakeRunner{result: core.SubAgentResult{Output: []core.Part{{Kind: core.PartText, Text: "ok"}}}}
		src := NewSource(runner, map[string]core.AgentSpec{"x": {}}, 0)

		// Act
		res, err := src.Call(context.Background(), DelegateName, json.RawMessage(`{"agent":"x","task":"y"}`), nil)

		// Assert
		require.NoError(t, err)
		assert.False(t, res.IsError)
		assert.Equal(t, 1, runner.calls)
	})

	t.Run("Depth 1 não pode delegar", func(t *testing.T) {
		// Arrange
		runner := &fakeRunner{}
		src := NewSource(runner, map[string]core.AgentSpec{"x": {}}, 0)
		ctx := WithCallContext(context.Background(), CallContext{Depth: 1})

		// Act
		res, err := src.Call(ctx, DelegateName, json.RawMessage(`{"agent":"x","task":"y"}`), nil)

		// Assert
		require.NoError(t, err)
		assert.True(t, res.IsError)
		assert.Contains(t, firstText(res), "profundidade máxima")
		assert.Zero(t, runner.calls)
	})
}
