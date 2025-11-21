package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"excel-translator/pkg/model"

	"github.com/go-resty/resty/v2"
	"golang.org/x/time/rate"
)

type Client struct {
	apiKey     string
	endpointID string
	apiURL     string
	client     *resty.Client
	limiter    *rate.Limiter
}

func NewClient(apiKey, endpointID, apiURL string, rpm int) *Client {
	// Create a rate limiter. RPM / 60 seconds = limit per second.
	// Burst size of 1 means strictly 1 request every (60/rpm) seconds.
	// We can allow a small burst.
	limit := rate.Limit(float64(rpm) / 60.0)
	limiter := rate.NewLimiter(limit, 1)

	if apiURL == "" {
		apiURL = "https://ark.cn-beijing.volces.com/api/v3/chat/completions"
	}

	return &Client{
		apiKey:     apiKey,
		endpointID: endpointID,
		apiURL:     apiURL,
		client: resty.New().
			SetTimeout(120 * time.Second).
			SetRetryCount(3).
			SetRetryWaitTime(1 * time.Second).
			SetRetryMaxWaitTime(5 * time.Second),
		limiter:    limiter,
	}
}

func (c *Client) TranslateBatch(ctx context.Context, cells []model.Cell, sourceLang, targetLang string) ([]model.Cell, error) {
	// 1. Construct the payload
	payload := model.TranslationPayload{
		Cells:      cells,
		SourceLang: sourceLang,
		TargetLang: targetLang,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	// 2. Construct System Prompt
	systemPrompt := fmt.Sprintf(`You are a localization assistant. You must strictly adhere to the user's provided JSON structure and output ONLY the translated JSON. Target Language: %s. Please follow these rules:
1) Execute any cells[i].constraint or global constraint.
2) Preserve/correctly convert symbols and numbers. CJK targets must use CJK symbols; NON-CJK targets MUST NOT contain CJK symbols. Convert symbols according to the target language conventions (e.g., CJK brackets 【 】 must become Western [ ]).
3) Do not add any explanations or extra text.
4) Output sequentially by cell_id order.`, targetLang)

	reqBody := model.ChatCompletionRequest{
		Model: c.endpointID,
		Messages: []model.ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: string(payloadBytes)},
		},
	}

	// 3. Rate Limiting
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter error: %w", err)
	}

	// Mock response for testing
	if c.apiKey == "TEST_KEY" {
		mockCells := make([]model.Cell, len(cells))
		for i, cell := range cells {
			mockCells[i] = model.Cell{
				CellID:  cell.CellID,
				Content: fmt.Sprintf("[MOCK %s] %s", targetLang, cell.Content),
			}
		}
		return mockCells, nil
	}

	// 4. Execute Request with Retry
	var respBody model.ChatCompletionResponse
	var errResp map[string]interface{}

	// Doubao API endpoint: https://ark.cn-beijing.volces.com/api/v3/chat/completions
	// Use retry mechanism for 5xx errors or network issues

	resp, err := c.client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", "Bearer "+c.apiKey).
		SetBody(reqBody).
		SetResult(&respBody).
		SetError(&errResp).
		Post(c.apiURL)

	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("api error: status=%d body=%s", resp.StatusCode(), resp.String())
	}

	if len(respBody.Choices) == 0 {
		return nil, errors.New("no choices in response")
	}

	content := respBody.Choices[0].Message.Content
    // remove markdown code block if present
    content = strings.TrimSpace(content)
    if strings.HasPrefix(content, "```json") {
        content = strings.TrimPrefix(content, "```json")
        content = strings.TrimSuffix(content, "```")
    } else if strings.HasPrefix(content, "```") {
        content = strings.TrimPrefix(content, "```")
        content = strings.TrimSuffix(content, "```")
    }
    content = strings.TrimSpace(content)

	// 5. Parse Response
	var resultPayload model.TranslationPayload
	if err := json.Unmarshal([]byte(content), &resultPayload); err != nil {
		log.Printf("Failed JSON content: %s", content)
		return nil, fmt.Errorf("failed to unmarshal response json: %w", err)
	}

	// 6. Validation
	if len(resultPayload.Cells) != len(cells) {
		return nil, fmt.Errorf("mismatch in cell count: expected %d, got %d", len(cells), len(resultPayload.Cells))
	}

    // Map cell IDs to verify
    inputMap := make(map[int]bool)
    for _, cell := range cells {
        inputMap[cell.CellID] = true
    }

    for _, cell := range resultPayload.Cells {
        if !inputMap[cell.CellID] {
             return nil, fmt.Errorf("response contains unknown cell_id: %d", cell.CellID)
        }
    }

	return resultPayload.Cells, nil
}
