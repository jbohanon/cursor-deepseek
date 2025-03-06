package deepseek

import (
	"context"

	"github.com/danilofalcao/cursor-deepseek/internal/api/deepseek/v1"
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

		// Handle assistant messages with tool calls
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			lgr.Debugf(ctx, "Processing assistant message with %d tool calls", len(msg.ToolCalls))
			// DeepSeek expects tool_calls in a specific format
			toolCalls := make([]deepseek.ToolCall, len(msg.ToolCalls))
			for j, tc := range msg.ToolCalls {
				toolCalls[j] = deepseek.ToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: deepseek.ToolCallFunction{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
				lgr.Debugf(ctx, "Tool call %d - ID: %s, Function: %s", j, tc.ID, tc.Function.Name)
			}
			converted[i].ToolCalls = toolCalls
		}

		// Handle function response messages
		if msg.Role == "function" {
			lgr.Debugf(ctx, "Converting function response to tool response")
			// Convert to tool response format
			converted[i].Role = "tool"
		}
	}

	// Log the final converted messages
	for i, msg := range converted {
		lgr.Debugf(ctx, "Final message %d - Role: %s, Content: %s", i, msg.Role, truncateString(msg.Content, 50))
		if len(msg.ToolCalls) > 0 {
			lgr.Debugf(ctx, "Message %d has %d tool calls", i, len(msg.ToolCalls))
		}
	}

	return converted
}

func convertResponseChoices(ctx context.Context, choices []deepseek.Choice) []openai.Choice {
	openaiChoices := make([]openai.Choice, len(choices))
	for i, choice := range choices {
		openaiChoices[i] = openai.Choice{
			Index:        choice.Index,
			Message:      convertResponseMessage(ctx, choice.Message),
			FinishReason: choice.FinishReason,
		}
	}
	return openaiChoices
}

func convertResponseMessage(ctx context.Context, message deepseek.Message) openai.Message {
	return openai.Message{
		Role: message.Role,
		Content: openai.Content_String{
			Content: message.Content,
		},
		ToolCalls:  convertResponseToolCalls(ctx, message.ToolCalls),
		ToolCallID: message.ToolCallID,
		Name:       message.Name,
	}
}

func convertResponseToolCalls(ctx context.Context, toolCalls []deepseek.ToolCall) []openai.ToolCall {
	lgr := logutils.FromContext(ctx)
	openaiToolCalls := make([]openai.ToolCall, 0)
	for i, tc := range toolCalls {
		lgr.Debugf(ctx, "Tool call %d: %+v", i, tc)
		// Ensure the tool call has the required fields
		if tc.Function.Name == "" {
			lgr.Debugf(ctx, "Warning: Empty function name in tool call %d", i)
			continue
		}
		openaiToolCalls = append(openaiToolCalls, openai.ToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: openai.ToolCallFunction{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		})
	}
	return openaiToolCalls
}
