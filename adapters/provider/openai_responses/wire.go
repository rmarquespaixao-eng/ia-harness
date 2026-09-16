// Package openai_responses implementa harness.Provider sobre a OpenAI Responses
// API (POST /responses), usada pelos modelos GPT/Grok/Muse no OpenCode Zen/Go.
// Streaming SSE, tool calling por itens function_call/function_call_output e
// usage reportado (FR-ZG-002/003, ADR 0011).
package openai_responses

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"rmarquespaixao/ia-harness/harness"
)

// buildBody monta o corpo do request: Params do perfil entram primeiro e os
// campos do protocolo prevalecem (model, input, stream, store, tools).
func buildBody(model string, req harness.ChatRequest) ([]byte, error) {
	body := make(map[string]any, len(req.Params)+6)
	for key, value := range req.Params {
		body[key] = value
	}
	input, err := wireInput(req)
	if err != nil {
		return nil, err
	}
	body["model"] = model
	body["input"] = input
	body["stream"] = true
	// store=false: o histórico é do host/harness (D-ZG-3/ADR 0011).
	body["store"] = false
	if req.MaxOutputTokens > 0 {
		body["max_output_tokens"] = req.MaxOutputTokens
	}
	if len(req.Tools) > 0 {
		tools, err := wireTools(req.Tools)
		if err != nil {
			return nil, err
		}
		body["tools"] = tools
	}
	if len(req.OutputSchema) > 0 {
		// Structured output (feature 009): formato json_schema no campo text.
		var schema any
		if err := json.Unmarshal(req.OutputSchema, &schema); err != nil {
			return nil, fmt.Errorf("schema de saída: %w", err)
		}
		body["text"] = map[string]any{
			"format": map[string]any{
				"type":   "json_schema",
				"name":   "output",
				"schema": schema,
				"strict": true,
			},
		}
	}
	return json.Marshal(body)
}

// wireTool publica a tool no formato da Responses API (type=function achatado).
type wireTool struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters"`
}

