# Feature Specification: Retry classificado nas chamadas de provider

**Feature Branch**: `nucleo/007-retry-provider`

**Created**: 2026-09-16

**Status**: Aprovado e implementado (pacote P0 aprovado pelo operador)

**Input**: lacuna P0 do diagnóstico "o que falta pro básico": providers não repetiam erros transitórios (só o `mcpclient` tinha retry). Um 429 derrubava o turno; o fallback não é retry.

## 1. Problema

Os adaptadores de modelo faziam uma única tentativa por chamada. Erros transitórios (429, timeout, 5xx, falha de transporte) encerravam o turno mesmo com o provedor voltando em instantes. A comunidade trata isso como básico (todo client de provider repete com backoff).

## 2. Requisitos (EARS)

- **FR-RETRY-001**: WHEN a chamada de modelo falha com erro transitório (429, timeout, 5xx ou transporte), o adaptador SHALL repetir com backoff exponencial + jitter, até `Config.MaxAttempts`.
- **FR-RETRY-002**: O adaptador MUST NOT repetir erros permanentes (4xx diferentes de 429) nem cancelamento de contexto.
- **FR-RETRY-003**: O retry MUST ocorrer **antes de consumir o stream**; falha no meio do stream não é repetida (evita duplicar texto/cobrança/eventos).
- **FR-RETRY-004**: O retry MUST ser configurável (`MaxAttempts`, `RetryBaseDelay`, `RetryMaxDelay`); default resiliente (`MaxAttempts=3`, base 200ms, teto 2s).
- **FR-RETRY-005**: A classificação MUST reusar os códigos estáveis de `*ProviderError` e a última falha MUST preservar a causa (`errors.As`/`Is`).

## 3. Critérios de aceite (Gherkin)

```gherkin
Cenário: 429 seguido de sucesso
  Dado um provedor que responde 429 duas vezes e 200 na terceira
  Quando o adaptador chama o modelo
  Então ele repete até obter sucesso
  E o turno conclui

Cenário: erro permanente não repetido
  Dado um provedor que responde 400
  Quando o adaptador chama o modelo
  Então há uma única tentativa
  E o erro preserva o código provider_http

Cenário: cancelamento não repetido
  Dado um contexto cancelado
  Quando a chamada falha
  Então não há nova tentativa
```

## 4. Não-objetivos

- Retry de tool MCP (o `mcpclient` já reconecta).
- Retry de stream parcial (por design não repetido).
- Circuit breaker / rate limiting (evolução).
