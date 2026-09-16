package engine

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// retrievalK é o número de trechos recuperados por turno (CU-HAR-6).
const retrievalK = 5

// memoryMessages injeta fatos duráveis do usuário e trechos recuperados como
// mensagens de sistema com proveniência (FR-027/FR-028). Sem as portas Memory/
// Retriever não há leitura alguma e o retorno é nil (FR-029). Falha de porta
// degrada o turno para o contexto já obtido — a assinatura não devolve erro ao
// loop (CU-HAR-6, fluxo 3a) — e o erro é registrado no Logger com a causa.
func (h *Harness) memoryMessages(ctx context.Context, session *Session, input []Part) []Message {
	if h.cfg.Memory == nil && h.cfg.Retriever == nil {
		return nil
	}
	now := h.cfg.Clock.Now()
	var messages []Message

	if h.cfg.Memory != nil {
		facts, err := h.cfg.Memory.List(ctx, session.UserID)
		if err != nil {
			h.cfg.Logger.WarnContext(ctx, "harness: memória: listar fatos falhou",
				"error", fmt.Errorf("harness: memória: %w", err))
		} else if text := h.factsContext(facts); text != "" {
			messages = append(messages, newSystemMessage(text, now))
		}
	}

	if h.cfg.Retriever != nil {
		if query := lastInputText(input); query != "" {
			items, err := h.cfg.Retriever.Retrieve(ctx, session.UserID, query, retrievalK)
			if err != nil {
				h.cfg.Logger.WarnContext(ctx, "harness: memória: recuperar trechos falhou",
					"error", fmt.Errorf("harness: memória: %w", err))
			} else if text := h.retrievedContext(items); text != "" {
				messages = append(messages, newSystemMessage(text, now))
			}
		}
	}
	return messages
}

// factsContext renderiza os fatos do usuário com a proveniência de cada um; o
// texto é redigido/truncado pelo Redactor campo a campo, para a fonte (RN-2)
// sobreviver mesmo a um texto longo (FR-024/FR-028).
func (h *Harness) factsContext(facts []Fact) string {
	if len(facts) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Memória do usuário (fatos duráveis):")
	for _, fact := range facts {
		text, _ := h.redactor.RedactString(fact.Text)
		source, _ := h.redactor.RedactString(fact.Source)
		fmt.Fprintf(&b, "\n- (%s) %s (fonte: %s)", fact.Kind, text, source)
	}
	return b.String()
}

// retrievedContext renderiza os trechos recuperados com citação ([id] fonte) e
// conteúdo redigido/truncado pelo Redactor campo a campo, para a citação
// sobreviver mesmo a um trecho longo (FR-028/FR-024).
func (h *Harness) retrievedContext(items []RetrievedItem) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Trechos recuperados (cite a fonte):")
	for _, item := range items {
		id, _ := h.redactor.RedactString(item.ID)
		source, _ := h.redactor.RedactString(item.Source)
		text, _ := h.redactor.RedactString(item.Text)
		fmt.Fprintf(&b, "\n- [%s] %s: %s", id, source, text)
	}
	return b.String()
}

// newSystemMessage monta uma mensagem de sistema de um único texto.
func newSystemMessage(text string, now time.Time) Message {
	return Message{
		ID:        newID(),
		Role:      RoleSystem,
		Parts:     []Part{{Kind: PartText, Text: text}},
		CreatedAt: now,
	}
}

// lastInputText devolve o último texto não vazio da entrada: a consulta da
// recuperação semântica é a mensagem atual do usuário.
func lastInputText(input []Part) string {
	for i := len(input) - 1; i >= 0; i-- {
		if input[i].Kind != PartText {
			continue
		}
		if text := strings.TrimSpace(input[i].Text); text != "" {
			return text
		}
	}
	return ""
}
