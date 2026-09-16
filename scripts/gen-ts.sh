#!/usr/bin/env bash
# gen-ts.sh — gera os tipos TypeScript dos JSON Schemas canônicos
# (contracts/<assunto>/*.json) para consumo do financeiro-ui-v2 (T082/D-11,
# ADR 0006): o schema é a fonte única e o nome do tipo vem do "title".
#
# Uso: scripts/gen-ts.sh
# Saída: contracts/gen-ts/<assunto>/<schema>.ts + index.ts (diretório
# gitignored — artefato gerado, regenerável a qualquer momento).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCHEMAS_DIR="$ROOT/contracts"
OUT_DIR="$SCHEMAS_DIR/gen-ts"

if command -v json2ts >/dev/null 2>&1; then
  JSON2TS=(json2ts)
elif command -v npx >/dev/null 2>&1; then
  JSON2TS=(npx -y json-schema-to-typescript@15)
else
  echo "gen-ts: instale json-schema-to-typescript (npm i -g) ou tenha npx disponível" >&2
  exit 1
fi

mapfile -t schemas < <(find "$SCHEMAS_DIR" -mindepth 2 -maxdepth 2 -name '*.json' -print | sort)
if [ "${#schemas[@]}" -eq 0 ]; then
  echo "gen-ts: nenhum schema encontrado em $SCHEMAS_DIR/<assunto>/*.json" >&2
  exit 1
fi

rm -rf -- "$OUT_DIR"
mkdir -p "$OUT_DIR"

index_entries=()
for schema in "${schemas[@]}"; do
  subject="$(basename "$(dirname "$schema")")"
  name="$(basename "$schema" .json)"
  target="$OUT_DIR/$subject"
  mkdir -p "$target"
  echo "gen-ts: $subject/$name.json -> contracts/gen-ts/$subject/$name.ts"
  "${JSON2TS[@]}" "$schema" -o "$target/$name.ts"
  index_entries+=("./$subject/$name")
done

{
  echo "// Gerado por scripts/gen-ts.sh — não editar à mão."
  echo "// Fonte: contracts/<assunto>/*.json (JSON Schema é o contrato — ADR 0006)."
  for entry in "${index_entries[@]}"; do
    echo "export * from \"$entry\";"
  done
} > "$OUT_DIR/index.ts"

echo "gen-ts: ${#index_entries[@]} schema(s) gerados em contracts/gen-ts/index.ts"
