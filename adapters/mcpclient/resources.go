package mcpclient

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Resource descreve um recurso MCP (feature 017/ADR 0019).
type Resource struct {
	URI         string
	Name        string
	Title       string
	Description string
	MIMEType    string
}

// ResourceContent é o conteúdo textual lido de um recurso (blobs não são
// propagados — o host trata binário).
type ResourceContent struct {
	URI      string
	MIMEType string
	Text     string
}

// Prompt descreve um prompt MCP.
type Prompt struct {
	Name        string
	Title       string
	Description string
	Arguments   []PromptArgument
}

// PromptArgument descreve um argumento de prompt.
type PromptArgument struct {
	Name        string
	Title       string
	Description string
	Required    bool
}

// PromptMessage é uma mensagem devolvida por um prompt (texto).
type PromptMessage struct {
	Role string
	Text string
}

// ListResources lista os recursos publicados pelo servidor.
func (c *Client) ListResources(ctx context.Context) ([]Resource, error) {
	var out []Resource
	err := c.withReconnect(ctx, func(ctx context.Context, cs *mcp.ClientSession) error {
		result, err := cs.ListResources(ctx, nil)
		if err != nil {
			return err
		}
		out = out[:0]
		for _, r := range result.Resources {
			if r == nil {
				continue
			}
			out = append(out, Resource{
				URI: r.URI, Name: r.Name, Title: r.Title,
				Description: r.Description, MIMEType: r.MIMEType,
			})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("mcpclient: listar recursos de %q: %w", c.cfg.Name, err)
	}
	return out, nil
}

// ReadResource lê o conteúdo textual de um recurso por URI.
func (c *Client) ReadResource(ctx context.Context, uri string) ([]ResourceContent, error) {
	var out []ResourceContent
	err := c.withReconnect(ctx, func(ctx context.Context, cs *mcp.ClientSession) error {
		result, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
		if err != nil {
			return err
		}
		out = out[:0]
		for _, content := range result.Contents {
			if content == nil {
				continue
			}
			out = append(out, ResourceContent{URI: content.URI, MIMEType: content.MIMEType, Text: content.Text})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("mcpclient: ler recurso %q de %q: %w", uri, c.cfg.Name, err)
	}
	return out, nil
}

// ListPrompts lista os prompts publicados pelo servidor.
func (c *Client) ListPrompts(ctx context.Context) ([]Prompt, error) {
	var out []Prompt
	err := c.withReconnect(ctx, func(ctx context.Context, cs *mcp.ClientSession) error {
		result, err := cs.ListPrompts(ctx, nil)
		if err != nil {
			return err
		}
		out = out[:0]
		for _, p := range result.Prompts {
			if p == nil {
				continue
			}
			out = append(out, Prompt{
				Name: p.Name, Title: p.Title, Description: p.Description,
				Arguments: promptArguments(p.Arguments),
			})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("mcpclient: listar prompts de %q: %w", c.cfg.Name, err)
	}
	return out, nil
}

// GetPrompt materializa um prompt com os argumentos informados.
func (c *Client) GetPrompt(ctx context.Context, name string, args map[string]string) ([]PromptMessage, error) {
	var out []PromptMessage
	err := c.withReconnect(ctx, func(ctx context.Context, cs *mcp.ClientSession) error {
		result, err := cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: name, Arguments: args})
		if err != nil {
			return err
		}
		out = out[:0]
		for _, msg := range result.Messages {
			if msg == nil {
				continue
			}
			out = append(out, PromptMessage{Role: string(msg.Role), Text: contentText(msg.Content)})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("mcpclient: obter prompt %q de %q: %w", name, c.cfg.Name, err)
	}
	return out, nil
}

// promptArguments converte os argumentos do SDK.
func promptArguments(args []*mcp.PromptArgument) []PromptArgument {
	if len(args) == 0 {
		return nil
	}
	out := make([]PromptArgument, 0, len(args))
	for _, a := range args {
		if a == nil {
			continue
		}
		out = append(out, PromptArgument{Name: a.Name, Title: a.Title, Description: a.Description, Required: a.Required})
	}
	return out
}

// contentText extrai o texto de um content block (não-texto é ignorado).
func contentText(content mcp.Content) string {
	if text, ok := content.(*mcp.TextContent); ok {
		return text.Text
	}
	return ""
}
