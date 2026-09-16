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

// buildMessages monta o contexto enviado ao modelo (histórico persistido + a
// entrada do turno) aplicando a ContextPolicy.
//
// Orçamento: MaxTokens <= 0 desliga a janela (comportamento anterior). A
// estimativa usa Pricing.BytesPerToken (default 4) sobre texto/JSON das
// mensagens — sem tokenizer específico de provedor.
//
// truncate_oldest (default): remove as mensagens mais antigas que não sejam de
// sistema até caber, protegendo a última mensagem do conjunto — a entrada do
// turno entra sempre. summarize: se o Summarizer está configurado e houve corte,
// chama Summarize(ctx, removidas, MaxTokens) no máximo 1×/turno e injeta o
// resumo como RoleSystem com proveniência; erro do Summarizer cai para o
// truncamento sem derrubar o turno.
//
// Registro do corte: o loop do turno chama buildMessages uma única vez e não
// expõe gancho para evento; a decisão de janela fica representada nas próprias
// mensagens devolvidas (preservadas/removidas/resumo) e será auditada pela US5
// (FR-021), que observa este resultado. Nenhum estado extra é guardado no
// Harness — evita acoplamento e mantém o núcleo livre de I/O.
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

	maxTokens := h.cfg.Context.MaxTokens
	if maxTokens <= 0 || len(messages) == 0 || h.estimateWindowTokens(messages) <= maxTokens {
		return messages
	}

	kept, removed := h.truncateOldest(messages, maxTokens)
	if len(removed) == 0 {
		return kept
	}
	if h.cfg.Context.Strategy != StrategySummarize || h.cfg.Context.Summarizer == nil {
		return kept
	}

	summary, err := h.cfg.Context.Summarizer.Summarize(ctx, removed, maxTokens)
	if err != nil {
		h.cfg.Logger.Warn("janela de contexto: sumarização falhou; seguindo com truncamento",
			"error", err.Error(), "session_id", session.ID)
		return kept
	}
	if summary == "" {
		return kept
	}
	return injectSummary(kept, summary, h.cfg.Clock.Now())
}

// truncateOldest remove, do mais antigo para o mais novo, as mensagens que não
// sejam de sistema e que não sejam a última do conjunto (entrada atual ou
// mensagem mais recente do histórico), até o total caber no orçamento.
func (h *Harness) truncateOldest(messages []Message, maxTokens int) (kept, removed []Message) {
	kept = append([]Message(nil), messages...)
	for len(kept) > 0 && h.estimateWindowTokens(kept) > maxTokens {
		index := -1
		for i := 0; i < len(kept)-1; i++ {
			if kept[i].Role != RoleSystem {
				index = i
				break
			}
		}
		if index < 0 {
			break
		}
		removed = append(removed, kept[index])
		kept = append(kept[:index], kept[index+1:]...)
	}
	return kept, removed
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
