package policy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestEvaluatePolicy cobre o motor puro de política (CU-HAR-3/FR-016): default
// deny/allow, deny vence, allowlist com glob, read_only, confirmação e herança
// de modo, sempre case sensitive.
func TestEvaluatePolicy(t *testing.T) {
	saldo := Tool{Name: "saldo", Namespace: "financeiro"}
	excluir := Tool{Name: "excluir_transacao", Namespace: "financeiro"}
	consulta := Tool{Name: "consulta", Namespace: "outro"}

	tests := []struct {
		name        string
		policy      PolicyConfig
		agentID     string
		tool        Tool
		wantAllow   bool
		wantConfirm bool
	}{
		{
			name:    "default vazio nega (deny implícito)",
			policy:  PolicyConfig{},
			agentID: "a",
			tool:    saldo,
		},
		{
			name:    "default deny nega",
			policy:  PolicyConfig{Default: PolicyDeny},
			agentID: "a",
			tool:    saldo,
		},
		{
			name:      "default allow permite",
			policy:    PolicyConfig{Default: PolicyAllow},
			agentID:   "a",
			tool:      saldo,
			wantAllow: true,
		},
		{
			name: "agente inexistente cai no default deny",
			policy: PolicyConfig{
				Default: PolicyDeny,
				Agents:  map[string]AgentPolicy{"a": {Mode: PolicyAllow}},
			},
			agentID: "b",
			tool:    saldo,
		},
		{
			name: "agente inexistente cai no default allow",
			policy: PolicyConfig{
				Default: PolicyAllow,
				Agents:  map[string]AgentPolicy{"a": {Mode: PolicyDeny}},
			},
			agentID:   "b",
			tool:      saldo,
			wantAllow: true,
		},
		{
			name: "deny_tools vence a allowlist",
			policy: PolicyConfig{Agents: map[string]AgentPolicy{"a": {
				Mode:       PolicyAllow,
				AllowTools: []string{"financeiro.*"},
				DenyTools:  []string{"financeiro.excluir*"},
			}}},
			agentID: "a",
			tool:    excluir,
		},
		{
			name: "allowlist com glob de servidor casa",
			policy: PolicyConfig{Agents: map[string]AgentPolicy{"a": {
				Mode:       PolicyAllow,
				AllowTools: []string{"financeiro.*"},
			}}},
			agentID:   "a",
			tool:      saldo,
			wantAllow: true,
		},
		{
			name: "allowlist com glob de servidor nega outro servidor",
			policy: PolicyConfig{Agents: map[string]AgentPolicy{"a": {
				Mode:       PolicyAllow,
				AllowTools: []string{"financeiro.*"},
			}}},
			agentID: "a",
			tool:    consulta,
		},
		{
			name: "allowlist pelo nome puro casa tool com namespace",
			policy: PolicyConfig{Agents: map[string]AgentPolicy{"a": {
				Mode:       PolicyAllow,
				AllowTools: []string{"saldo"},
			}}},
			agentID:   "a",
			tool:      saldo,
			wantAllow: true,
		},
		{
			name: "allowlist é case sensitive",
			policy: PolicyConfig{Agents: map[string]AgentPolicy{"a": {
				Mode:       PolicyAllow,
				AllowTools: []string{"Saldo"},
			}}},
			agentID: "a",
			tool:    saldo,
		},
		{
			name: "allowlist com glob estrela permite tudo",
			policy: PolicyConfig{Agents: map[string]AgentPolicy{"a": {
				Mode:       PolicyAllow,
				AllowTools: []string{"*"},
			}}},
			agentID:   "a",
			tool:      consulta,
			wantAllow: true,
		},
		{
			name: "allowlist malformada nunca casa",
			policy: PolicyConfig{Agents: map[string]AgentPolicy{"a": {
				Mode:       PolicyAllow,
				AllowTools: []string{"["},
			}}},
			agentID: "a",
			tool:    saldo,
		},
		{
			name: "read_only permite tool com override de leitura",
			policy: PolicyConfig{Agents: map[string]AgentPolicy{"a": {
				Mode:      PolicyReadOnly,
				Overrides: map[string]ToolPolicy{"financeiro.saldo": {ReadOnly: true}},
			}}},
			agentID:   "a",
			tool:      saldo,
			wantAllow: true,
		},
		{
			name: "read_only permite override pela chave do nome puro",
			policy: PolicyConfig{Agents: map[string]AgentPolicy{"a": {
				Mode:      PolicyReadOnly,
				Overrides: map[string]ToolPolicy{"saldo": {ReadOnly: true}},
			}}},
			agentID:   "a",
			tool:      saldo,
			wantAllow: true,
		},
		{
			name: "read_only nega tool sem override",
			policy: PolicyConfig{Agents: map[string]AgentPolicy{"a": {
				Mode:      PolicyReadOnly,
				Overrides: map[string]ToolPolicy{"financeiro.saldo": {ReadOnly: true}},
			}}},
			agentID: "a",
			tool:    excluir,
		},
		{
			name: "read_only nega override explicitamente falso",
			policy: PolicyConfig{Agents: map[string]AgentPolicy{"a": {
				Mode:      PolicyReadOnly,
				Overrides: map[string]ToolPolicy{"financeiro.saldo": {ReadOnly: false}},
			}}},
			agentID: "a",
			tool:    saldo,
		},
		{
			name: "read_only interseca a allowlist",
			policy: PolicyConfig{Agents: map[string]AgentPolicy{"a": {
				Mode:       PolicyReadOnly,
				AllowTools: []string{"outro.*"},
				Overrides:  map[string]ToolPolicy{"financeiro.saldo": {ReadOnly: true}},
			}}},
			agentID: "a",
			tool:    saldo,
		},
		{
			name: "modo vazio do agente herda default deny",
			policy: PolicyConfig{Agents: map[string]AgentPolicy{"a": {
				AllowTools: []string{"*"},
			}}},
			agentID: "a",
			tool:    saldo,
		},
		{
			name: "modo vazio do agente herda default allow",
			policy: PolicyConfig{
				Default: PolicyAllow,
				Agents:  map[string]AgentPolicy{"a": {}},
			},
			agentID:   "a",
			tool:      consulta,
			wantAllow: true,
		},
		{
			name: "modo vazio herda default allow e respeita a allowlist",
			policy: PolicyConfig{
				Default: PolicyAllow,
				Agents: map[string]AgentPolicy{"a": {
					AllowTools: []string{"financeiro.*"},
				}},
			},
			agentID: "a",
			tool:    consulta,
		},
		{
			name: "modo deny do agente vence default allow",
			policy: PolicyConfig{
				Default: PolicyAllow,
				Agents: map[string]AgentPolicy{"a": {
					Mode:       PolicyDeny,
					AllowTools: []string{"*"},
				}},
			},
			agentID: "a",
			tool:    saldo,
		},
		{
			name: "confirm_tools pela lista marca confirmação",
			policy: PolicyConfig{Agents: map[string]AgentPolicy{"a": {
				Mode:         PolicyAllow,
				ConfirmTools: []string{"excluir_transacao"},
			}}},
			agentID:     "a",
			tool:        excluir,
			wantAllow:   true,
			wantConfirm: true,
		},
		{
			name: "confirm_tools com glob de servidor marca confirmação",
			policy: PolicyConfig{Agents: map[string]AgentPolicy{"a": {
				Mode:         PolicyAllow,
				ConfirmTools: []string{"financeiro.*"},
			}}},
			agentID:     "a",
			tool:        saldo,
			wantAllow:   true,
			wantConfirm: true,
		},
		{
			name: "override Confirm marca confirmação",
			policy: PolicyConfig{Agents: map[string]AgentPolicy{"a": {
				Mode:      PolicyAllow,
				Overrides: map[string]ToolPolicy{"financeiro.saldo": {Confirm: true}},
			}}},
			agentID:     "a",
			tool:        saldo,
			wantAllow:   true,
			wantConfirm: true,
		},
		{
			name: "confirm_tools que não casa não marca confirmação",
			policy: PolicyConfig{Agents: map[string]AgentPolicy{"a": {
				Mode:         PolicyAllow,
				ConfirmTools: []string{"outro.*"},
			}}},
			agentID:   "a",
			tool:      saldo,
			wantAllow: true,
		},
		{
			name: "deny vence a exigência de confirmação",
			policy: PolicyConfig{Agents: map[string]AgentPolicy{"a": {
				Mode:         PolicyAllow,
				DenyTools:    []string{"financeiro.excluir*"},
				ConfirmTools: []string{"financeiro.excluir*"},
			}}},
			agentID: "a",
			tool:    excluir,
		},
		{
			name: "tool fora da allowlist não pede confirmação",
			policy: PolicyConfig{Agents: map[string]AgentPolicy{"a": {
				Mode:         PolicyAllow,
				AllowTools:   []string{"outro.*"},
				ConfirmTools: []string{"financeiro.*"},
			}}},
			agentID: "a",
			tool:    saldo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := evaluatePolicy(tt.policy, tt.agentID, tt.tool)

			// Assert
			assert.Equal(t, tt.wantAllow, got.Allowed)
			assert.Equal(t, tt.wantConfirm, got.RequiresConfirmation)
			if !tt.wantAllow || tt.wantConfirm {
				assert.NotEmpty(t, got.Reason, "negativa e confirmação carregam motivo")
			} else {
				assert.Empty(t, got.Reason, "permissão direta não carrega motivo")
			}
		})
	}
}

