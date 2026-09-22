// Package config carrega e valida o arquivo de configuração do harness
// (contracts/config/harness_config.json) e o aplica sobre harness.Config. O
// arquivo descreve modelos, política, pricing, contexto, redação, system prompt
// e servidores MCP; os Providers/Tools/portas continuam vindo por DI (constitution
// §2/§4 — nenhum segredo no arquivo). Feature 008/ADR 0015.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/rmarquespaixao-eng/ia-harness/contracts"
	gen "github.com/rmarquespaixao-eng/ia-harness/contracts/gen"
	"github.com/rmarquespaixao-eng/ia-harness/harness"
	"github.com/rmarquespaixao-eng/ia-harness/internal/platform/schema"
)

// schemaPath é o contrato canônico do arquivo de configuração.
const schemaPath = "config/harness_config.json"

// MCPServer descreve um servidor MCP declarado no arquivo (o host monta o
// mcpclient com a credencial resolvida por env:/file:).
type MCPServer struct {
	Name             string
	Endpoint         string
	CredentialRef    string
	ToolTimeout      time.Duration
	Command          string
	Args             []string
	Env              map[string]string
	EnvCredentials   map[string]string
	Dir              string
	TerminateTimeout time.Duration
}

// File é um arquivo de configuração já validado contra o schema.
type File struct {
	doc gen.HarnessConfigFile
}

// Load lê o arquivo do disco, valida contra o schema canônico e devolve o File.
func Load(path string) (*File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: ler %q: %w", path, err)
	}
	file, err := Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("config: %q: %w", path, err)
	}
	return file, nil
}

// Parse valida os bytes contra o schema canônico e interpreta no tipo gerado.
func Parse(raw []byte) (*File, error) {
	validator, err := compileValidator()
	if err != nil {
		return nil, err
	}
	if err := validator.Validate(json.RawMessage(raw)); err != nil {
		return nil, fmt.Errorf("config: schema inválido: %w", err)
	}
	var doc gen.HarnessConfigFile
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("config: interpretar: %w", err)
	}
	return &File{doc: doc}, nil
}

// cache do validador compilado (schema embutido, imutável).
var cachedValidator *schema.Validator

// compileValidator compila o schema do arquivo uma vez.
func compileValidator() (*schema.Validator, error) {
	if cachedValidator != nil {
		return cachedValidator, nil
	}
	raw, err := contracts.Schema(schemaPath)
	if err != nil {
		return nil, err
	}
	compiled, err := schema.Compile(raw)
	if err != nil {
		return nil, err
	}
	cachedValidator = compiled
	return compiled, nil
}

// Apply aplica os campos de configuração ao Config do host: modelos,
// default_model, system_prompt, política, pricing, contexto e redação. Não toca
// em Providers/Tools/portas (DI).
func (f *File) Apply(cfg *harness.Config) {
	if f == nil {
		return
	}
	if f.doc.DefaultModel != nil {
		cfg.DefaultModel = *f.doc.DefaultModel
	}
	if f.doc.SystemPrompt != nil {
		cfg.SystemPrompt = *f.doc.SystemPrompt
	}
	if models := f.models(); len(models) > 0 {
		if cfg.Models == nil {
			cfg.Models = map[string]harness.ModelProfile{}
		}
		for alias, profile := range models {
			cfg.Models[alias] = profile
		}
	}
	if f.doc.Policy.Default != "" || len(f.doc.Policy.Agents) > 0 {
		cfg.Policy = f.policy()
	}
	if p := f.doc.Pricing; p != nil {
		cfg.Pricing = harness.Pricing{
			Currency:                         p.Currency,
			InputPriceMicrosPerMillion:       int64(p.InputPriceMicrosPerMillion),
			OutputPriceMicrosPerMillion:      int64(p.OutputPriceMicrosPerMillion),
			BytesPerToken:                    p.BytesPerToken,
			CachedInputPriceMicrosPerMillion: int64Value(p.CachedInputPriceMicrosPerMillion),
			CacheWritePriceMicrosPerMillion:  int64Value(p.CacheWritePriceMicrosPerMillion),
		}
	}
	if c := f.doc.Context; c != nil {
		cfg.Context = harness.ContextPolicy{MaxTokens: intValue(c.MaxTokens, 0), Strategy: harness.ContextStrategy(stringValue(c.Strategy))}
	}
	if r := f.doc.Redaction; r != nil {
		cfg.Redaction = harness.RedactionConfig{SensitiveKeys: r.SensitiveKeys, MaxFieldBytes: intValue(r.MaxFieldBytes, 0)}
	}
}

