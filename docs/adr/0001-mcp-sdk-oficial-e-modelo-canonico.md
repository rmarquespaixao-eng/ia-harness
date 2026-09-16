# ADR 0001 — SDK oficial MCP e modelo canônico com adaptadores por provedor

**Status**: Accepted
**Data**: 2026-09-15

## Contexto

O harness precisa (a) falar MCP com servidores externos — o primeiro host expõe ~96 tools via Streamable HTTP, com notificações de progresso em operações longas — e (b) falar com múltiplos provedores de LLM sem que seus tipos cruzem a API pública. Também precisa validar argumentos de tools contra o JSON Schema que cada tool publica: o schema é dinâmico (vem do servidor), então validação por struct tags (`validator/v10`) não se aplica.

Três incógnitas foram resolvidas na pesquisa R1–R3: qual cliente MCP adotar, como abstrair provedores e qual validador de JSON Schema usar. Restrições: árvore de dependências pequena (constitution §2), nenhum tipo de SDK vazando na API pública, testes determinísticos sem rede (FR-030) e conformidade de protocolo com o servidor MCP que já roda no `financeiro-api-v2` (SDK v1.8.x, `/mcp` com SSE).

## Decisão

1. **Cliente MCP**: adotar o SDK oficial `github.com/modelcontextprotocol/go-sdk` (pacote `mcp`), pinado na linha v1.8.x do financeiro, com transporte `StreamableClientTransport`, sessão persistente com reconexão (`MaxRetries`), progresso via `ProgressNotificationHandler` e testes com `mcp.NewInMemoryTransports()`. Versão recente obrigatória e `govulncheck` no gate, por causa dos advisories de versões antigas (GO-2026-5771/4770/4773).
2. **Modelo canônico próprio + adaptadores**: o núcleo define `ChatRequest`, `ChatResponse`, `TextDelta`, `ToolCall`, `Usage` e `StopReason`; os adaptadores `adapters/provider/openai` (API compatível OpenAI — cobre OpenAI, OpenRouter, Groq, DeepSeek, Ollama, vLLM, LM Studio) e `adapters/provider/anthropic` (Messages API) convertem o formato nativo para o modelo canônico. Trocar de provedor é configuração, não código (SC-002/SC-008).
3. **Validação de argumentos de tool**: `github.com/santhosh-tekuri/jsonschema/v6`, compilando o schema publicado pela tool uma vez na descoberta e reutilizando a instância compilada a cada chamada.

## Alternativas consideradas

| Alternativa | Motivo da rejeição |
|---|---|
| SDKs MCP de terceiros (`mark3labs/mcp-go`, `modelcontextprotocol-ce/go-sdk`) | Menor garantia de conformidade e de manutenção; o SDK oficial é mantido com Google e já validado server-side pelo financeiro. |
| Cliente JSON-RPC próprio | Custo alto de implementação e manutenção sem ganho funcional. |
| SDK oficial de cada provedor de LLM | Peso de dependências, lock-in e tipos de SDK vazando na API pública (viola a restrição de API estável). |
| `langchaingo` e libs de orquestração | Abstração larga demais e ciclo de release alheio ao núcleo. |
| `gojsonschema`/`xeipuuv` para validação | Sem manutenção ativa; o `santhosh-tekuri/v6` passa a suíte oficial de testes e cobre drafts recentes. |
| Validação manual dos argumentos | Frágil, não cobre o JSON Schema publicado pela tool e vira dívida a cada schema novo. |

## Consequências

**Positivas**
- Cliente e servidor negociam a mesma linha de protocolo (v1.8.x) sem surpresa; progresso de tools longas chega ao host.
- O transporte em memória viabiliza testes de integração do loop determinísticos e sem rede.
- Um adaptador OpenAI-compatible cobre a maior parte do mercado e do homelab; o modelo canônico blinda a API pública de mudanças de fornecedor.
- Schema compilado uma vez por tool mantém a validação de argumentos barata no caminho quente (FR-005).

**Negativas / trade-offs**
- O histórico de advisories do SDK exige pin recente e `govulncheck` contínuo (R9).
- Dois adaptadores são mantidos no repo; famílias de API novas exigem novo adaptador.
- Traduzir o streaming nativo para eventos canônicos concentra a maior parte da complexidade dos adaptadores (R4).
- O adaptador OpenAI-compatible depende de cada provedor honrar o mesmo dialeto de `tools` e streaming; divergências viram fixtures de contrato (R5).
