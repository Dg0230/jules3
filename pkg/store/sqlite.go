package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type TaskStatus string

const (
	StatusPending  TaskStatus = "PENDING"
	StatusProcessing TaskStatus = "PROCESSING"
	StatusComplete TaskStatus = "COMPLETE"
	StatusFailed   TaskStatus = "FAILED"
)

type Task struct {
	ID             int64
	JobID          string
	RowIndex       int
	SourceContent  string
	TargetLang     string
	TranslatedText string
	Status         TaskStatus
	ErrorMessage   string
	Attempts       int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Store struct {
	db *sql.DB
}

func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	s := &Store{db: db}
	if err := s.init(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) init() error {
	query := `
	CREATE TABLE IF NOT EXISTS tasks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		job_id TEXT NOT NULL,
		row_index INTEGER NOT NULL,
		source_content TEXT NOT NULL,
		target_lang TEXT NOT NULL,
		translated_text TEXT,
		status TEXT NOT NULL,
		error_message TEXT,
		attempts INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(job_id, row_index, target_lang)
	);
	`
	_, err := s.db.Exec(query)
	return err
}

func (s *Store) AddTasks(tasks []Task) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO tasks (job_id, row_index, source_content, target_lang, status)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(job_id, row_index, target_lang) DO NOTHING
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, task := range tasks {
		_, err := stmt.Exec(task.JobID, task.RowIndex, task.SourceContent, task.TargetLang, StatusPending)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) GetPendingTasks(jobID string, limit int) ([]Task, error) {
	query := `
		SELECT id, row_index, source_content, target_lang, attempts
		FROM tasks
		WHERE job_id = ? AND status IN ('PENDING', 'FAILED') AND attempts < 5
		ORDER BY id ASC
		LIMIT ?
	`
	rows, err := s.db.Query(query, jobID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.RowIndex, &t.SourceContent, &t.TargetLang, &t.Attempts); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

func (s *Store) MarkProcessing(taskIDs []int64) error {
	if len(taskIDs) == 0 {
		return nil
	}

	// Build query manually for IN clause
	args := make([]interface{}, len(taskIDs))
	for i, id := range taskIDs {
		args[i] = id
	}

	placeholders := strings.Repeat("?,", len(taskIDs))
	placeholders = placeholders[:len(placeholders)-1]

	query := fmt.Sprintf("UPDATE tasks SET status = ?, attempts = attempts + 1, updated_at = CURRENT_TIMESTAMP WHERE id IN (%s)", placeholders)

	// Add status as first arg
	finalArgs := append([]interface{}{StatusProcessing}, args...)

	_, err := s.db.Exec(query, finalArgs...)
	return err
}

func (s *Store) MarkComplete(tasks []Task) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		UPDATE tasks
		SET status = ?, translated_text = ?, error_message = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, t := range tasks {
		_, err := stmt.Exec(StatusComplete, t.TranslatedText, t.ID)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) MarkFailed(taskIDs []int64, errMsg string) error {
	if len(taskIDs) == 0 {
		return nil
	}

	args := make([]interface{}, len(taskIDs))
	for i, id := range taskIDs {
		args[i] = id
	}

	placeholders := strings.Repeat("?,", len(taskIDs))
	placeholders = placeholders[:len(placeholders)-1]

	query := fmt.Sprintf("UPDATE tasks SET status = ?, error_message = ?, updated_at = CURRENT_TIMESTAMP WHERE id IN (%s)", placeholders)

	finalArgs := append([]interface{}{StatusFailed, errMsg}, args...)

	_, err := s.db.Exec(query, finalArgs...)
	return err
}

func (s *Store) GetCompletedTasks(jobID string) ([]Task, error) {
	query := `
		SELECT row_index, target_lang, translated_text
		FROM tasks
		WHERE job_id = ? AND status = 'COMPLETE'
	`
	rows, err := s.db.Query(query, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.RowIndex, &t.TargetLang, &t.TranslatedText); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

func (s *Store) GetTotalPendingCount() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT count(*) FROM tasks WHERE status IN ('PENDING', 'FAILED') AND attempts < 5").Scan(&count)
	return count, err
}
