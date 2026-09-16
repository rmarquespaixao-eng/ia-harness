# Feature Specification: Tokenizer plugável

**Feature Branch**: `nucleo/010-tokenizer-port`

**Status**: Aprovado e implementado

**Input**: lacuna P0 — budget/janela usavam estimativa por bytes; sem tokenizer do modelo, o corte de contexto é impreciso.

## 1. Problema

A contagem de tokens por `bytes/per_token` (default 4) é uma aproximação. Modelos têm tokenizers distintos; a janela e o orçamento deveriam usar a contagem real quando o host a fornece. Adicionar dependência de tokenizer (ex.: tiktoken) ao núcleo seria pesado e multi-provedor; a solução é uma **porta**.

## 2. Requisitos (EARS)

- **FR-TOK-001**: O harness MUST expor a porta `Tokenizer` (`Count(text) int`).
- **FR-TOK-002**: WHEN `Config.Tokenizer` está injetado, a janela de contexto MUST estimar tokens pela contagem da porta (texto, args de tool call e conteúdo de resultado).
- **FR-TOK-003**: Sem `Tokenizer`, MUST manter a heurística por `Pricing.BytesPerToken` (comportamento anterior).
- **FR-TOK-004**: O núcleo MUST NOT adicionar dependência de tokenizer (o host pluga a implementação, ex.: tiktoken por modelo).

## 3. Critérios de aceite (Gherkin)

```gherkin
Cenário: tokenizer injetada
  Dado um Tokenizer que conta palavras
  Quando a janela estima um conjunto de mensagens
  Então usa a contagem do Tokenizer

Cenário: sem tokenizer
  Dado nenhum Tokenizer
  Quando a janela estima
  Então usa bytes/token da Pricing
```

## 4. Não-objetivos

- Implementar tokenizer BPE (o host/Tiktoken faz).
- Trocar a estimativa de **custo** (mantém volume; a porta é para janela).
