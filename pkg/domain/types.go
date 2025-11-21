package domain

import (
	"context"
	"time"
)

// --- Entities ---

// Cell represents a unit of content to be translated.
type Cell struct {
	ID      string            `json:"id"` // Logical ID (e.g., "row_1")
	Content string            `json:"content"`
	Terms   map[string]string `json:"terms,omitempty"`
}

// TranslationResult represents the outcome of a translation.
type TranslationResult struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

// TaskStatus represents the state of a translation task.
type TaskStatus string

const (
	StatusPending    TaskStatus = "PENDING"
	StatusProcessing TaskStatus = "PROCESSING"
	StatusComplete   TaskStatus = "COMPLETE"
	StatusFailed     TaskStatus = "FAILED"
)

// Task represents a persisted translation unit.
type Task struct {
	ID            int64
	JobID         string
	RowIndex      int
	SourceContent string
	TargetLang    string
	TranslatedText string
	Status        TaskStatus
	ErrorMessage  string
	Attempts      int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// --- Interfaces ---

// Translator defines the capability to translate text.
type Translator interface {
	// TranslateBatch translates a batch of cells from source to target language.
	// It should handle rate limiting and retries internally or bubble up errors.
	TranslateBatch(ctx context.Context, cells []Cell, sourceLang, targetLang string) ([]TranslationResult, error)
}

// TaskRepository defines the capability to persist and retrieve tasks.
type TaskRepository interface {
	// Init ensures the storage schema is ready.
	Init() error
	// AddTasks adds new tasks to the store. Ignores duplicates.
	AddTasks(ctx context.Context, tasks []Task) error
	// GetPendingTasks fetches tasks that need processing for a given job.
	GetPendingTasks(ctx context.Context, jobID string, limit int) ([]Task, error)
	// MarkProcessing marks tasks as being processed to prevent double-work.
	MarkProcessing(ctx context.Context, taskIDs []int64) error
	// MarkComplete marks tasks as successfully completed.
	MarkComplete(ctx context.Context, results []Task) error
	// MarkFailed marks tasks as failed with an error message.
	MarkFailed(ctx context.Context, taskIDs []int64, reason string) error
	// GetCompletedTasks retrieves all completed tasks for a job.
	GetCompletedTasks(ctx context.Context, jobID string) ([]Task, error)
	// Close closes the connection.
	Close() error
}
