package excel
import (
	"testing"
	"github.com/xuri/excelize/v2"
	"os"
)

func TestDynamicHeaders(t *testing.T) {
	// Create a temporary excel file with dynamic headers
	f := excelize.NewFile()
	defer func() {
		if err := f.Close(); err != nil {
			t.Log(err)
		}
	}()

	// Headers: key, CH, ES, DE (Dynamic targets)
	f.SetCellValue("Sheet1", "A1", "key")
	f.SetCellValue("Sheet1", "B1", "CH")
	f.SetCellValue("Sheet1", "C1", "ES")
	f.SetCellValue("Sheet1", "D1", "DE")

	// Data
	f.SetCellValue("Sheet1", "A2", "101")
	f.SetCellValue("Sheet1", "B2", "你好")
	// C2 (ES) and D2 (DE) are empty, so they should be picked up as tasks

	tmpFile := "test_dynamic.xlsx"
	if err := f.SaveAs(tmpFile); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile)

	adapter := NewAdapter()
	_, tasks, err := adapter.ReadRawRows(tmpFile)
	if err != nil {
		t.Fatalf("ReadRawRows failed: %v", err)
	}

	// We expect 2 tasks: one for ES, one for DE
	if len(tasks) != 2 {
		t.Fatalf("Expected 2 tasks, got %d", len(tasks))
	}

	langs := make(map[string]bool)
	for _, task := range tasks {
		langs[task.TargetLang] = true
	}

	if !langs["ES"] {
		t.Error("Missing ES task")
	}
	if !langs["DE"] {
		t.Error("Missing DE task")
	}
}
