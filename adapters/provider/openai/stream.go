package openai

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
	// sseDone marca o fim do stream da Chat Completions.
	sseDone = "[DONE]"
	// maxSSELineBytes limita uma linha do SSE (tool calls grandes cabem).
	maxSSELineBytes = 4 << 20
	// maxSSESnippetRunes limita o trecho do chunk inválido no erro.
	maxSSESnippetRunes = 120
)

// streamChunk é um chunk de chat.completion.chunk com os campos consumidos.
type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			Reasoning        string `json:"reasoning"`
			ToolCalls        []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *streamUsage `json:"usage"`
}

// streamUsage é o usage reportado no chunk final (stream_options.include_usage).
type streamUsage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	// PromptTokensDetails.cached_tokens reporta o prompt caching (feature 006).
	PromptTokensDetails struct {
		CachedTokens int64 `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

// toolAccum acumula os fragmentos de uma tool call por índice.
type toolAccum struct {
	id   string
	name string
	args strings.Builder
}

// streamState acumula o stream até montar a resposta canônica.
type streamState struct {
	text   strings.Builder
	tools  []*toolAccum
	usage  *streamUsage
	finish string
	done   bool
}

// parseStream consome o SSE da Chat Completions e monta a resposta canônica;
// EOF antes de [DONE] é stream cortado (T041, R4). O sink recebe texto,
// raciocínio e fragmentos de argumentos (feature 012).
func parseStream(ctx context.Context, body io.Reader, model string, sink harness.StreamSink) (harness.ChatResponse, error) {
	state := &streamState{}
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
			state.done = true
			break
		}
		var chunk streamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return harness.ChatResponse{}, providerError(codeStream,
				fmt.Sprintf("chunk SSE inválido (%s)", truncate(payload, maxSSESnippetRunes)), err)
		}
		state.consume(chunk, sink)
	}
	if err := scanner.Err(); err != nil {
		return harness.ChatResponse{}, transportError(ctx, err)
	}
	if !state.done {
		message := "stream encerrado sem " + sseDone
		if state.finish != "" {
			message += " (finish_reason=" + state.finish + ")"
		}
		return harness.ChatResponse{}, providerError(codeStream, message, io.ErrUnexpectedEOF)
	}
	return state.response(model), nil
}

// consume aplica um chunk ao estado: texto vai para o sink e tool calls são
// acumuladas por índice (id/name e arguments concatenados), emitindo os
// fragmentos de argumentos como ToolCallArgs (feature 012).
func (s *streamState) consume(chunk streamChunk, sink harness.StreamSink) {
	if chunk.Usage != nil {
		s.usage = chunk.Usage
	}
	for _, choice := range chunk.Choices {
		if choice.Delta.Content != "" {
			s.text.WriteString(choice.Delta.Content)
			if sink != nil {
				sink.Text(choice.Delta.Content)
			}
		}
		if reasoning := firstNonEmpty(choice.Delta.ReasoningContent, choice.Delta.Reasoning); reasoning != "" && sink != nil {
			sink.Reasoning(reasoning)
		}
		for _, call := range choice.Delta.ToolCalls {
			acc := s.at(call.Index)
			if call.ID != "" {
				acc.id = call.ID
			}
			acc.name += call.Function.Name
			if call.Function.Arguments != "" && sink != nil {
				sink.ToolCallArgs(acc.id, acc.name, call.Function.Arguments)
			}
			acc.args.WriteString(call.Function.Arguments)
		}
		if choice.FinishReason != "" {
			s.finish = choice.FinishReason
		}
	}
}

// firstNonEmpty devolve o primeiro valor não vazio.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// at devolve (criando) o acumulador do índice, preservando a ordem das calls.
func (s *streamState) at(index int) *toolAccum {
	for len(s.tools) <= index {
		s.tools = append(s.tools, &toolAccum{})
	}
	return s.tools[index]
}

// response monta a resposta canônica: texto + tool calls (fragmentos de
// argumento inválidos são devolvidos como vieram; a validação é do núcleo) e
// usage reportado ou marcado como estimado (R5).
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
		if acc == nil || (acc.id == "" && acc.name == "") {
			continue
		}
		out.ToolCalls = append(out.ToolCalls, harness.ToolCall{
			ID:   acc.id,
			Name: acc.name,
			Args: normalizeArgs(acc.args.String()),
		})
	}
	for i := range out.ToolCalls {
		out.Message.Parts = append(out.Message.Parts, harness.Part{Kind: harness.PartToolCall, Call: &out.ToolCalls[i]})
	}
	if s.usage != nil {
		out.Usage = harness.Usage{
			InputTokens:       s.usage.PromptTokens,
			OutputTokens:      s.usage.CompletionTokens,
			CachedInputTokens: s.usage.PromptTokensDetails.CachedTokens,
			Estimated:         false,
		}
	} else {
		out.Usage = harness.Usage{Estimated: true}
	}
	return out
}

// normalizeArgs devolve os argumentos acumulados; vazio vira "{}" e fragmento
// inválido é devolvido como veio (a validação é do harness/loop).
func normalizeArgs(args string) json.RawMessage {
	args = strings.TrimSpace(args)
	if args == "" {
		return json.RawMessage("{}")
	}
	return json.RawMessage(args)
}
