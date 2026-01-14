package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"excel-translator/pkg/adapter/doubao"
	"excel-translator/pkg/adapter/excel"
	"excel-translator/pkg/adapter/sqlite"
	"excel-translator/pkg/service"
)

func main() {
	// 1. Setup Logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// 2. Parse Args
	if len(os.Args) < 3 {
		fmt.Println("Usage: translator <input.xlsx> <output.xlsx>")
		os.Exit(1)
	}
	inputFile := os.Args[1]
	outputFile := os.Args[2]

	// 3. Load Config
	apiKey := os.Getenv("DOUBAO_API_KEY")
	endpointID := os.Getenv("DOUBAO_ENDPOINT_ID")
	apiURL := os.Getenv("DOUBAO_API_URL")
	rpmStr := os.Getenv("DOUBAO_RPM")
	rpm := 60
	if rpmStr != "" {
		if val, err := strconv.Atoi(rpmStr); err == nil {
			rpm = val
		}
	}

	if apiKey == "" {
		slog.Error("DOUBAO_API_KEY is required")
		os.Exit(1)
	}

	// 4. Initialize Adapters
	// SQLite
	store, err := sqlite.NewStore("tasks.db")
	if err != nil {
		slog.Error("Failed to init sqlite", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	// Doubao
	translator := doubao.NewClient(apiKey, endpointID, apiURL, rpm)

	// Excel
	excelAdapter := excel.NewAdapter()

	// 5. Initialize Service
	engine := service.NewTranslationEngine(translator, store)

	// 6. Setup Signal Handling
	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		slog.Info("Received shutdown signal. Stopping processing...")
		cancel()
	}()

	// 7. Workflow
	// A. Read Raw Rows
	slog.Info("Reading input file", "path", inputFile)
	jobID, tasks, err := excelAdapter.ReadRawRows(inputFile)
	if err != nil {
		slog.Error("Failed to read input file", "error", err)
		os.Exit(1)
	}
	slog.Info("File read successfully", "jobID", jobID, "potential_tasks", len(tasks))

	// B. Ingest
	if err := engine.Ingest(ctx, tasks); err != nil {
		slog.Error("Failed to ingest tasks", "error", err)
		os.Exit(1)
	}

	// C. Run Loop
	slog.Info("Starting translation engine")
	start := time.Now()
	if err := engine.RunLoop(ctx, jobID); err != nil {
		if err == context.Canceled {
			slog.Info("Processing interrupted")
		} else {
			slog.Error("Engine error", "error", err)
		}
	}
	duration := time.Since(start)
	slog.Info("Processing finished", "duration", duration)

	// D. Export Results (Always export what we have)
	slog.Info("Exporting results", "path", outputFile)
	completedTasks, err := engine.GetResults(context.Background(), jobID) // Use background context to ensure export happens even if canceled
	if err != nil {
		slog.Error("Failed to retrieve results", "error", err)
		os.Exit(1)
	}

	slog.Info("Found completed tasks", "count", len(completedTasks))
	if err := excelAdapter.WriteResults(inputFile, outputFile, completedTasks); err != nil {
		slog.Error("Failed to write output file", "error", err)
		os.Exit(1)
	}

	slog.Info("Done", "output", outputFile)
}
