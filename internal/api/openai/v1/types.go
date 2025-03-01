package v1

import (
	"encoding/json"
	"log"
)

// ChatCompletionRequest represents an OpenAI-compatible chat completion request
type ChatCompletionRequest struct {
	Model       string     `json:"model"`
	Messages    []Message  `json:"messages"`
	Stream      bool       `json:"stream"`
	Temperature *float64   `json:"temperature,omitempty"`
	MaxTokens   *int       `json:"max_tokens,omitempty"`
	Functions   []Function `json:"functions,omitempty"`
	Tools       []Tool     `json:"tools,omitempty"`
	ToolChoice  any        `json:"tool_choice,omitempty"`
}

// Function represents a callable function
type Function struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

// Tool represents an available tool
type Tool struct {
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

// ToolCall represents a call to a tool
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ChatCompletionResponse represents a chat completion response
type ChatCompletionResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// ChatCompletionStreamResponse represents a chat completion streaming response
type ChatCompletionStreamResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []StreamChoice `json:"choices"`
	Usage   Usage          `json:"usage"`
}
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Choice represents a completion choice
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// StreamChoice represents a streaming completion choice
type StreamChoice struct {
	Index        int    `json:"index"`
	Delta        Delta  `json:"delta"`
	FinishReason string `json:"finish_reason,omitempty"`
}

// Delta represents a streaming response delta
type Delta struct {
	Role string `json:"role,omitempty"`
	// Stealing this convention from protobuf because it's what I know,
	// but it could probably be improved.
	// Types that are valid to be assigned to Content:
	// - *Content_String
	// - *Content_Array
	Content isContent `json:"content"`
}

func (d *Delta) MarshalJSON() ([]byte, error) {

	msgMap := map[string]interface{}{
		"role": d.Role,
	}

	switch d.Content.(type) {
	case Content_String:
		msgMap["content"] = d.Content.(Content_String).Content
	case Content_Array:
		msgMap["content"] = d.Content.(Content_Array)
	}

	return json.Marshal(msgMap)
}
func (d *Delta) UnmarshalJSON(data []byte) error {
	var err error
	var msg map[string]interface{}
	if err = json.Unmarshal(data, &msg); err != nil {
		return err
	}

	if msg["role"] != nil {
		d.Role = msg["role"].(string)
	}

	if msg["content"] != nil {
		switch msg["content"].(type) {
		case string:
			d.Content = Content_String{Content: msg["content"].(string)}
		case []interface{}:
			contentArray := msg["content"].([]interface{})
			contentArrayParts := make(Content_Array, len(contentArray))
			for i, contentPart := range contentArray {
				switch val := contentPart.(type) {
				case map[string]interface{}:
					if val["type"] == "text" {
						contentArrayParts[i] = ContentPart_Text{Type: "text", Text: val["text"].(string)}
					} else {
						log.Printf("Unknown content part type: %s", val["type"])
					}
				default:
					log.Printf("Unknown content part type: %T", val)
				}
			}
			d.Content = contentArrayParts
		}
	}

	return nil
}

// Models response structure
type ModelsResponse struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// Message represents a chat message
type Message struct {
	Role string `json:"role"`
	// Stealing this convention from protobuf because it's what I know,
	// but it could probably be improved.
	// Types that are valid to be assigned to Content:
	// - *Content_String
	// - *Content_Array
	Content    isContent  `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

func (m *Message) MarshalJSON() ([]byte, error) {
	toolCallsBytes, err := json.Marshal(m.ToolCalls)
	if err != nil {
		return nil, err
	}

	toolCallsIface := []interface{}{}
	if err := json.Unmarshal(toolCallsBytes, &toolCallsIface); err != nil {
		return nil, err
	}

	msgMap := map[string]interface{}{
		"role":         m.Role,
		"tool_calls":   toolCallsIface,
		"tool_call_id": m.ToolCallID,
		"name":         m.Name,
	}

	switch m.Content.(type) {
	case Content_String:
		msgMap["content"] = m.Content.(Content_String).Content
	case Content_Array:
		msgMap["content"] = m.Content.(Content_Array)
	}

	return json.Marshal(msgMap)
}
func (m *Message) UnmarshalJSON(data []byte) error {
	var err error
	var msg map[string]interface{}
	if err = json.Unmarshal(data, &msg); err != nil {
		return err
	}

	if msg["role"] != nil {
		m.Role = msg["role"].(string)
	}

	if msg["content"] != nil {
		switch msg["content"].(type) {
		case string:
			m.Content = Content_String{Content: msg["content"].(string)}
		case []interface{}:
			contentArray := msg["content"].([]interface{})
			contentArrayParts := make(Content_Array, len(contentArray))
			for i, contentPart := range contentArray {
				switch val := contentPart.(type) {
				case map[string]interface{}:
					if val["type"] == "text" {
						contentArrayParts[i] = ContentPart_Text{Type: "text", Text: val["text"].(string)}
					} else {
						log.Printf("Unknown content part type: %s", val["type"])
					}
				default:
					log.Printf("Unknown content part type: %T", val)
				}
			}
			m.Content = contentArrayParts
		}
	}

	if msg["tool_calls"] != nil {
		var toolCalls []ToolCall
		if toolCallsInterface, ok := msg["tool_calls"].([]interface{}); ok {
			for _, toolCall := range toolCallsInterface {
				toolCallMap := toolCall.(map[string]interface{})
				toolCalls = append(toolCalls, ToolCall{
					ID:       toolCallMap["id"].(string),
					Type:     toolCallMap["type"].(string),
					Function: ToolCallFunction{Name: toolCallMap["function"].(map[string]interface{})["name"].(string), Arguments: toolCallMap["function"].(map[string]interface{})["arguments"].(string)},
				})
			}
		}
		m.ToolCalls = toolCalls
	}

	if msg["tool_call_id"] != nil {
		m.ToolCallID = msg["tool_call_id"].(string)
	}

	if msg["name"] != nil {
		m.Name = msg["name"].(string)
	}

	return nil
}

func (m *Message) GetContent() isContent {
	if m != nil {
		return m.Content
	}

	return nil
}

func (m *Message) GetContentString() string {
	if m != nil {
		if c, ok := m.Content.(Content_String); ok {
			return c.Content
		}
	}
	return ""
}

func (m *Message) GetContentArray() Content_Array {
	if m != nil {
		if c, ok := m.Content.(Content_Array); ok {
			return c
		}
	}
	return nil
}

type isContent interface {
	isContent()
}

type Content_String struct {
	Content string
}

func (c Content_String) isContent() {}

type Content_Array []isContentPart

func (c Content_Array) isContent() {}

// Currently only text is supported
type isContentPart interface {
	isContentPart()
}

type ContentPart_Text struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (c ContentPart_Text) isContentPart() {}

func (a Content_Array) GetContentPartAtIndex(index int) isContentPart {
	if len(a) > index && a[index] != nil {
		return a[index]
	}
	return nil
}

func (a Content_Array) GetContentPartTextAtIndex(index int) *ContentPart_Text {
	if len(a) > index && a[index] != nil {
		if cp, ok := a[index].(ContentPart_Text); ok {
			return &cp
		}
	}
	return nil
}
