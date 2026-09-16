package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rmarquespaixao-eng/ia-harness/harness"
)

// Call executa uma tool no servidor MCP. args nil vira objeto vazio; o
// progresso do servidor é correlacionado pelo progressToken da chamada e
// entregue a onProgress antes do retorno (FR-009). Falha de transporte é
// devolvida como erro; isError do servidor vira ToolResult.IsError (FR-011).
func (c *Client) Call(ctx context.Context, name string, args json.RawMessage, onProgress func(harness.ProgressUpdate)) (harness.ToolResult, error) {
	arguments := map[string]any{}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &arguments); err != nil {
			return harness.ToolResult{}, fmt.Errorf("mcpclient: argumentos de %q não são objeto JSON: %w", name, err)
		}
	}

	var result harness.ToolResult
	err := c.withReconnect(ctx, func(ctx context.Context, cs *mcp.ClientSession) error {
		res, err := c.callTool(ctx, cs, name, arguments, onProgress)
		if err != nil {
			return err
		}
		result = res
		return nil
	})
	if err != nil {
		return harness.ToolResult{}, err
	}
	return result, nil
}

// callTool registra o callback de progresso no token da chamada, invoca o
// servidor e converte o resultado para o modelo canônico.
func (c *Client) callTool(ctx context.Context, cs *mcp.ClientSession, name string, arguments map[string]any, onProgress func(harness.ProgressUpdate)) (harness.ToolResult, error) {
	params := &mcp.CallToolParams{Name: name, Arguments: arguments}
	if onProgress != nil {
		token := c.progress.register(onProgress)
		defer c.progress.unregister(token)
		params.SetProgressToken(token)
	}

	res, err := cs.CallTool(ctx, params)
	if err != nil {
		return harness.ToolResult{}, fmt.Errorf("mcpclient: chamar tool %q: %w", name, err)
	}
	return mapResult(res)
}

// mapResult converte o resultado do SDK: texto vira ResultText e o conteúdo
// estruturado vira ResultJSON; binário não é propagado (harness/session.go).
func mapResult(res *mcp.CallToolResult) (harness.ToolResult, error) {
	result := harness.ToolResult{IsError: res.IsError}
	for _, content := range res.Content {
		text, ok := content.(*mcp.TextContent)
		if !ok || text == nil {
			continue
		}
		result.Content = append(result.Content, harness.ResultContent{Kind: harness.ResultText, Text: text.Text})
	}
	if res.StructuredContent != nil {
		raw, err := json.Marshal(res.StructuredContent)
		if err != nil {
			return harness.ToolResult{}, fmt.Errorf("mcpclient: conteúdo estruturado da tool: %w", err)
		}
		result.Content = append(result.Content, harness.ResultContent{Kind: harness.ResultJSON, JSON: raw})
	}
	return result, nil
}

// progressRegistry correlaciona progressToken → callback ativo da chamada.
type progressRegistry struct {
	mu    sync.Mutex
	seq   uint64
	calls map[string]func(harness.ProgressUpdate)
}

// newProgressRegistry cria o registro vazio.
func newProgressRegistry() *progressRegistry {
	return &progressRegistry{calls: map[string]func(harness.ProgressUpdate){}}
}

// register associa o callback a um token único e devolve o token.
func (r *progressRegistry) register(cb func(harness.ProgressUpdate)) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	token := fmt.Sprintf("ia-harness-%d", r.seq)
	r.calls[token] = cb
	return token
}

// unregister remove o callback do token.
func (r *progressRegistry) unregister(token string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.calls, token)
}

// dispatch entrega a atualização ao callback do token, se houver.
func (r *progressRegistry) dispatch(token any, update harness.ProgressUpdate) {
	r.mu.Lock()
	cb := r.calls[tokenKey(token)]
	r.mu.Unlock()
	if cb != nil {
		cb(update)
	}
}

// tokenKey normaliza o token recebido (string ou número após o round-trip JSON).
func tokenKey(token any) string {
	switch t := token.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return t.String()
	case int, int32, int64, uint, uint32, uint64:
		return fmt.Sprint(t)
	default:
		raw, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprint(t)
		}
		return string(raw)
	}
}
