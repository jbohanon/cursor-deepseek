package openrouter

import (
	"context"

	deepseek "github.com/danilofalcao/cursor-deepseek/internal/api/deepseek/v1"
	"github.com/danilofalcao/cursor-deepseek/internal/api/openai/v1"
	logutils "github.com/danilofalcao/cursor-deepseek/internal/utils/logger"
)

func convertTools(tools []openai.Tool) []deepseek.Tool {
	converted := make([]deepseek.Tool, len(tools))
	for i, tool := range tools {
		converted[i] = deepseek.Tool{
			Type: tool.Type,
			Function: deepseek.Function{
				Name:        tool.Function.Name,
				Parameters:  tool.Function.Parameters,
				Description: tool.Function.Description,
			},
		}
	}
	return converted
}

func convertToolChoice(choice interface{}) string {
	if choice == nil {
		return ""
	}

	// If string "auto" or "none"
	if str, ok := choice.(string); ok {
		switch str {
		case "auto", "none":
			return str
		}
	}

	// Try to parse as map for function call
	if choiceMap, ok := choice.(map[string]interface{}); ok {
		if choiceMap["type"] == "function" {
			return "auto" // DeepSeek doesn't support specific function selection, default to auto
		}
	}

	return ""
}

func convertToolCalls(toolCalls []openai.ToolCall, toolType string) []deepseek.ToolCall {
	getToolType := func(toolCall openai.ToolCall) string {
		if toolType != "" {
			return toolType
		}
		return toolCall.Type
	}
	converted := make([]deepseek.ToolCall, len(toolCalls))
	for i, tc := range toolCalls {
		converted[i] = deepseek.ToolCall{
			ID:   tc.ID,
			Type: getToolType(tc),
			Function: deepseek.ToolCallFunction{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		}
	}
	return converted
}

func convertMessages(ctx context.Context, messages []openai.Message) []deepseek.Message {
	lgr := logutils.FromContext(ctx)
	converted := make([]deepseek.Message, len(messages))
	for i, msg := range messages {
		lgr.Debugf(ctx, "Converting message %d - Role: %s", i, msg.Role)
		var content string
		switch msg.GetContent().(type) {
		case openai.Content_String:
			content = msg.GetContentString()
		case openai.Content_Array:
			contentArray := msg.GetContentArray()
			for i := range contentArray {
				t := contentArray.GetContentPartTextAtIndex(i).Text
				if t != "" {
					content += "; " + t
				}
			}
		}
		converted[i] = deepseek.Message{
			Role:       msg.Role,
			Content:    content,
			ToolCallID: msg.ToolCallID,
			Name:       msg.Name,
		}

		// Convert function role to tool role
		if msg.Role == "function" {
			converted[i].Role = "tool"
			converted[i].ToolCalls = convertToolCalls(msg.ToolCalls, "")
			lgr.Debug(ctx, "Converted function role to tool role")
		}

		// Handle assistant messages with tool calls
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			lgr.Debugf(ctx, "Processing assistant message with %d tool calls", len(msg.ToolCalls))

			// Ensure tool calls are properly formatted
			converted[i].ToolCalls = convertToolCalls(msg.ToolCalls, "function")
		}

		// Handle tool response messages
		if msg.Role == "tool" || msg.Role == "function" {
			lgr.Debugf(ctx, "Processing tool/function response message")
			converted[i].Role = "tool"
			if msg.Name != "" {
				lgr.Debugf(ctx, "Tool response from function: %s", msg.Name)
			}
		}
	}

	return converted
}
