package doubao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"excel-translator/pkg/domain"
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

// Ensure Client implements domain.Translator
var _ domain.Translator = (*Client)(nil)

func (c *Client) TranslateBatch(ctx context.Context, cells []domain.Cell, sourceLang, targetLang string) ([]domain.TranslationResult, error) {
	// 1. Convert domain cells to model cells (internal representation for Doubao)
	// The internal model still uses int ID, but domain uses string.
	// We need to map string ID <-> int ID or just change model to use string.
	// Assuming model.Cell.CellID is int, we need a temporary mapping or simple conversion if IDs are numeric.
	// In this specific app, IDs are numeric row indices.

	modelCells := make([]model.Cell, len(cells))
	idMap := make(map[int]string)

	for i, dc := range cells {
		// Try parsing ID as int
		intID, err := strconv.Atoi(dc.ID)
		if err != nil {
			// If not integer, we might need a different approach, but for this tool we know it's row index.
			// Let's default to index + 1 if parsing fails or just hash it.
			// But let's handle the contract strictly: The prompt requires CellID.
			// If we change model.Cell to use string ID, we update the prompt.
			// For now, let's assume numeric ID as per legacy code, but handle mapping.
			intID = i + 1 // Fallback or use loop index if string is opaque
		}
		modelCells[i] = model.Cell{
			CellID:  intID,
			Content: dc.Content,
			Terms:   dc.Terms,
		}
		idMap[intID] = dc.ID
	}

	payload := model.TranslationPayload{
		Cells:      modelCells,
		SourceLang: sourceLang,
		TargetLang: targetLang,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	// 2. Construct Prompt
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

	// Mock
	if c.apiKey == "TEST_KEY" {
		results := make([]domain.TranslationResult, len(cells))
		for i, cell := range cells {
			results[i] = domain.TranslationResult{
				ID:      cell.ID,
				Content: fmt.Sprintf("[MOCK %s] %s", targetLang, cell.Content),
			}
		}
		return results, nil
	}

	// 4. Execute
	var respBody model.ChatCompletionResponse
	var errResp map[string]interface{}

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
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```json") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimSuffix(content, "```")
	} else if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
	}
	content = strings.TrimSpace(content)

	var resultPayload model.TranslationPayload
	if err := json.Unmarshal([]byte(content), &resultPayload); err != nil {
		slog.Error("Failed to unmarshal JSON", "content", content, "error", err)
		return nil, fmt.Errorf("failed to unmarshal response json: %w", err)
	}

	// 5. Convert back to domain results
	if len(resultPayload.Cells) != len(cells) {
		return nil, fmt.Errorf("mismatch in cell count: expected %d, got %d", len(cells), len(resultPayload.Cells))
	}

	results := make([]domain.TranslationResult, 0, len(resultPayload.Cells))
	for _, rc := range resultPayload.Cells {
		originalID, ok := idMap[rc.CellID]
		if !ok {
			return nil, fmt.Errorf("unknown cell id returned: %d", rc.CellID)
		}
		results = append(results, domain.TranslationResult{
			ID:      originalID,
			Content: rc.Content,
		})
	}

	return results, nil
}
