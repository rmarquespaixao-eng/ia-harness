package openai_responses

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"rmarquespaixao/ia-harness/harness"
)

const (
	// sseDataPrefix é o prefixo das linhas de payload do SSE.
	sseDataPrefix = "data:"
	// sseDone marca o fim opcional do stream (alguns gateways emitem [DONE]).
	sseDone = "[DONE]"
	// maxSSELineBytes limita uma linha do SSE (argumentos grandes cabem).
	maxSSELineBytes = 4 << 20
	// maxSSESnippetRunes limita o trecho do evento inválido no erro.
	maxSSESnippetRunes = 120
)

// streamEvent é um evento do SSE da Responses API com os campos consumidos.
type streamEvent struct {
	Type   string `json:"type"`
	Delta  string `json:"delta"`
	ItemID string `json:"item_id"`
	Item   *struct {
		Type      string `json:"type"`
		ID        string `json:"id"`
		CallID    string `json:"call_id"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"item"`
	Arguments string `json:"arguments"`
	Response  *struct {
		Usage *streamUsage `json:"usage"`
	} `json:"response"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// streamUsage é o usage reportado em response.completed.
type streamUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	// InputTokensDetails.cached_tokens reporta o prompt caching (feature 006).
	InputTokensDetails struct {
		CachedTokens int64 `json:"cached_tokens"`
	} `json:"input_tokens_details"`
}

// toolAccum acumula uma chamada de função por item_id.
type toolAccum struct {
	itemID string
	callID string
	name   string
	args   strings.Builder
}

// streamState acumula o stream até montar a resposta canônica.
type streamState struct {
	text   strings.Builder
	tools  []*toolAccum
	index  map[string]*toolAccum
	usage  *streamUsage
	done   bool
	failed string
}

// parseStream consome o SSE da Responses API e monta a resposta canônica; EOF
// antes de response.completed é stream cortado (FR-ZG-003). O sink recebe
// texto, raciocínio e fragmentos de argumentos (feature 012).
func parseStream(ctx context.Context, body io.Reader, model string, sink harness.StreamSink) (harness.ChatResponse, error) {
	state := &streamState{index: map[string]*toolAccum{}}
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), maxSSELineBytes)

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return harness.ChatResponse{}, contextError(err)
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, sseDataPrefix) {
			// Linhas vazias, comentários/keep-alive e campos event:/id: são ignorados.
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, sseDataPrefix))
		if payload == "" {
			continue
		}
		if payload == sseDone {
			break
		}
		var event streamEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return harness.ChatResponse{}, providerError(codeStream,
				fmt.Sprintf("evento SSE inválido (%s)", truncate(payload, maxSSESnippetRunes)), err)
		}
		state.consume(event, sink)
		if state.failed != "" {
			return harness.ChatResponse{}, providerError(codeStream, "resposta falhou: "+state.failed, nil)
		}
	}
	if err := scanner.Err(); err != nil {
		return harness.ChatResponse{}, transportError(ctx, err)
	}
	if !state.done {
		return harness.ChatResponse{}, providerError(codeStream, "stream encerrado sem response.completed", io.ErrUnexpectedEOF)
	}
	return state.response(model), nil
}

// consume aplica um evento ao estado; eventos desconhecidos são ignorados
// (forward-compatible — FR-ZG-003). Texto, raciocínio e fragmentos de
// argumentos vão ao sink (feature 012).
func (s *streamState) consume(event streamEvent, sink harness.StreamSink) {
	switch event.Type {
	case "response.output_text.delta":
		if event.Delta != "" {
			s.text.WriteString(event.Delta)
			if sink != nil {
				sink.Text(event.Delta)
			}
		}
	case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
		if event.Delta != "" && sink != nil {
			sink.Reasoning(event.Delta)
		}
	case "response.output_item.added":
		if event.Item != nil && event.Item.Type == "function_call" {
			acc := &toolAccum{itemID: event.Item.ID, callID: event.Item.CallID, name: event.Item.Name}
			acc.args.WriteString(event.Item.Arguments)
			s.tools = append(s.tools, acc)
			if acc.itemID != "" {
				s.index[acc.itemID] = acc
			}
		}
	case "response.function_call_arguments.delta":
		if acc := s.at(event.ItemID); acc != nil {
			acc.args.WriteString(event.Delta)
			if sink != nil && event.Delta != "" {
				sink.ToolCallArgs(acc.callID, acc.name, event.Delta)
			}
		}
	case "response.function_call_arguments.done":
		if acc := s.at(event.ItemID); acc != nil && event.Arguments != "" {
			acc.args.Reset()
			acc.args.WriteString(event.Arguments)
		}
	case "response.completed":
		if event.Response != nil {
			s.usage = event.Response.Usage
		}
		s.done = true
	case "response.failed", "response.incomplete":
		s.done = true
		s.failed = "resposta incompleta"
	case "error":
		if event.Error != nil && event.Error.Message != "" {
			s.failed = event.Error.Message
		} else {
			s.failed = "erro do provedor"
		}
	default:
		// reasoning, output_text.done, output_item.done e eventos futuros.
	}
}

// at devolve o acumulador do item_id (nil se desconhecido).
func (s *streamState) at(itemID string) *toolAccum {
	if itemID == "" {
		return nil
	}
	return s.index[itemID]
}

// response monta a resposta canônica: texto + tool calls (fragmento inválido é
// devolvido como veio; a validação é do núcleo) e usage reportado ou estimado.
func (s *streamState) response(model string) harness.ChatResponse {
	out := harness.ChatResponse{
		Message:    harness.Message{Role: harness.RoleAssistant},
		StopReason: harness.StopCompleted,
		Model:      model,
	}
	if text := s.text.String(); text != "" {
		out.Message.Parts = append(out.Message.Parts, harness.Part{Kind: harness.PartText, Text: text})
	}
	for _, acc := range s.tools {
		if acc == nil || (acc.callID == "" && acc.name == "") {
			continue
		}
		out.ToolCalls = append(out.ToolCalls, harness.ToolCall{
			ID:   acc.callID,
			Name: acc.name,
			Args: normalizeArgs(acc.args.String()),
		})
	}
	for i := range out.ToolCalls {
		out.Message.Parts = append(out.Message.Parts, harness.Part{Kind: harness.PartToolCall, Call: &out.ToolCalls[i]})
	}
	if s.usage != nil {
		out.Usage = harness.Usage{
			InputTokens:       s.usage.InputTokens,
			OutputTokens:      s.usage.OutputTokens,
			CachedInputTokens: s.usage.InputTokensDetails.CachedTokens,
			Estimated:         false,
		}
	} else {
		out.Usage = harness.Usage{Estimated: true}
	}
	return out
}

// normalizeArgs devolve os argumentos acumulados; vazio vira "{}".
func normalizeArgs(args string) json.RawMessage {
	args = strings.TrimSpace(args)
	if args == "" {
		return json.RawMessage("{}")
	}
	return json.RawMessage(args)
}
