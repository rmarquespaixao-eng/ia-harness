package engine

import (
	"context"
	"math"
	"time"

	"github.com/rmarquespaixao-eng/ia-harness/internal/engine/telemetry"
)

// summaryProvenance prefixa o resumo injetado pela estratégia summarize para que
// o contexto enviado ao modelo deixe explícito de onde o texto veio (FR-021/RN-2:
// janela transparente, nunca corte silencioso).
const summaryProvenance = "resumo de histórico: "

// defaultCompactAtRatio é o gatilho de compactação quando a janela vem da
// capacidade do modelo e o host não define ContextPolicy.CompactAtRatio.
const defaultCompactAtRatio = 0.8

// buildMessages monta o contexto do turno a partir do histórico + entrada e
// aplica a janela com o orçamento global legado (Context.MaxTokens, gatilho
// 1,0). O loop usa applyWindow a cada chamada de modelo; esta função é mantida
// para o contrato anterior (feature 0005) e a suíte histórica.
func (h *Harness) buildMessages(ctx context.Context, session *Session, input []Part) []Message {
	messages := make([]Message, 0, len(session.Messages)+1)
	messages = append(messages, session.Messages...)
	if len(input) > 0 {
		messages = append(messages, Message{
			ID:        newID(),
			Role:      RoleUser,
			Parts:     input,
			CreatedAt: h.cfg.Clock.Now(),
		})
	}
	summarize := h.cfg.Context.Strategy == StrategySummarize && h.cfg.Context.Summarizer != nil
	kept, _ := h.applyWindow(ctx, session, messages, 0, h.cfg.Context.MaxTokens, 1, summarize, "")
	return kept
}

// contextBudget devolve o orçamento de entrada do modelo. Quando o perfil
// declara MaxContextTokens, o orçamento é a janela do modelo menos a reserva de
// saída e a margem de segurança (fromModel=true); senão cai para a política
// global legada (Context.MaxTokens). budget<=0 ⇒ janela desligada.
func (h *Harness) contextBudget(profile ModelProfile) (budget int, fromModel bool) {
	if limit := profile.Capabilities.MaxContextTokens; limit > 0 {
		reserve := profile.Capabilities.MaxOutputTokens + h.cfg.Context.SafetyMargin
		budget = limit - reserve
		if budget < 1 {
			budget = 1
		}
		return budget, true
	}
	if h.cfg.Context.MaxTokens > 0 {
		return h.cfg.Context.MaxTokens, false
	}
	return 0, false
}

// compactionRatio devolve o gatilho de compactação: configurável quando o
// orçamento vem do modelo (default 0,8), 1,0 no fallback legado.
func (h *Harness) compactionRatio(fromModel bool) float64 {
	if !fromModel {
		return 1
	}
	if h.cfg.Context.CompactAtRatio > 0 {
		return h.cfg.Context.CompactAtRatio
	}
	return defaultCompactAtRatio
}

// applyWindow estima o contexto (mensagens + extraTokens) e, ao atingir o
// gatilho (budget × ratio), remove os blocos mais antigos preservando o último
// e a integridade dos pares de tool. Com allowSummarize, resume o removido e
// injeta como mensagem de sistema com proveniência (≤1×/turno é garantido pelo
// chamador). Devolve o contexto ajustado e o registro da compactação (nil
// quando nada mudou).
func (h *Harness) applyWindow(ctx context.Context, session *Session, messages []Message, extraTokens, budget int, ratio float64, allowSummarize bool, model string) ([]Message, *CompactionInfo) {
	if budget <= 0 || len(messages) == 0 {
		return messages, nil
	}
	if ratio <= 0 {
		ratio = 1
	}
	before := h.estimateWindowTokens(messages) + extraTokens
	if float64(before) <= float64(budget)*ratio {
		return messages, nil
	}

	fits := func(kept []Message) bool {
		return h.estimateWindowTokens(kept)+extraTokens <= budget
	}
	kept, removed := h.removeOldestBlocks(messages, fits)
	if len(removed) == 0 {
		return messages, nil
	}

	info := &CompactionInfo{TokensBefore: before, MessagesRemoved: len(removed)}
	if allowSummarize {
		summary, err := h.summarize(ctx, removed, budget, model)
		switch {
		case err != nil:
			h.cfg.Logger.Warn("janela de contexto: sumarização falhou; seguindo com truncamento",
				"error", err.Error(), "session_id", session.ID)
		case summary != "":
			kept = injectSummary(kept, summary, h.cfg.Clock.Now())
			info.Summarized = true
		}
	}
	info.TokensAfter = h.estimateWindowTokens(kept) + extraTokens
	return kept, info
}

