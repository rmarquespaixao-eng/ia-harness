# Feature Specification: Middleware/filtros (decorators)

**Feature Branch**: `nucleo/014-middleware` · **Status**: Aprovado e implementado

## Problema
Guardrails, logging e telemetria exigiam editar o núcleo. A comunidade expõe middleware/filtros.

## Requisitos (EARS)
- **FR-MW-001**: O harness MUST expor `ProviderMiddleware`, `ToolSourceMiddleware` e `HandlerMiddleware` (decorators).
- **FR-MW-002**: `Config.ProviderMiddleware`/`ToolMiddleware` MUST envolver os providers/tools no boot (em ordem), sem mutar as estruturas do host.
- **FR-MW-003**: `Config.HandlerMiddleware` MUST envolver o `Handler` a cada turno (onion).
- **FR-MW-004**: Middleware MUST NOT alterar a semântica padrão quando ausente.

## Critérios (Gherkin)
```gherkin
Cenário: decorator de provider
  Dado um ProviderMiddleware que conta chamadas
  Quando um turno roda
  Então o middleware envolve o provider e a chamada é contada
```

## Não-objetivos
- Middleware como plugin dinâmico (hot-reload).
- Interceptação por etapa interna do loop (além do Handler).
