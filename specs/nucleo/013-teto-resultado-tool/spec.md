# Feature Specification: Teto de resultado de tool ao modelo

**Feature Branch**: `nucleo/013-teto-resultado-tool` · **Status**: Aprovado e implementado

## Problema
Um resultado de tool gigante entrava inteiro no histórico enviado ao modelo, podendo estourar a janela (a janela preserva a última mensagem).

## Requisitos (EARS)
- **FR-CAP-001**: O resultado de tool que entra no histórico MUST ser limitado a `Config.ToolResultMaxBytes` (default 32 KiB).
- **FR-CAP-002**: Ao cortar, o resultado MUST marcar `Truncated=true` e preservar os primeiros bytes.
- **FR-CAP-003**: Teto `<= 0` desliga o limite (comportamento anterior opt-in); o resumo do evento continua limitado a 4096.
- **FR-CAP-004**: Resultado já truncado MUST NOT ser re-cortado.

## Critérios (Gherkin)
```gherkin
Cenário: resultado grande
  Dado uma tool que devolve mais que o teto
  Quando o resultado entra no histórico
  Então o conteúdo é cortado no teto
  E Truncated é verdadeiro
```

## Não-objetivos
- Sumarizar o resultado cortado (futuro).
- Teto por tool (o catálogo já tem Timeout; teto é global no v1).
