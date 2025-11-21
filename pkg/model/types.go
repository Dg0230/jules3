package model

// Cell represents a single cell to be translated.
type Cell struct {
	CellID  int               `json:"cell_id"`
	Content string            `json:"content"`
	Terms   map[string]string `json:"terms,omitempty"`
}

// TranslationPayload represents the JSON structure sent in the user message content.
type TranslationPayload struct {
	Cells      []Cell `json:"cells"`
	SourceLang string `json:"source_lang"`
	TargetLang string `json:"target_lang"`
}

// ChatMessage represents a message in the OpenAI chat format.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatCompletionRequest represents the request body for the API.
type ChatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
}

// ChatCompletionResponse represents the response from the API.
type ChatCompletionResponse struct {
	Choices []struct {
		Message ChatMessage `json:"message"`
	} `json:"choices"`
}
