package main

import (
	"log"
	"os"
	"context"
	"strconv"

	"excel-translator/pkg/adapter/doubao"
	"excel-translator/pkg/adapter/excel"
	"excel-translator/pkg/adapter/sqlite"
	"excel-translator/pkg/service"

	"github.com/joho/godotenv"
)

func main() {
	// Load .env file
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .env file not found, relying on environment variables")
	}

	apiKey := os.Getenv("DOUBAO_API_KEY")
	endpointID := os.Getenv("DOUBAO_ENDPOINT_ID")
	if apiKey == "" || endpointID == "" {
		log.Fatal("DOUBAO_API_KEY and DOUBAO_ENDPOINT_ID must be set")
	}

	rpmStr := os.Getenv("DOUBAO_RPM")
	rpm := 60 // Default RPM
	if rpmStr != "" {
		if v, err := strconv.Atoi(rpmStr); err == nil {
			rpm = v
		}
	}

	apiURL := os.Getenv("DOUBAO_API_URL")

	log.Printf("Starting translator with RPM limit: %d", rpm)

	// 1. Initialize Adapters
	dbPath := "translation.db"
	repo, err := sqlite.NewStore(dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer repo.Close()

	translator := doubao.NewClient(apiKey, endpointID, apiURL, rpm)
	excelAdapter := excel.NewAdapter()

	// 2. Initialize Service
	engine := service.NewTranslationEngine(translator, repo)

	// 3. CLI Logic
	inputFile := "sample.xlsx"
	if len(os.Args) > 1 {
		inputFile = os.Args[1]
	}

	outputFile := "output.xlsx"
	if len(os.Args) > 2 {
		outputFile = os.Args[2]
	}

	ctx := context.Background()
	log.Printf("Processing %s -> %s", inputFile, outputFile)

	// Step A: Read & Ingest
	jobID, tasks, err := excelAdapter.ReadRawRows(inputFile)
	if err != nil {
		log.Fatalf("Failed to read input: %v", err)
	}
	log.Printf("Job ID: %s. Found %d potential translation tasks.", jobID, len(tasks))

	if err := engine.Ingest(ctx, tasks); err != nil {
		log.Fatalf("Failed to ingest tasks: %v", err)
	}

	// Step B: Process
	if err := engine.RunLoop(ctx, jobID); err != nil {
		log.Fatalf("Processing loop failed: %v", err)
	}

	// Step C: Export
	completedTasks, err := engine.GetResults(ctx, jobID)
	if err != nil {
		log.Fatalf("Failed to get results: %v", err)
	}
	log.Printf("Retrieved %d completed tasks.", len(completedTasks))

	if err := excelAdapter.WriteResults(inputFile, outputFile, completedTasks); err != nil {
		log.Fatalf("Failed to write output: %v", err)
	}

	log.Println("Translation complete!")
}
