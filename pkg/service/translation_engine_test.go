package service

import (
	"context"
	"errors"
	"testing"

	"excel-translator/pkg/domain"
)

// MockTranslator
type MockTranslator struct {
	TranslateFunc func(ctx context.Context, cells []domain.Cell, sourceLang, targetLang string) ([]domain.TranslationResult, error)
}

func (m *MockTranslator) TranslateBatch(ctx context.Context, cells []domain.Cell, sourceLang, targetLang string) ([]domain.TranslationResult, error) {
	return m.TranslateFunc(ctx, cells, sourceLang, targetLang)
}

// MockRepo
type MockRepo struct {
	AddTasksFunc        func(ctx context.Context, tasks []domain.Task) error
	GetPendingTasksFunc func(ctx context.Context, jobID string, limit int) ([]domain.Task, error)
	MarkProcessingFunc  func(ctx context.Context, taskIDs []int64) error
	MarkCompleteFunc    func(ctx context.Context, tasks []domain.Task) error
	MarkFailedFunc      func(ctx context.Context, taskIDs []int64, reason string) error
	GetCompletedTasksFunc func(ctx context.Context, jobID string) ([]domain.Task, error)
}

func (m *MockRepo) Init() error { return nil }
func (m *MockRepo) Close() error { return nil }
func (m *MockRepo) AddTasks(ctx context.Context, tasks []domain.Task) error {
	if m.AddTasksFunc != nil {
		return m.AddTasksFunc(ctx, tasks)
	}
	return nil
}
func (m *MockRepo) GetPendingTasks(ctx context.Context, jobID string, limit int) ([]domain.Task, error) {
	if m.GetPendingTasksFunc != nil {
		return m.GetPendingTasksFunc(ctx, jobID, limit)
	}
	return nil, nil
}
func (m *MockRepo) MarkProcessing(ctx context.Context, taskIDs []int64) error {
	if m.MarkProcessingFunc != nil {
		return m.MarkProcessingFunc(ctx, taskIDs)
	}
	return nil
}
func (m *MockRepo) MarkComplete(ctx context.Context, tasks []domain.Task) error {
	if m.MarkCompleteFunc != nil {
		return m.MarkCompleteFunc(ctx, tasks)
	}
	return nil
}
func (m *MockRepo) MarkFailed(ctx context.Context, taskIDs []int64, reason string) error {
	if m.MarkFailedFunc != nil {
		return m.MarkFailedFunc(ctx, taskIDs, reason)
	}
	return nil
}
func (m *MockRepo) GetCompletedTasks(ctx context.Context, jobID string) ([]domain.Task, error) {
	if m.GetCompletedTasksFunc != nil {
		return m.GetCompletedTasksFunc(ctx, jobID)
	}
	return nil, nil
}

func TestEngine_RunLoop(t *testing.T) {
	// Setup
	pendingTasks := []domain.Task{
		{ID: 1, JobID: "job1", SourceContent: "Hello", TargetLang: "FR"},
	}

	repo := &MockRepo{
		GetPendingTasksFunc: func(ctx context.Context, jobID string, limit int) ([]domain.Task, error) {
			if len(pendingTasks) > 0 {
				batch := pendingTasks
				pendingTasks = nil // drain
				return batch, nil
			}
			return nil, nil
		},
		MarkProcessingFunc: func(ctx context.Context, taskIDs []int64) error {
			return nil
		},
		MarkCompleteFunc: func(ctx context.Context, tasks []domain.Task) error {
			if len(tasks) != 1 {
				t.Errorf("Expected 1 completed task, got %d", len(tasks))
			}
			if tasks[0].TranslatedText != "Bonjour" {
				t.Errorf("Expected 'Bonjour', got '%s'", tasks[0].TranslatedText)
			}
			return nil
		},
	}

	translator := &MockTranslator{
		TranslateFunc: func(ctx context.Context, cells []domain.Cell, sourceLang, targetLang string) ([]domain.TranslationResult, error) {
			return []domain.TranslationResult{
				{ID: cells[0].ID, Content: "Bonjour"},
			}, nil
		},
	}

	engine := NewTranslationEngine(translator, repo)

	err := engine.RunLoop(context.Background(), "job1")
	if err != nil {
		t.Errorf("RunLoop failed: %v", err)
	}
}

func TestEngine_TranslationFailure(t *testing.T) {
	// Test that failure calls MarkFailed
	pendingTasks := []domain.Task{
		{ID: 1, JobID: "job1", SourceContent: "FailMe", TargetLang: "FR"},
	}

	failedCalled := false

	repo := &MockRepo{
		GetPendingTasksFunc: func(ctx context.Context, jobID string, limit int) ([]domain.Task, error) {
			if len(pendingTasks) > 0 {
				batch := pendingTasks
				pendingTasks = nil
				return batch, nil
			}
			return nil, nil
		},
		MarkProcessingFunc: func(ctx context.Context, taskIDs []int64) error { return nil },
		MarkFailedFunc: func(ctx context.Context, taskIDs []int64, reason string) error {
			failedCalled = true
			if reason != "API Error" {
				t.Errorf("Expected 'API Error', got '%s'", reason)
			}
			return nil
		},
	}

	translator := &MockTranslator{
		TranslateFunc: func(ctx context.Context, cells []domain.Cell, sourceLang, targetLang string) ([]domain.TranslationResult, error) {
			return nil, errors.New("API Error")
		},
	}

	engine := NewTranslationEngine(translator, repo)
	engine.RunLoop(context.Background(), "job1")

	if !failedCalled {
		t.Error("MarkFailed was not called")
	}
}
