# scripts

## `gen-ts.sh` — tipos TypeScript dos contratos (T082)

Gera os tipos TypeScript dos JSON Schemas canônicos em `contracts/<assunto>/*.json`
para consumo do `financeiro-ui-v2` (ADR 0006: o schema é a fonte única; a UI não
redescobre o formato de eventos/sessão). O nome de cada tipo vem do `title` do
schema, e um `index.ts` exporta todos.

```bash
scripts/gen-ts.sh
```

- Pré-requisitos: `json2ts` no PATH ou `npx` (usa `npx -y json-schema-to-typescript`).
- Saída: `contracts/gen-ts/<assunto>/<schema>.ts` + `contracts/gen-ts/index.ts`.
- O diretório `contracts/gen-ts/` é **gitignored**: artefato gerado, regenerável.
- No host, aponte o `tsconfig` (ou copie o diretório) para o `financeiro-ui-v2`;
  nunca edite os `.ts` à mão — regenere a partir dos schemas.
