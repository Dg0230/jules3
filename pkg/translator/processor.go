package translator

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"log"
	"os"

	"excel-translator/pkg/llm"
	"excel-translator/pkg/model"
	"excel-translator/pkg/store"

	"github.com/xuri/excelize/v2"
)

type Processor struct {
	client *llm.Client
	store  *store.Store
}

func NewProcessor(client *llm.Client, store *store.Store) *Processor {
	return &Processor{client: client, store: store}
}

func (p *Processor) ProcessFile(inputPath, outputPath string) error {
	// Calculate job ID based on file hash
	jobID, err := calculateFileHash(inputPath)
	if err != nil {
		return fmt.Errorf("failed to calculate file hash: %w", err)
	}
	log.Printf("Job ID for %s: %s", inputPath, jobID)

	// 1. Open Excel File
	f, err := excelize.OpenFile(inputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	sheetName := "Sheet1"
	rows, err := f.GetRows(sheetName)
	if err != nil {
		return err
	}

	if len(rows) == 0 {
		return fmt.Errorf("empty file")
	}

	// Map headers
	headerMap := make(map[string]int)
	for i, cell := range rows[0] {
		headerMap[cell] = i
	}
	requiredHeaders := []string{"key", "CH", "FR", "PT"}
	for _, h := range requiredHeaders {
		if _, ok := headerMap[h]; !ok {
			return fmt.Errorf("missing header: %s", h)
		}
	}

	// 2. Ingest Tasks into SQLite
	log.Println("Ingesting tasks into SQLite...")
	var newTasks []store.Task
	targets := []string{"FR", "PT"}
	chIndex := headerMap["CH"]

	for i := 1; i < len(rows); i++ {
		row := rows[i]
		if len(row) <= chIndex {
			continue
		}
		chText := row[chIndex]
		if chText == "" {
			continue
		}

		for _, targetLang := range targets {
			colIndex := headerMap[targetLang]
			targetText := ""
			if len(row) > colIndex {
				targetText = row[colIndex]
			}

			// If target is empty, we need to translate it
			if targetText == "" {
				newTasks = append(newTasks, store.Task{
					JobID:         jobID,
					RowIndex:      i,
					SourceContent: chText,
					TargetLang:    targetLang,
				})
			}
		}
	}

	if len(newTasks) > 0 {
		if err := p.store.AddTasks(newTasks); err != nil {
			return fmt.Errorf("failed to add tasks: %w", err)
		}
		log.Printf("Added/Ensured %d tasks in queue.", len(newTasks))
	} else {
		log.Println("No new tasks to add.")
	}

	// 3. Execute Pending Tasks
	batchSize := 10
	for {
		// Get Pending
		tasks, err := p.store.GetPendingTasks(jobID, batchSize)
		if err != nil {
			return fmt.Errorf("failed to get pending tasks: %w", err)
		}

		if len(tasks) == 0 {
			log.Println("No more pending tasks.")
			break
		}

		log.Printf("Processing batch of %d tasks...", len(tasks))

		// Group by target lang to optimize LLM context (usually better to send same lang in one go)
		// Our current batch might be mixed. Let's sort or just process as sub-batches.
		// For simplicity, let's just group by lang locally.
		tasksByLang := make(map[string][]store.Task)
		for _, t := range tasks {
			tasksByLang[t.TargetLang] = append(tasksByLang[t.TargetLang], t)
		}

		// Mark as processing
		taskIDs := make([]int64, len(tasks))
		for i, t := range tasks {
			taskIDs[i] = t.ID
		}
		if err := p.store.MarkProcessing(taskIDs); err != nil {
			return fmt.Errorf("failed to mark processing: %w", err)
		}

		for lang, langTasks := range tasksByLang {
			var llmCells []model.Cell
			for _, t := range langTasks {
				llmCells = append(llmCells, model.Cell{
					CellID:  int(t.ID), // Use DB ID as Cell ID for correlation
					Content: t.SourceContent,
				})
			}

			log.Printf("Sending %d items for %s...", len(llmCells), lang)
			translatedCells, err := p.client.TranslateBatch(context.Background(), llmCells, "CH", lang)

			if err != nil {
				log.Printf("Batch failed for %s: %v", lang, err)
				// Mark these specific IDs as failed
				var failIDs []int64
				for _, t := range langTasks {
					failIDs = append(failIDs, t.ID)
				}
				p.store.MarkFailed(failIDs, err.Error())
			} else {
				// Success
				var completedTasks []store.Task
				for _, tc := range translatedCells {
					completedTasks = append(completedTasks, store.Task{
						ID:             int64(tc.CellID),
						TranslatedText: tc.Content,
					})
				}
				if err := p.store.MarkComplete(completedTasks); err != nil {
					log.Printf("Failed to mark complete: %v", err)
				}
			}
		}

		// Optional: Sleep slightly to be nice? The rate limiter in client handles the hard limit.
		// But client limiter is blocking.
	}

	// 4. Export Results to Excel
	log.Println("Exporting results to Excel...")
	completed, err := p.store.GetCompletedTasks(jobID)
	if err != nil {
		return err
	}

	for _, t := range completed {
		colIndex := headerMap[t.TargetLang]
		// RowIndex is 0-based from the loop (i=1...), matching Excelize row handling?
		// Excelize GetRows returns slice. Index 0 is row 1.
		// We stored `i` from `for i := 1; i < len(rows)`.
		// So `i` corresponds to `rows[i]`.
		// If `i=1`, it's the second row (header is 0).
		// In Excel coordinates, Row 1 is A1. Row 2 is A2.
		// `rows[0]` -> A1. `rows[1]` -> A2.
		// `i=1` is Row 2.
		// `CoordinatesToCellName(col, row)` takes 1-based indices.
		// So Row 2 is row=2. `i+1`?
		// Let's check `i` usage.
		// `for i := 1; i < len(rows)`: if i=1, it is the second row.
		// `CoordinatesToCellName` for second row is row=2.
		// So we need `i + 1`.

		cellName, _ := excelize.CoordinatesToCellName(colIndex+1, t.RowIndex+1)
		f.SetCellValue(sheetName, cellName, t.TranslatedText)
	}

	if err := f.SaveAs(outputPath); err != nil {
		return err
	}

	return nil
}

func calculateFileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
