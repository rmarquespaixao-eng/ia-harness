package openai

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordSink registra os deltas recebidos (feature 012).
type recordSink struct {
	texts     []string
	reasoning []string
	args      []string
}

func (s *recordSink) Text(t string)      { s.texts = append(s.texts, t) }
func (s *recordSink) Reasoning(t string) { s.reasoning = append(s.reasoning, t) }
func (s *recordSink) ToolCallArgs(_, _, fragment string) {
	s.args = append(s.args, fragment)
}

func TestParseStream_EmiteArgsEReasoning(t *testing.T) {
	// Arrange — reasoning_content + tool_calls com arguments fragmentados.
	const sse = "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"penso\"},\"finish_reason\":null}]}\n" +
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"c1\",\"function\":{\"name\":\"buscar\",\"arguments\":\"{\\\"q\\\":\"}}]},\"finish_reason\":null}]}\n" +
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"\\\"x\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n" +
		"data: [DONE]\n\n"
	sink := &recordSink{}

	// Act
	resp, err := parseStream(context.Background(), strings.NewReader(sse), "m", sink)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"penso"}, sink.reasoning)
	assert.Len(t, sink.args, 2)
	assert.Equal(t, `{"q":"x"}`, strings.Join(sink.args, ""))
	require.Len(t, resp.ToolCalls, 1)
	assert.Equal(t, "c1", resp.ToolCalls[0].ID)
}
