// Package gen contém os tipos gerados a partir dos JSON Schemas versionados em
// contracts/ (fonte única do contrato — ADR 0006). Não editar à mão: o gate
// `go generate ./...` falha se o gerado divergir do commitado.
package gen

//go:generate sh -c "cd ../.. && go run ./cmd/contractgen"