// wireFunctionCall é o item de chamada de função do histórico do assistente.
type wireFunctionCall struct {
	Type      string `json:"type"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// wireFunctionOutput é o item de resultado de tool enviado de volta.
type wireFunctionOutput struct {
	Type   string `json:"type"`
	CallID string `json:"call_id"`
	Output string `json:"output"`
}

// wireInput converte o histórico canônico na lista de itens `input` da Responses
// API: mensagens de papel viram itens `{role, content}`; chamadas de tool viram
// `function_call`; resultados viram `function_call_output`.
func wireInput(req harness.ChatRequest) ([]any, error) {
	items := make([]any, 0, len(req.Messages)+1)
	if system := strings.TrimSpace(req.System); system != "" {
		items = append(items, map[string]any{"role": "system", "content": system})
	}
	for _, m := range req.Messages {
		switch m.Role {
		case harness.RoleSystem:
			if text := messageText(m); text != "" {
				items = append(items, map[string]any{"role": "system", "content": text})
			}
		case harness.RoleUser:
			if content := responsesUserContent(m); content != nil {
				items = append(items, map[string]any{"role": "user", "content": content})
			}
			_, results := userParts(m)
			items = append(items, outputItems(results)...)
		case harness.RoleAssistant:
			if text := messageText(m); text != "" {
				items = append(items, map[string]any{"role": "assistant", "content": text})
			}
			for _, part := range m.Parts {
				if part.Kind != harness.PartToolCall || part.Call == nil {
					continue
				}
				args := part.Call.Args
				if len(bytes.TrimSpace(args)) == 0 {
					args = json.RawMessage("{}")
				}
				items = append(items, wireFunctionCall{
					Type:      "function_call",
					CallID:    part.Call.ID,
					Name:      part.Call.Name,
					Arguments: string(args),
				})
			}
		case harness.RoleTool:
			items = append(items, outputItems(messageResults(m))...)
		}
	}
	return items, nil
}

// outputItems converte resultados de tool em itens function_call_output.
func outputItems(results []*harness.ToolResult) []any {
	out := make([]any, 0, len(results))
	for _, result := range results {
		if result == nil {
			continue
		}
		out = append(out, wireFunctionOutput{
			Type:   "function_call_output",
			CallID: result.CallID,
			Output: toolResultText(result),
		})
	}
	return out
}

// wireTools publica as tools no formato function da Responses API.
func wireTools(tools []harness.Tool) ([]wireTool, error) {
	out := make([]wireTool, 0, len(tools))
	for _, tool := range tools {
		params, err := decodeSchema(tool.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("schema da tool %q: %w", tool.Name, err)
		}
		out = append(out, wireTool{
			Type:        "function",
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  params,
		})
	}
	return out, nil
}

// messageText concatena as partes de texto de uma mensagem.
func messageText(m harness.Message) string {
	var text strings.Builder
	for _, part := range m.Parts {
		if part.Kind == harness.PartText {
			text.WriteString(part.Text)
		}
	}
	return text.String()
}

// responsesUserContent monta o content do usuário na Responses API: string
// quando só há texto; array de input_text/input_image/input_file com mídia
// (feature 002/FR-MM-006). nil quando não há conteúdo.
func responsesUserContent(m harness.Message) any {
	var text strings.Builder
	var media []any
	for _, part := range m.Parts {
		switch part.Kind {
		case harness.PartText:
			text.WriteString(part.Text)
		case harness.PartImage:
			if part.Media != nil {
				media = append(media, map[string]any{
					"type":      "input_image",
					"image_url": responsesMediaURL(part.Media),
				})
			}
		case harness.PartDocument:
			if part.Media != nil {
				media = append(media, responsesDocumentContent(part.Media))
			}
		}
	}
	if len(media) == 0 {
		if text.Len() == 0 {
			return nil
		}
		return text.String()
	}
	parts := make([]any, 0, len(media)+1)
	if text.Len() > 0 {
		parts = append(parts, map[string]any{"type": "input_text", "text": text.String()})
	}
	parts = append(parts, media...)
	return parts
}

// responsesMediaURL devolve a referência (URL) ou um data URI base64.
func responsesMediaURL(m *harness.Media) string {
	if m.Reference != "" {
		return m.Reference
	}
	return "data:" + m.MIME + ";base64," + base64.StdEncoding.EncodeToString(m.Bytes)
}

// responsesDocumentContent mapeia documento: textual vira input_text; binário
// (PDF) vira input_file com file_data (FR-MM-006/007).
func responsesDocumentContent(m *harness.Media) map[string]any {
	if strings.HasPrefix(strings.ToLower(m.MIME), "text/") {
		text := m.Reference
		if len(m.Bytes) > 0 {
			text = string(m.Bytes)
		}
		return map[string]any{"type": "input_text", "text": text}
	}
	data := m.Reference
	if len(m.Bytes) > 0 {
		data = "data:" + m.MIME + ";base64," + base64.StdEncoding.EncodeToString(m.Bytes)
	}
	return map[string]any{"type": "input_file", "filename": m.Name, "file_data": data}
}

// userParts separa o texto dos resultados de tool de uma mensagem de usuário.
func userParts(m harness.Message) ([]string, []*harness.ToolResult) {
	var text []string
	var results []*harness.ToolResult
	for _, part := range m.Parts {
		switch part.Kind {
		case harness.PartText:
			text = append(text, part.Text)
		case harness.PartToolResult:
			if part.Result != nil {
				results = append(results, part.Result)
			}
		}
	}
	return text, results
}

// messageResults extrai os resultados de tool de uma mensagem de tool.
func messageResults(m harness.Message) []*harness.ToolResult {
	var results []*harness.ToolResult
	for _, part := range m.Parts {
		if part.Kind == harness.PartToolResult && part.Result != nil {
			results = append(results, part.Result)
		}
	}
	return results
}

// toolResultText serializa o resultado para o output; erro/negativa sem
// conteúdo recebem texto estável para o modelo não ver vazio.
func toolResultText(result *harness.ToolResult) string {
	parts := make([]string, 0, len(result.Content))
	for _, content := range result.Content {
		switch content.Kind {
		case harness.ResultText:
			if content.Text != "" {
				parts = append(parts, content.Text)
			}
		case harness.ResultJSON:
			if len(content.JSON) > 0 {
				parts = append(parts, string(content.JSON))
			}
		}
	}
	if len(parts) == 0 {
		switch {
		case result.Denied:
			return "chamada negada pela política"
		case result.IsError:
			return "erro na execução da tool"
		}
	}
	return strings.Join(parts, "\n")
}

// decodeSchema decodifica o InputSchema para um objeto JSON; schema ausente
// vira o objeto vazio aceito pela API.
func decodeSchema(raw json.RawMessage) (any, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return emptySchema(), nil
	}
	var schema any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, err
	}
	if schema == nil {
		return emptySchema(), nil
	}
	return schema, nil
}

// emptySchema devolve o schema objeto vazio.
func emptySchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
