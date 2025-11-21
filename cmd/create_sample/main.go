package main

import (
	"fmt"
	"log"

	"github.com/xuri/excelize/v2"
)

func main() {
	f := excelize.NewFile()
	defer func() {
		if err := f.Close(); err != nil {
			log.Println(err)
		}
	}()

	// Create a new sheet.
	index, err := f.NewSheet("Sheet1")
	if err != nil {
		log.Fatal(err)
	}

	// Set headers
	headers := []string{"key", "CH", "FR", "PT"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue("Sheet1", cell, h)
	}

	// Sample data
	data := []struct {
		key string
		ch  string
	}{
		{"110300", "【免疫】"},
		{"110301", "【普通攻击】造成{0}点伤害"},
		{"110302", "对区域内所有敌人造成火焰伤害，并附加【灼烧】。"},
		{"110303", "提示"},
		{"110304", "取消"},
		{"110305", "確認"},
		{"110306", "前往儲值"},
	}

	for i, row := range data {
		f.SetCellValue("Sheet1", fmt.Sprintf("A%d", i+2), row.key)
		f.SetCellValue("Sheet1", fmt.Sprintf("B%d", i+2), row.ch)
	}

	f.SetActiveSheet(index)

	if err := f.SaveAs("sample.xlsx"); err != nil {
		log.Fatal(err)
	}

	fmt.Println("sample.xlsx created successfully")
}
