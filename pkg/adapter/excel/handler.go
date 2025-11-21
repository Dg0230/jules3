package excel

import (
	"fmt"
	"os"
	"crypto/md5"
	"io"

	"github.com/xuri/excelize/v2"
	"excel-translator/pkg/domain"
)

// Adapter handles Excel file operations.
type Adapter struct {
}

func NewAdapter() *Adapter {
	return &Adapter{}
}

// ReadRawRows reads the "CH" column and determines targets ("FR", "PT") for rows that need translation.
// Returns the list of raw task intents and the Job ID.
func (a *Adapter) ReadRawRows(path string) (jobID string, tasks []domain.Task, err error) {
	jobID, err = calculateFileHash(path)
	if err != nil {
		return "", nil, err
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return "", nil, err
	}
	defer f.Close()

	sheetName := "Sheet1"
	rows, err := f.GetRows(sheetName)
	if err != nil {
		return "", nil, err
	}

	if len(rows) == 0 {
		return "", nil, fmt.Errorf("empty file")
	}

	headerMap := make(map[string]int)
	for i, cell := range rows[0] {
		headerMap[cell] = i
	}

	requiredHeaders := []string{"key", "CH", "FR", "PT"}
	for _, h := range requiredHeaders {
		if _, ok := headerMap[h]; !ok {
			return "", nil, fmt.Errorf("missing header: %s", h)
		}
	}

	chIndex := headerMap["CH"]
	targets := []string{"FR", "PT"}

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

			if targetText == "" {
				tasks = append(tasks, domain.Task{
					JobID:         jobID,
					RowIndex:      i,
					SourceContent: chText,
					TargetLang:    targetLang,
				})
			}
		}
	}

	return jobID, tasks, nil
}

// WriteResults updates the Excel file with completed translations.
func (a *Adapter) WriteResults(path string, outputPath string, tasks []domain.Task) error {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return err
	}
	defer f.Close()

	sheetName := "Sheet1"

	// Re-read headers to find indices
	rows, err := f.GetRows(sheetName)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("empty file")
	}
	headerMap := make(map[string]int)
	for i, cell := range rows[0] {
		headerMap[cell] = i
	}

	for _, t := range tasks {
		colIndex, ok := headerMap[t.TargetLang]
		if !ok {
			continue // Should not happen if file is same
		}
		// RowIndex is 0-based from rows slice. Excelize uses 1-based coordinates.
		// rows[0] is A1. rows[1] is A2.
		// t.RowIndex corresponds to the index in `rows`.
		// So t.RowIndex=1 -> A2.
		// CoordinatesToCellName(col, row) expects 1-based.
		// A2 -> col=1, row=2.
		// So row argument should be t.RowIndex + 1.

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