// TestPolicyToolIdempotent cobre o acessor de idempotência usado em retry/
// fallback (FR-014/T051): o override habilita a repetição, mas não desabilita a
// declaração publicada pela tool.
func TestPolicyToolIdempotent(t *testing.T) {
	saldo := Tool{Name: "saldo", Namespace: "financeiro"}

	tests := []struct {
		name    string
		policy  PolicyConfig
		agentID string
		tool    Tool
		want    bool
	}{
		{
			name:    "sem política usa a declaração da tool",
			policy:  PolicyConfig{},
			agentID: "a",
			tool:    Tool{Name: "saldo", Namespace: "financeiro", Idempotent: true},
			want:    true,
		},
		{
			name:    "sem política e tool não declarada",
			policy:  PolicyConfig{},
			agentID: "a",
			tool:    saldo,
			want:    false,
		},
		{
			name: "override habilita tool não declarada",
			policy: PolicyConfig{Agents: map[string]AgentPolicy{"a": {
				Overrides: map[string]ToolPolicy{"financeiro.saldo": {Idempotent: true}},
			}}},
			agentID: "a",
			tool:    saldo,
			want:    true,
		},
		{
			name: "agente sem override usa a declaração da tool",
			policy: PolicyConfig{Agents: map[string]AgentPolicy{"a": {
				Mode: PolicyAllow,
			}}},
			agentID: "a",
			tool:    Tool{Name: "saldo", Namespace: "financeiro", Idempotent: true},
			want:    true,
		},
		{
			name: "override parcial (só confirm) não zera a idempotência publicada",
			policy: PolicyConfig{Agents: map[string]AgentPolicy{"a": {
				Overrides: map[string]ToolPolicy{"financeiro.saldo": {Confirm: true}},
			}}},
			agentID: "a",
			tool:    Tool{Name: "saldo", Namespace: "financeiro", Idempotent: true},
			want:    true,
		},
		{
			name: "override falso não desabilita a declaração da tool",
			policy: PolicyConfig{Agents: map[string]AgentPolicy{"a": {
				Overrides: map[string]ToolPolicy{"financeiro.saldo": {Idempotent: false}},
			}}},
			agentID: "a",
			tool:    Tool{Name: "saldo", Namespace: "financeiro", Idempotent: true},
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := toolIdempotent(tt.policy, tt.agentID, tt.tool)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}
