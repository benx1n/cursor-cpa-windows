package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReasoningIsSeparateAndIncludedInEstimatedUsage(t *testing.T) {
	turn := NewTurn("cursor/auto")
	chunk, err := turn.StreamReasoning(strings.Repeat("thinking ", 20))
	require.NoError(t, err)
	var delta chatCompletion
	require.NoError(t, json.Unmarshal(chunk, &delta))
	require.Empty(t, delta.Choices[0].Delta.Content)
	require.NotEmpty(t, delta.Choices[0].Delta.ReasoningContent)
	turn.AddReasoning("more thinking")
	turn.AddText("answer")
	output, err := turn.Completion("prompt")
	require.NoError(t, err)
	var completed chatCompletion
	require.NoError(t, json.Unmarshal(output, &completed))
	require.Equal(t, "answer", completed.Choices[0].Message.Content)
	require.Contains(t, completed.Choices[0].Message.ReasoningContent, "more thinking")
	require.Greater(t, completed.Usage.CompletionTokens, 40)
}
