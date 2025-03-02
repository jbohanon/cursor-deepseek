package ollama

import (
	ollama "github.com/danilofalcao/cursor-deepseek/internal/api/ollama/v1"
	"github.com/danilofalcao/cursor-deepseek/internal/api/openai/v1"
)

func convertMessages(messages []openai.Message) []ollama.Message {
	ollamaMessages := make([]ollama.Message, len(messages))
	for i, message := range messages {
		var content string
		switch message.GetContent().(type) {
		case openai.Content_String:
			content = message.GetContentString()
		case openai.Content_Array:
			contentArray := message.GetContentArray()
			for i := range contentArray {
				t := contentArray.GetContentPartTextAtIndex(i).Text
				if t != "" {
					content += "; " + t
				}
			}
		}
		ollamaMessages[i] = ollama.Message{
			Role:    message.Role,
			Content: content,
		}
	}
	return ollamaMessages
}