// summarize delega ao Summarizer; quando ele implementa ModelSummarizer
// (feature 018), recebe o alias do modelo do turno para resumir com o mesmo
// modelo do perfil.
func (h *Harness) summarize(ctx context.Context, removed []Message, budget int, model string) (string, error) {
	if summarizer, ok := h.cfg.Context.Summarizer.(ModelSummarizer); ok {
		return summarizer.SummarizeForModel(ctx, model, removed, budget)
	}
	return h.cfg.Context.Summarizer.Summarize(ctx, removed, budget)
}

// removeOldestBlocks remove blocos inteiros do mais antigo para o mais novo
// (preservando mensagens de sistema e o último bloco) até o conjunto satisfazer
// fits. Um bloco é uma mensagem isolada ou uma chamada de tool com os
// resultados subsequentes — nunca se separa a chamada do resultado.
func (h *Harness) removeOldestBlocks(messages []Message, fits func([]Message) bool) (kept, removed []Message) {
	blocks := contextBlocks(messages)
	for len(blocks) > 1 && !fits(flattenBlocks(blocks)) {
		index := -1
		for i := 0; i < len(blocks)-1; i++ {
			if blocks[i][0].Role != RoleSystem {
				index = i
				break
			}
		}
		if index < 0 {
			break
		}
		removed = append(removed, blocks[index]...)
		blocks = append(blocks[:index:index], blocks[index+1:]...)
	}
	return flattenBlocks(blocks), removed
}

// contextBlocks agrupa o histórico em blocos: uma chamada de tool (mensagem do
// assistente com PartToolCall) absorve os resultados subsequentes.
func contextBlocks(messages []Message) [][]Message {
	blocks := make([][]Message, 0, len(messages))
	for i := 0; i < len(messages); {
		block := []Message{messages[i]}
		i++
		if messageHasToolCall(block[0]) {
			for i < len(messages) && messageHasToolResult(messages[i]) {
				block = append(block, messages[i])
				i++
			}
		}
		blocks = append(blocks, block)
	}
	return blocks
}

func messageHasToolCall(message Message) bool {
	for _, part := range message.Parts {
		if part.Kind == PartToolCall {
			return true
		}
	}
	return false
}

func messageHasToolResult(message Message) bool {
	for _, part := range message.Parts {
		if part.Kind == PartToolResult {
			return true
		}
	}
	return false
}

func flattenBlocks(blocks [][]Message) []Message {
	out := make([]Message, 0)
	for _, block := range blocks {
		out = append(out, block...)
	}
	return out
}

// injectSummary insere o resumo como mensagem de sistema com proveniência, logo
// após as mensagens de sistema preservadas (topo do contexto enviado).
func injectSummary(messages []Message, summary string, now time.Time) []Message {
	message := Message{
		ID:        newID(),
		Role:      RoleSystem,
		Parts:     []Part{{Kind: PartText, Text: summaryProvenance + summary}},
		CreatedAt: now,
	}
	at := 0
	for at < len(messages) && messages[at].Role == RoleSystem {
		at++
	}
	out := make([]Message, 0, len(messages)+1)
	out = append(out, messages[:at]...)
	out = append(out, message)
	out = append(out, messages[at:]...)
	return out
}