// MCPServers devolve os servidores MCP declarados (para o host montar o
// mcpclient).
func (f *File) MCPServers() []MCPServer {
	if f == nil {
		return nil
	}
	out := make([]MCPServer, 0, len(f.doc.McpServers))
	for _, s := range f.doc.McpServers {
		out = append(out, MCPServer{
			Name:             s.Name,
			Endpoint:         stringValue(s.Endpoint),
			CredentialRef:    stringValue(s.CredentialRef),
			ToolTimeout:      time.Duration(intValue(s.ToolTimeoutSeconds, 0)) * time.Second,
			Command:          stringValue(s.Command),
			Args:             s.Args,
			Env:              mapString(s.Env),
			EnvCredentials:   mapString(s.EnvCredentials),
			Dir:              stringValue(s.Dir),
			TerminateTimeout: time.Duration(intValue(s.TerminateTimeoutSeconds, 0)) * time.Second,
		})
	}
	return out
}

// models converte os perfis gerados para os tipos públicos.
func (f *File) models() map[string]harness.ModelProfile {
	out := make(map[string]harness.ModelProfile, len(f.doc.Models))
	for alias, m := range f.doc.Models {
		out[alias] = harness.ModelProfile{
			Provider:     m.Provider,
			Model:        m.Model,
			Capabilities: capabilities(m.Capabilities),
			Params:       map[string]any(m.Params),
			Fallbacks:    m.Fallbacks,
		}
	}
	return out
}

// capabilities converte as capacidades geradas (ponteiros) para o tipo público.
func capabilities(c *gen.Capabilities) harness.Capabilities {
	if c == nil {
		return harness.Capabilities{}
	}
	return harness.Capabilities{
		ToolCalling:      boolValue(c.ToolCalling),
		Streaming:        boolValue(c.Streaming),
		MaxContextTokens: intValue(c.MaxContextTokens, 0),
		MaxOutputTokens:  intValue(c.MaxOutputTokens, 0),
		Vision:           boolValue(c.Vision),
		Documents:        boolValue(c.Documents),
		MaxMediaBytes:    int64Value(c.MaxMediaBytes),
		PromptCaching:    boolValue(c.PromptCaching),
	}
}

// policy converte a política gerada para o tipo público.
func (f *File) policy() harness.PolicyConfig {
	out := harness.PolicyConfig{Default: harness.PolicyMode(f.doc.Policy.Default)}
	if len(f.doc.Policy.Agents) > 0 {
		out.Agents = make(map[string]harness.AgentPolicy, len(f.doc.Policy.Agents))
		for id, a := range f.doc.Policy.Agents {
			out.Agents[id] = agentPolicy(a)
		}
	}
	return out
}

// agentPolicy converte a política de agente, incluindo overrides por tool.
func agentPolicy(a gen.AgentPolicy) harness.AgentPolicy {
	out := harness.AgentPolicy{
		AllowTools:   a.AllowTools,
		DenyTools:    a.DenyTools,
		ConfirmTools: a.ConfirmTools,
		SystemPrompt: stringValue(a.SystemPrompt),
	}
	if a.Mode != nil {
		out.Mode = harness.PolicyMode(*a.Mode)
	}
	if len(a.Overrides) > 0 {
		out.Overrides = make(map[string]harness.ToolPolicy, len(a.Overrides))
		for name, o := range a.Overrides {
			out.Overrides[name] = harness.ToolPolicy{
				Confirm:    boolValue(o.Confirm),
				ReadOnly:   boolValue(o.ReadOnly),
				Idempotent: boolValue(o.Idempotent),
				Timeout:    time.Duration(intValue(o.TimeoutSeconds, 0)) * time.Second,
			}
		}
	}
	return out
}

func boolValue(p *bool) bool { return p != nil && *p }

func intValue(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}

func int64Value(p *int) int64 {
	if p == nil {
		return 0
	}
	return int64(*p)
}

func stringValue[T ~string](p *T) string {
	if p == nil {
		return ""
	}
	return string(*p)
}

func mapString[M ~map[string]string](m M) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
