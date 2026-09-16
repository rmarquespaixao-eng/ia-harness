package engine

import (
	"context"
	"fmt"

	"github.com/rmarquespaixao-eng/ia-harness/internal/platform/clock"
)

// applyDefaults preenche os campos opcionais da configuração com os defaults do
// núcleo (constitution §2/§7): relógio do sistema, sink de auditoria no-op,
// redação em 16 KiB, estratégia truncate_oldest e premissa de 4 bytes/token.
func applyDefaults(c *Config) {
	if c.Clock == nil {
		c.Clock = clock.System{}
	}
	if c.Audit == nil {
		c.Audit = nopAuditSink{}
	}
	if c.Redaction.MaxFieldBytes <= 0 {
		c.Redaction.MaxFieldBytes = 16 * 1024
	}
	if c.Context.Strategy == "" {
		c.Context.Strategy = StrategyTruncateOldest
	}
	if c.Pricing.BytesPerToken <= 0 {
		c.Pricing.BytesPerToken = 4
	}
	if c.Tracer == nil {
		c.Tracer = nopTracer{}
	}
	if c.ToolResultMaxBytes <= 0 {
		c.ToolResultMaxBytes = defaultToolResultMaxBytes
	}
	applyMiddleware(c)
}

// applyMiddleware envolve os providers/tools injetados com os decoradores da
// config (feature 014). Clona o mapa/slice para não mutar as estruturas do host.
func applyMiddleware(c *Config) {
	if len(c.ProviderMiddleware) > 0 && len(c.Providers) > 0 {
		cloned := make(map[string]Provider, len(c.Providers))
		for alias, provider := range c.Providers {
			if provider == nil {
				cloned[alias] = provider
				continue
			}
			wrapped := provider
			for _, mw := range c.ProviderMiddleware {
				wrapped = mw(wrapped)
			}
			cloned[alias] = wrapped
		}
		c.Providers = cloned
	}
	if len(c.ToolMiddleware) > 0 && len(c.Tools) > 0 {
		cloned := make([]ToolSource, len(c.Tools))
		for i, source := range c.Tools {
			if source == nil {
				cloned[i] = source
				continue
			}
			wrapped := source
			for _, mw := range c.ToolMiddleware {
				wrapped = mw(wrapped)
			}
			cloned[i] = wrapped
		}
		c.Tools = cloned
	}
}

// defaultToolResultMaxBytes é o teto do resultado de tool enviado ao modelo
// (feature 013): 32 KiB.
const defaultToolResultMaxBytes = 32 * 1024

// validate falha rápido com erro nomeado (contracts/library-api.md invariantes 1/7).
func validate(c *Config) error {
	if len(c.Providers) == 0 {
		return &ConfigError{Code: "config/providers-vazio", Message: "ao menos um Provider é obrigatório (DI)"}
	}
	for alias, p := range c.Providers {
		if p == nil {
			return &ConfigError{Code: "config/provider-nulo", Message: fmt.Sprintf("Provider %q é nulo", alias)}
		}
	}
	for i, t := range c.Tools {
		if t == nil {
			return &ConfigError{Code: "config/toolsource-nulo", Message: fmt.Sprintf("ToolSource[%d] é nulo", i)}
		}
	}
	for alias, m := range c.Models {
		if _, ok := c.Providers[m.Provider]; !ok {
			return &ConfigError{Code: "config/model-provider-desconhecido", Message: fmt.Sprintf("modelo %q referencia provider %q inexistente", alias, m.Provider)}
		}
	}
	if c.DefaultModel != "" {
		if _, ok := c.Models[c.DefaultModel]; !ok {
			return &ConfigError{Code: "config/default-model-desconhecido", Message: fmt.Sprintf("default_model %q não existe em Models", c.DefaultModel)}
		}
	}
	if c.Credentials == nil {
		return &ConfigError{Code: "config/credentials-obrigatorio", Message: "CredentialProvider é obrigatório"}
	}
	if c.Sessions == nil {
		return &ConfigError{Code: "config/sessions-obrigatorio", Message: "SessionStore é obrigatório"}
	}
	if c.Logger == nil {
		return &ConfigError{Code: "config/logger-obrigatorio", Message: "Logger é obrigatório"}
	}
	return nil
}

// nopAuditSink é o sink padrão quando o host não injeta um (auditoria opcional
// no núcleo; o host normalmente injeta audit/log).
type nopAuditSink struct{}

func (nopAuditSink) Emit(context.Context, AuditEvent) error { return nil }

// nopTracer é o tracer padrão quando o host não injeta um (observabilidade
// opcional; o host normalmente liga o OTel — feature 011).
type nopTracer struct{}

func (nopTracer) StartTurn(ctx context.Context, _ TurnAttrs) (context.Context, Span) {
	return ctx, nopSpan{}
}
func (nopTracer) StartModel(ctx context.Context, _ ModelAttrs) (context.Context, Span) {
	return ctx, nopSpan{}
}
func (nopTracer) StartTool(ctx context.Context, _ ToolAttrs) (context.Context, Span) {
	return ctx, nopSpan{}
}

type nopSpan struct{}

func (nopSpan) End(error) {}
