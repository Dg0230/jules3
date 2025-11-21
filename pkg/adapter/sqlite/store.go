package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"excel-translator/pkg/domain"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	s := &Store{db: db}
	if err := s.Init(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Ensure Store implements domain.TaskRepository
var _ domain.TaskRepository = (*Store)(nil)

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Init() error {
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

func (s *Store) AddTasks(ctx context.Context, tasks []domain.Task) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO tasks (job_id, row_index, source_content, target_lang, status)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(job_id, row_index, target_lang) DO NOTHING
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, task := range tasks {
		_, err := stmt.ExecContext(ctx, task.JobID, task.RowIndex, task.SourceContent, task.TargetLang, domain.StatusPending)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) GetPendingTasks(ctx context.Context, jobID string, limit int) ([]domain.Task, error) {
	query := `
		SELECT id, row_index, source_content, target_lang, attempts
		FROM tasks
		WHERE job_id = ? AND status IN ('PENDING', 'FAILED') AND attempts < 5
		ORDER BY id ASC
		LIMIT ?
	`
	rows, err := s.db.QueryContext(ctx, query, jobID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []domain.Task
	for rows.Next() {
		var t domain.Task
		if err := rows.Scan(&t.ID, &t.RowIndex, &t.SourceContent, &t.TargetLang, &t.Attempts); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

func (s *Store) MarkProcessing(ctx context.Context, taskIDs []int64) error {
	if len(taskIDs) == 0 {
		return nil
	}

	args := make([]interface{}, len(taskIDs))
	for i, id := range taskIDs {
		args[i] = id
	}

	placeholders := strings.Repeat("?,", len(taskIDs))
	placeholders = placeholders[:len(placeholders)-1]

	query := fmt.Sprintf("UPDATE tasks SET status = ?, attempts = attempts + 1, updated_at = CURRENT_TIMESTAMP WHERE id IN (%s)", placeholders)

	finalArgs := append([]interface{}{domain.StatusProcessing}, args...)

	_, err := s.db.ExecContext(ctx, query, finalArgs...)
	return err
}

func (s *Store) MarkComplete(ctx context.Context, tasks []domain.Task) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		UPDATE tasks
		SET status = ?, translated_text = ?, error_message = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, t := range tasks {
		_, err := stmt.ExecContext(ctx, domain.StatusComplete, t.TranslatedText, t.ID)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) MarkFailed(ctx context.Context, taskIDs []int64, errMsg string) error {
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

	finalArgs := append([]interface{}{domain.StatusFailed, errMsg}, args...)

	_, err := s.db.ExecContext(ctx, query, finalArgs...)
	return err
}

func (s *Store) GetCompletedTasks(ctx context.Context, jobID string) ([]domain.Task, error) {
	query := `
		SELECT row_index, target_lang, translated_text
		FROM tasks
		WHERE job_id = ? AND status = 'COMPLETE'
	`
	rows, err := s.db.QueryContext(ctx, query, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []domain.Task
	for rows.Next() {
		var t domain.Task
		if err := rows.Scan(&t.RowIndex, &t.TargetLang, &t.TranslatedText); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}
