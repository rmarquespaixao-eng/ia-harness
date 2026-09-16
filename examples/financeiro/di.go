// Command financeiro-example monta o wiring completo do harness para o host
// financeiro (T081, contracts/financeiro-integration.md): é documentação
// executável — apenas MONTA a Config (e a valida com harness.New), sem chamar
// Run/ResolveConfirmation, que pertencem ao financeiro-api-v2. Nenhum segredo
// literal: todas as credenciais entram por referência env:, resolvidas sob
// demanda (FR-004).
//
// O host troca as portas de exemplo pelas reais: SessionStore no Postgres,
// AuditSink na trilha do host, e o Handler de eventos pelo SSE da UI.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	auditlog "rmarquespaixao/ia-harness/adapters/audit/log"
	"rmarquespaixao/ia-harness/adapters/mcpclient"
	"rmarquespaixao/ia-harness/adapters/provider/anthropic"
	"rmarquespaixao/ia-harness/adapters/provider/openai"
	openairesponses "rmarquespaixao/ia-harness/adapters/provider/openai_responses"
	"rmarquespaixao/ia-harness/adapters/session/memory"
	"rmarquespaixao/ia-harness/harness"
)

// Endpoints, aliases e referências de credencial do host (sem valores).
const (
	mcpEndpoint     = "https://homolog-api-financeiro.homelab-cloud.com/mcp"
	mcpCredential   = "env:FINANCEIRO_MCP_KEY"
	providerBaseURL = "https://openrouter.ai/api/v1"
	providerKeyRef  = "env:OPENROUTER_API_KEY"
	agentID         = "financeiro-chat"
	modelAlias      = "financeiro-default"
	modelName       = "openai/gpt-4o-mini"

	// OpenCode Zen/Go (feature 004): as três famílias de fio do mesmo gateway.
	// chat/completions e messages já são cobertos por config; responses usa o
	// adapter dedicado. O Go pede User-Agent próprio e x-opencode-session.
	zenGoBaseURL        = "https://opencode.ai/zen/go/v1"
	zenGoMessagesURL    = "https://opencode.ai/zen/go"
	zenGoCredentialRef  = "env:OPENCODE_GO_API_KEY"
	zenGoUserAgent      = "ia-harness-example/0.1"
	zenGoSessionHeader  = "x-opencode-session"
	zenGoResponsesAlias = "go-gpt-luna"
	zenGoResponsesModel = "gpt-5.6-luna"
	zenGoChatAlias      = "go-kimi"
	zenGoChatModel      = "kimi-k3"
	zenGoMessagesAlias  = "go-qwen"
	zenGoMessagesModel  = "qwen3.7-max"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	credentials := envCredentials{}

	cfg := harness.Config{
		Providers: map[string]harness.Provider{
			"openai": openai.New(openai.Config{
				BaseURL:       providerBaseURL,
				CredentialRef: providerKeyRef,
				DefaultModel:  modelName,
			}, openai.Deps{Credentials: credentials, Logger: logger}),
			// Família Chat Completions do Zen/Go (GLM, Kimi, DeepSeek, MiniMax…).
			"opencode-go-chat": openai.New(openai.Config{
				BaseURL:       zenGoBaseURL,
				CredentialRef: zenGoCredentialRef,
				DefaultModel:  zenGoChatModel,
				Headers:       map[string]string{"User-Agent": zenGoUserAgent},
				SessionHeader: zenGoSessionHeader,
			}, openai.Deps{Credentials: credentials, Logger: logger}),
			// Família Messages do Zen/Go (Claude, Qwen…).
			"opencode-go-messages": anthropic.New(anthropic.Config{
				BaseURL:       zenGoMessagesURL,
				CredentialRef: zenGoCredentialRef,
				DefaultModel:  zenGoMessagesModel,
				Headers:       map[string]string{"User-Agent": zenGoUserAgent},
				SessionHeader: zenGoSessionHeader,
			}, anthropic.Deps{Credentials: credentials, Logger: logger}),
			// Família Responses do Zen/Go (GPT/Grok/Muse — inclui gpt-5.6-luna).
			"opencode-go-responses": openairesponses.New(openairesponses.Config{
				BaseURL:       zenGoBaseURL,
				CredentialRef: zenGoCredentialRef,
				DefaultModel:  zenGoResponsesModel,
				Headers:       map[string]string{"User-Agent": zenGoUserAgent},
				SessionHeader: zenGoSessionHeader,
			}, openairesponses.Deps{Credentials: credentials, Logger: logger}),
		},
		Models: map[string]harness.ModelProfile{
			modelAlias: {
				Provider: "openai",
				Model:    modelName,
				Capabilities: harness.Capabilities{
					ToolCalling:      true,
					Streaming:        true,
					MaxContextTokens: 128000,
					MaxOutputTokens:  4096,
				},
			},
			zenGoResponsesAlias: {
				Provider: "opencode-go-responses",
				Model:    zenGoResponsesModel,
				Capabilities: harness.Capabilities{
					ToolCalling:      true,
					Streaming:        true,
					MaxContextTokens: 272000,
					MaxOutputTokens:  8192,
				},
			},
			zenGoChatAlias: {
				Provider:     "opencode-go-chat",
				Model:        zenGoChatModel,
				Capabilities: harness.Capabilities{ToolCalling: true, Streaming: true, MaxContextTokens: 256000, MaxOutputTokens: 8192},
			},
			zenGoMessagesAlias: {
				Provider:     "opencode-go-messages",
				Model:        zenGoMessagesModel,
				Capabilities: harness.Capabilities{ToolCalling: true, Streaming: true, MaxContextTokens: 256000, MaxOutputTokens: 8192},
			},
		},
		Tools: []harness.ToolSource{
			mcpclient.New(mcpclient.Config{
				Name:          "financeiro",
				Endpoint:      mcpEndpoint,
				CredentialRef: mcpCredential,
				ToolTimeout:   2 * time.Minute,
			}, mcpclient.Deps{Credentials: credentials, Logger: logger}),
		},
		Policy: harness.PolicyConfig{
			Default: harness.PolicyDeny,
			Agents: map[string]harness.AgentPolicy{
				agentID: {
					Mode:       harness.PolicyAllow,
					AllowTools: []string{"financeiro.*"},
					ConfirmTools: []string{
						"financeiro.excluir_transacao",
						"financeiro.excluir_transacoes_em_lote",
						"financeiro.transferir_transacoes",
						"financeiro.pagar_fatura",
						"financeiro.executar_recorrencia_agora",
					},
				},
			},
		},
		Pricing: harness.Pricing{
			Currency:                    "BRL",
			InputPriceMicrosPerMillion:  500_000,
			OutputPriceMicrosPerMillion: 1_500_000,
			BytesPerToken:               4,
		},
		Context: harness.ContextPolicy{
			MaxTokens: 120_000,
			Strategy:  harness.StrategyTruncateOldest,
		},
		DefaultModel: modelAlias,
		Credentials:  credentials,
		Sessions:     memory.New(),
		Logger:       logger,
		Audit:        auditlog.New(logger),
	}

	// Falha rápida: valida o wiring sem abrir rede nem resolver credencial.
	h, err := harness.New(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "financeiro-example:", err)
		os.Exit(1)
	}
	defer func() { _ = h.Close() }()

	fmt.Println("wiring do harness financeiro válido (exemplo de DI; Run pertence ao host)")
}

// envCredentials resolve apenas referências env:NOME; o host substitui pela
// leitura da chave do usuário/agente já autenticado na API (contracts/
// financeiro-integration.md).
type envCredentials struct{}

// Resolve implementa harness.CredentialProvider.
func (envCredentials) Resolve(_ context.Context, ref string) (string, error) {
	name, ok := strings.CutPrefix(ref, "env:")
	if !ok || name == "" {
		return "", fmt.Errorf("credential: %q inválida; este exemplo resolve apenas env:NOME", ref)
	}
	value := os.Getenv(name)
	if value == "" {
		return "", fmt.Errorf("credential: variável de ambiente %q não definida", name)
	}
	return value, nil
}
