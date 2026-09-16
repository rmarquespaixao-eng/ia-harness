package engine

// truncateToolResult limita o conteúdo de um resultado de tool ao teto
// configurado, marcando Truncated quando corta (feature 013). Um resultado já
// truncado ou sem teto é devolvido como veio.
func truncateToolResult(res ToolResult, maxBytes int) ToolResult {
	if maxBytes <= 0 || res.Truncated {
		return res
	}
	used := 0
	out := make([]ResultContent, 0, len(res.Content))
	for _, content := range res.Content {
		size := len(content.Text) + len(content.JSON)
		if used+size <= maxBytes {
			out = append(out, content)
			used += size
			continue
		}
		remaining := maxBytes - used
		if remaining <= 0 {
			res.Truncated = true
			break
		}
		if content.Kind == ResultJSON {
			cut := remaining
			if cut > len(content.JSON) {
				cut = len(content.JSON)
			}
			out = append(out, ResultContent{Kind: content.Kind, JSON: append([]byte(nil), content.JSON[:cut]...)})
		} else {
			cut := remaining
			if cut > len(content.Text) {
				cut = len(content.Text)
			}
			out = append(out, ResultContent{Kind: content.Kind, Text: content.Text[:cut]})
		}
		res.Truncated = true
		break
	}
	res.Content = out
	return res
}