// emitCompaction publica o evento de compactação quando o Handler implementa a
// interface opcional CompactionHandler (não quebra handler existentes).
func (h *Harness) emitCompaction(ctx context.Context, handler Handler, sessionID string, info *CompactionInfo) {
	if handler == nil || info == nil {
		return
	}
	sink, ok := handler.(CompactionHandler)
	if !ok {
		return
	}
	sink.Compaction(ctx, CompactionEvent{
		SessionID:       sessionID,
		TokensBefore:    info.TokensBefore,
		TokensAfter:     info.TokensAfter,
		MessagesRemoved: info.MessagesRemoved,
		Summarized:      info.Summarized,
	})
}

// estimateTextTokens estima os tokens de um texto: com a Tokenizer injetada
// (feature 010) conta o payload real; senão usa a premissa de bytes/token do
// Pricing (ceil, nunca subestima).
func (h *Harness) estimateTextTokens(text string) int {
	if text == "" {
		return 0
	}
	if h.cfg.Tokenizer != nil {
		return h.cfg.Tokenizer.Count(text)
	}
	return int(telemetry.EstimateTokens(h.cfg.Pricing, len(text)))
}

// estimateToolsTokens estima os tokens das definições de tools (nome,
// descrição, schema e namespace), que também ocupam a janela do modelo
// (feature 018/FR-CTX-003).
func (h *Harness) estimateToolsTokens(tools []Tool) int {
	if len(tools) == 0 {
		return 0
	}
	if h.cfg.Tokenizer != nil {
		total := 0
		for _, tool := range tools {
			total += h.cfg.Tokenizer.Count(tool.Name)
			total += h.cfg.Tokenizer.Count(tool.Description)
			total += h.cfg.Tokenizer.Count(string(tool.InputSchema))
			total += h.cfg.Tokenizer.Count(tool.Namespace)
		}
		return total
	}
	bytes := 0
	for _, tool := range tools {
		bytes += len(tool.Name) + len(tool.Description) + len(tool.InputSchema) + len(tool.Namespace)
	}
	return int(telemetry.EstimateTokens(h.cfg.Pricing, bytes))
}

// estimateWindowTokens estima os tokens de um conjunto de mensagens: com a
// Tokenizer injetada conta o payload real (feature 010); senão usa a premissa de
// bytes/token do Pricing (cost.go: default 4, ceil — nunca subestima), somando o
// conteúdo textual/JSON das mensagens.
func (h *Harness) estimateWindowTokens(messages []Message) int {
	if h.cfg.Tokenizer != nil {
		total := 0
		for _, message := range messages {
			total += tokenizeMessage(h.cfg.Tokenizer, message)
		}
		return total
	}
	bytes := 0
	for _, message := range messages {
		bytes += messageBytes(message)
	}
	estimated := telemetry.EstimateTokens(h.cfg.Pricing, bytes)
	if estimated > math.MaxInt {
		return math.MaxInt
	}
	return int(estimated)
}

// tokenizeMessage soma os tokens do payload de uma mensagem pela Tokenizer
// injetada (texto, args de tool call e conteúdo de resultado).
func tokenizeMessage(t Tokenizer, message Message) int {
	total := 0
	for _, part := range message.Parts {
		total += t.Count(part.Text)
		if part.Call != nil {
			total += t.Count(string(part.Call.Args))
		}
		if part.Result != nil {
			for _, content := range part.Result.Content {
				total += t.Count(content.Text)
				total += t.Count(string(content.JSON))
			}
		}
	}
	return total
}

// messageBytes soma texto de partes, args de tool call e conteúdo (texto/JSON)
// de resultados — os campos que carregam payload para o modelo.
func messageBytes(message Message) int {
	bytes := 0
	for _, part := range message.Parts {
		bytes += len(part.Text)
		if part.Call != nil {
			bytes += len(part.Call.Args)
		}
		if part.Result != nil {
			for _, content := range part.Result.Content {
				bytes += len(content.Text) + len(content.JSON)
			}
		}
		if part.Media != nil {
			// Mídia conta pelo payload efetivo (FR-MM-009).
			bytes += len(part.Media.Bytes) + len(part.Media.Reference)
		}
	}
	return bytes
}
