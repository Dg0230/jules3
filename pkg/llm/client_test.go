package llm

import (
	"context"
	"testing"

	"excel-translator/pkg/model"
)

func TestMockMode(t *testing.T) {
	client := NewClient("TEST_KEY", "ep-test", "", 600)
	cells := []model.Cell{
		{CellID: 1, Content: "Hello"},
	}

	translated, err := client.TranslateBatch(context.Background(), cells, "CH", "FR")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(translated) != 1 {
		t.Fatalf("Expected 1 translated cell, got %d", len(translated))
	}

	if translated[0].Content != "[MOCK FR] Hello" {
		t.Errorf("Unexpected content: %s", translated[0].Content)
	}
}
