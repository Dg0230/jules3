package translator

import (
	"context"
	"fmt"
	"log"

	"excel-translator/pkg/llm"
	"excel-translator/pkg/model"

	"github.com/xuri/excelize/v2"
)

type Processor struct {
	client *llm.Client
}

func NewProcessor(client *llm.Client) *Processor {
	return &Processor{client: client}
}

func (p *Processor) ProcessFile(inputPath, outputPath string) error {
	f, err := excelize.OpenFile(inputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// Assume data is in "Sheet1"
	sheetName := "Sheet1"
	rows, err := f.GetRows(sheetName)
	if err != nil {
		return err
	}

	// Find headers
	headerMap := make(map[string]int)
	if len(rows) == 0 {
		return fmt.Errorf("empty file")
	}
	for i, cell := range rows[0] {
		headerMap[cell] = i
	}

	requiredHeaders := []string{"key", "CH", "FR", "PT"}
	for _, h := range requiredHeaders {
		if _, ok := headerMap[h]; !ok {
			return fmt.Errorf("missing header: %s", h)
		}
	}

	// Identify target languages
	targets := []string{"FR", "PT"}

	// Create a queue of tasks
	type Task struct {
		RowIndex int
		RowData  []string
	}

	// Batch size
	batchSize := 10 // Adjust based on token limits

	for _, targetLang := range targets {
		log.Printf("Processing target language: %s", targetLang)
		colIndex := headerMap[targetLang]
		chIndex := headerMap["CH"]
		// keyIndex := headerMap["key"] // Not strictly needed for logic but good for debugging

		var batch []model.Cell
		var batchRowIndices []int

		processBatch := func() {
			if len(batch) == 0 {
				return
			}

			log.Printf("Translating batch of %d cells to %s...", len(batch), targetLang)
			translatedCells, err := p.client.TranslateBatch(context.Background(), batch, "CH", targetLang)
			if err != nil {
				log.Printf("Error translating batch (rows %v): %v. Skipping batch.", batchRowIndices, err)
				// TODO: Implement finer-grained retry or fallback here if needed.
				// For now, we log and skip to avoid stopping the whole process.
			} else {
				// Update Excel
				for _, tCell := range translatedCells {
					// Find the row index for this cell_id (which we mapped to row index)
					// cell.CellID stores the row index
					rowIndex := tCell.CellID
					cellName, _ := excelize.CoordinatesToCellName(colIndex+1, rowIndex+1)
					f.SetCellValue(sheetName, cellName, tCell.Content)
				}
			}

			// Clear batch
			batch = nil
			batchRowIndices = nil
		}

		for i := 1; i < len(rows); i++ {
			row := rows[i]
			// Ensure row has enough columns
			if len(row) <= chIndex {
				continue
			}

			chText := row[chIndex]

			// Check if target is empty
			targetText := ""
			if len(row) > colIndex {
				targetText = row[colIndex]
			}

			if chText != "" && targetText == "" {
				// Add to batch
				batch = append(batch, model.Cell{
					CellID:  i, // Use row index as ID
					Content: chText,
				})
				batchRowIndices = append(batchRowIndices, i)

				if len(batch) >= batchSize {
					processBatch()
				}
			}
		}
		// Process remaining
		processBatch()
	}

	if err := f.SaveAs(outputPath); err != nil {
		return err
	}

	return nil
}
