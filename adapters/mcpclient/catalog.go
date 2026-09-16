package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"rmarquespaixao/ia-harness/harness"
)

// List descobre o catálogo do servidor e o publica namespaceado com
// Config.Name; o resultado é cacheado por sessão e invalidado na reconexão
// (FR-005).
func (c *Client) List(ctx context.Context) ([]harness.Tool, error) {
	if cached := c.cachedCatalog(); cached != nil {
		return cached, nil
	}

	var catalog []harness.Tool
	err := c.withReconnect(ctx, func(ctx context.Context, cs *mcp.ClientSession) error {
		tools, err := c.fetchTools(ctx, cs)
		if err != nil {
			return err
		}
		catalog = tools
		return nil
	})
	if err != nil {
		return nil, err
	}
	return catalog, nil
}

// cachedCatalog devolve o catálogo da sessão atual, se já descoberto.
func (c *Client) cachedCatalog() []harness.Tool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session == nil {
		return nil
	}
	return c.catalog
}

// fetchTools percorre as páginas de tools/list e converte para o modelo
// canônico, publicando o cache da sessão ao final.
func (c *Client) fetchTools(ctx context.Context, cs *mcp.ClientSession) ([]harness.Tool, error) {
	var catalog []harness.Tool
	params := &mcp.ListToolsParams{}
	for {
		result, err := cs.ListTools(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("mcpclient: listar tools de %q: %w", c.cfg.Name, err)
		}
		for _, tool := range result.Tools {
			if tool == nil {
				continue
			}
			schema, err := marshalSchema(tool.InputSchema)
			if err != nil {
				return nil, fmt.Errorf("mcpclient: schema da tool %q: %w", tool.Name, err)
			}
			catalog = append(catalog, harness.Tool{
				Name:        tool.Name,
				Namespace:   c.cfg.Name,
				Description: tool.Description,
				InputSchema: schema,
				Timeout:     c.cfg.ToolTimeout,
			})
		}
		if result.NextCursor == "" {
			break
		}
		params.Cursor = result.NextCursor
	}

	c.mu.Lock()
	c.catalog = catalog
	c.mu.Unlock()
	return catalog, nil
}

// marshalSchema serializa o schema dinâmico da tool para json.RawMessage.
func marshalSchema(schema any) (json.RawMessage, error) {
	if schema == nil {
		return nil, nil
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	return raw, nil
}
