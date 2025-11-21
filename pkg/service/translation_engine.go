package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"excel-translator/pkg/domain"
)

// TranslationEngine orchestrates the translation process.
// It is pure business logic, decoupled from specific file formats or APIs.
type TranslationEngine struct {
	translator domain.Translator
	repo       domain.TaskRepository
	batchSize  int
}

func NewTranslationEngine(t domain.Translator, r domain.TaskRepository) *TranslationEngine {
	return &TranslationEngine{
		translator: t,
		repo:       r,
		batchSize:  10, // Default batch size
	}
}

// Ingest accepts a list of intended tasks and persists them if not already present.
func (e *TranslationEngine) Ingest(ctx context.Context, tasks []domain.Task) error {
	if len(tasks) == 0 {
		return nil
	}
	slog.Info("Ingesting tasks", "count", len(tasks), "jobID", tasks[0].JobID)
	return e.repo.AddTasks(ctx, tasks)
}

// RunLoop processes pending tasks until none remain.
func (e *TranslationEngine) RunLoop(ctx context.Context, jobID string) error {
	for {
		// Check for cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		tasks, err := e.repo.GetPendingTasks(ctx, jobID, e.batchSize)
		if err != nil {
			return fmt.Errorf("failed to fetch pending tasks: %w", err)
		}

		if len(tasks) == 0 {
			slog.Info("No more pending tasks", "jobID", jobID)
			break
		}

		slog.Info("Processing batch", "count", len(tasks), "jobID", jobID)
		if err := e.processBatch(ctx, tasks); err != nil {
			slog.Error("Batch processing failed", "error", err)
			// Continue loop to retry or pick up others?
			// Depending on error type. For now, log and continue (retry count handles infinite loops)
		}

		// Optional short sleep to yield
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}

func (e *TranslationEngine) processBatch(ctx context.Context, tasks []domain.Task) error {
	// Mark processing
	taskIDs := make([]int64, len(tasks))
	for i, t := range tasks {
		taskIDs[i] = t.ID
	}
	if err := e.repo.MarkProcessing(ctx, taskIDs); err != nil {
		return err
	}

	// Group by language
	tasksByLang := make(map[string][]domain.Task)
	for _, t := range tasks {
		tasksByLang[t.TargetLang] = append(tasksByLang[t.TargetLang], t)
	}

	for lang, langTasks := range tasksByLang {
		cells := make([]domain.Cell, len(langTasks))
		for i, t := range langTasks {
			cells[i] = domain.Cell{
				ID:      fmt.Sprintf("%d", t.ID), // Use ID as string
				Content: t.SourceContent,
			}
		}

		slog.Debug("Sending batch to translator", "lang", lang, "count", len(cells))
		results, err := e.translator.TranslateBatch(ctx, cells, "CH", lang)

		if err != nil {
			slog.Error("Translation API failed", "lang", lang, "error", err)
			// Mark failed
			ids := make([]int64, len(langTasks))
			for i, t := range langTasks {
				ids[i] = t.ID
			}
			e.repo.MarkFailed(ctx, ids, err.Error())
			continue
		}

		// Map results back
		// Result ID matches Task ID (stringified)
		completedTasks := make([]domain.Task, 0, len(results))
		for _, res := range results {
			// Parse ID back to int64
			// In domain.Cell we converted ID to string.
			// We can find the original task.
			var originalTask domain.Task
			found := false
			for _, t := range langTasks {
				if fmt.Sprintf("%d", t.ID) == res.ID {
					originalTask = t
					found = true
					break
				}
			}
			if found {
				originalTask.TranslatedText = res.Content
				completedTasks = append(completedTasks, originalTask)
			}
		}

		if err := e.repo.MarkComplete(ctx, completedTasks); err != nil {
			slog.Error("Failed to persist completions", "error", err)
		}
	}
	return nil
}

// GetResults retrieves all completed translations for the job.
func (e *TranslationEngine) GetResults(ctx context.Context, jobID string) ([]domain.Task, error) {
	return e.repo.GetCompletedTasks(ctx, jobID)
}
