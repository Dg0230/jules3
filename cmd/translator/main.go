package main

import (
	"log"
	"os"
	"strconv"

	"excel-translator/pkg/llm"
	"excel-translator/pkg/translator"

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

	client := llm.NewClient(apiKey, endpointID, apiURL, rpm)
	proc := translator.NewProcessor(client)

	inputFile := "sample.xlsx"
	if len(os.Args) > 1 {
		inputFile = os.Args[1]
	}

	outputFile := "output.xlsx"
	if len(os.Args) > 2 {
		outputFile = os.Args[2]
	}

	log.Printf("Processing %s -> %s", inputFile, outputFile)
	if err := proc.ProcessFile(inputFile, outputFile); err != nil {
		log.Fatalf("Processing failed: %v", err)
	}

	log.Println("Translation complete!")
}
