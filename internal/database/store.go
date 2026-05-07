package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/midweste/mcp-cli-gateway/internal/config"
	"github.com/midweste/mcp-cli-gateway/internal/domain"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

// Store implements all database interfaces using SQLite.
// Satisfies: RequestReader, RequestWriter, PacingStore, Maintainer.
type Store struct {
	db     *sql.DB
	cfg    *config.Config
	logger *slog.Logger
}

// NewStore opens (or creates) the SQLite database and applies schema + migrations.
func NewStore(cfg *config.Config, dbPath string, logger *slog.Logger) (*Store, error) {
	if dbPath == "" {
		dbPath = cfg.DB_PATH()
	}

	dsn := dbPath
	if dbPath == ":memory:" {
		dsn = "file::memory:?mode=memory&cache=shared"
	} else {
		dir := filepath.Dir(dbPath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db directory %s: %w", dir, err)
		}
		dsn = "file:" + dbPath
	}

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Pin to one connection: ensures PRAGMAs persist and avoids SQLITE_BUSY.
	// SQLite is single-writer; one connection is fine for this workload.
	db.SetMaxOpenConns(1)

	// Apply PRAGMAs explicitly (DSN _pragma= syntax is silently ignored by both
	// modernc.org/sqlite and ncruces/go-sqlite3 drivers).
	if dbPath != ":memory:" {
		for _, pragma := range []string{
			"PRAGMA journal_mode=WAL",
			"PRAGMA busy_timeout=10000",
			"PRAGMA synchronous=NORMAL",
		} {
			if _, err := db.Exec(pragma); err != nil {
				db.Close()
				return nil, fmt.Errorf("exec %s: %w", pragma, err)
			}
		}
	}

	s := &Store{db: db, cfg: cfg, logger: logger}

	if err := s.applySchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	if err := s.runMigrations(); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return s, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// DB returns the underlying *sql.DB for exclusive transaction use.
func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) applySchema() error {
	_, err := s.db.Exec(SchemaSQL)
	return err
}

// columnMigration defines a single ALTER TABLE ADD COLUMN migration.
// The guard column is checked via columnExists before executing the SQL list.
type columnMigration struct {
	table     string
	guard     string   // column to check — if it exists, skip this migration
	addSQL    []string // one or more ALTER TABLE statements
}

// columnMigrations is the ordered list of schema migrations.
// Add new column migrations here; the runner applies them idempotently.
var columnMigrations = []columnMigration{
	{
		table: "requests", guard: "prompt_text",
		addSQL: []string{"ALTER TABLE requests ADD COLUMN prompt_text TEXT"},
	},
	{
		table: "requests", guard: "tokens_in",
		addSQL: []string{
			"ALTER TABLE requests ADD COLUMN tokens_in INTEGER",
			"ALTER TABLE requests ADD COLUMN tokens_out INTEGER",
			"ALTER TABLE requests ADD COLUMN tokens_cached INTEGER",
			"ALTER TABLE requests ADD COLUMN tokens_thoughts INTEGER",
			"ALTER TABLE requests ADD COLUMN tool_calls INTEGER",
			"ALTER TABLE requests ADD COLUMN api_latency_ms INTEGER",
		},
	},
	{
		table: "requests", guard: "batch_id",
		addSQL: []string{"ALTER TABLE requests ADD COLUMN batch_id TEXT"},
	},
	{
		table: "requests", guard: "response_text",
		addSQL: []string{"ALTER TABLE requests ADD COLUMN response_text TEXT"},
	},
}

func (s *Store) runMigrations() error {
	for _, m := range columnMigrations {
		if s.columnExists(m.table, m.guard) {
			continue
		}
		for _, sql := range m.addSQL {
			if _, err := s.db.Exec(sql); err != nil {
				return fmt.Errorf("migration %s.%s: %w", m.table, m.guard, err)
			}
		}
	}

	// Index migration: rebuild idx_requests_active to include 'queued' status.
	// SQLite's CREATE INDEX IF NOT EXISTS won't update an existing index,
	// so we drop and re-create unconditionally (idempotent after first run).
	if _, err := s.db.Exec("DROP INDEX IF EXISTS idx_requests_active"); err != nil {
		return fmt.Errorf("drop idx_requests_active: %w", err)
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_requests_active
		ON requests(model, status)
		WHERE status IN ('waiting', 'running', 'retrying', 'queued')`); err != nil {
		return fmt.Errorf("create idx_requests_active: %w", err)
	}

	return nil
}

func (s *Store) columnExists(table, column string) bool {
	rows, err := s.db.Query("SELECT name FROM pragma_table_info(?) WHERE name=?", table, column)
	if err != nil {
		return false
	}
	defer rows.Close()
	return rows.Next()
}

// selectColumns is the canonical column list for all request queries.
// Single source of truth — add new columns here only.
const selectColumns = `id, model, status, label, prompt_hash, prompt_text, pid, cwd,
		created_at, started_at, finished_at, exit_code, retry_count, error,
		tokens_in, tokens_out, tokens_cached, tokens_thoughts, tool_calls,
		api_latency_ms, batch_id, response_text`

// ── RequestReader implementation ──

// CountRunning returns the number of running requests for a model.
func (s *Store) CountRunning(ctx context.Context, model string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM requests WHERE model=? AND status=?", model, domain.StatusRunning,
	).Scan(&count)
	return count, err
}

// CountPending returns the total pending (queued+waiting+running+retrying) for a model.
func (s *Store) CountPending(ctx context.Context, model string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM requests WHERE model=? AND status IN ('queued','waiting','running','retrying')", model,
	).Scan(&count)
	return count, err
}

// CountByStatus returns count of requests with a given status for a model.
func (s *Store) CountByStatus(ctx context.Context, model, status string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM requests WHERE model=? AND status=?", model, status,
	).Scan(&count)
	return count, err
}

// StatusCounts returns counts for all active statuses for a model in a single query.
// Keys: domain.StatusRunning, domain.StatusWaiting, domain.StatusQueued, domain.StatusRetrying.
func (s *Store) StatusCounts(ctx context.Context, model string) (map[string]int, error) {
	counts := map[string]int{domain.StatusRunning: 0, domain.StatusWaiting: 0, domain.StatusQueued: 0, domain.StatusRetrying: 0}
	rows, err := s.db.QueryContext(ctx,
		`SELECT status, COUNT(*) FROM requests
		 WHERE model=? AND status IN ('running','waiting','queued','retrying')
		 GROUP BY status`, model,
	)
	if err != nil {
		return counts, err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return counts, err
		}
		counts[status] = count
	}
	return counts, rows.Err()
}

// GetRequest returns a single request by ID.
func (s *Store) GetRequest(ctx context.Context, id int64) (*domain.Request, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+selectColumns+` FROM requests WHERE id=?`, id,
	)
	return scanRequest(row)
}

// ListActive returns all requests in active states (queued, waiting, running, retrying).
func (s *Store) ListActive(ctx context.Context) ([]domain.Request, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+selectColumns+` FROM requests
		 WHERE status IN ('queued','waiting','running','retrying')
		 ORDER BY created_at`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRequests(rows)
}

// ListFailed returns recent failed requests since a cutoff time, limited.
func (s *Store) ListFailed(ctx context.Context, since time.Time, limit int) ([]domain.Request, error) {
	cutoff := float64(since.Unix())
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+selectColumns+` FROM requests
		 WHERE status='failed' AND finished_at IS NOT NULL AND finished_at > ?
		 ORDER BY finished_at DESC LIMIT ?`, cutoff, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRequests(rows)
}

// ListCompleted returns completed requests for a model since a cutoff time.
func (s *Store) ListCompleted(ctx context.Context, model string, since time.Time) ([]domain.Request, error) {
	cutoff := float64(since.Unix())
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+selectColumns+` FROM requests
		 WHERE model=? AND finished_at IS NOT NULL AND finished_at > ?`, model, cutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRequests(rows)
}

// StatsAggregate computes aggregate statistics for a model using SQL, avoiding
// loading all request rows into Go memory. Returns nil if no matching rows exist.
func (s *Store) StatsAggregate(ctx context.Context, model string, since time.Time) (*domain.StatsAggregate, error) {
	cutoff := float64(since.Unix())
	row := s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*) AS total_jobs,
			COALESCE(SUM(CASE WHEN status='done' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status='failed' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status='cancelled' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(retry_count), 0),
			COALESCE(SUM(CASE WHEN exit_code=-1 THEN 1 ELSE 0 END), 0),
			AVG(CASE WHEN started_at IS NOT NULL AND finished_at IS NOT NULL
				THEN finished_at - started_at END),
			AVG(CASE WHEN started_at IS NOT NULL
				THEN started_at - created_at END)
		FROM requests
		WHERE model=? AND finished_at IS NOT NULL AND finished_at > ?`, model, cutoff,
	)

	var agg domain.StatsAggregate
	err := row.Scan(
		&agg.TotalJobs, &agg.Succeeded, &agg.Failed, &agg.Cancelled,
		&agg.RateLimitedAttempts, &agg.Timeouts,
		&agg.AvgExecutionS, &agg.AvgWaitS,
	)
	if err != nil {
		return nil, err
	}
	if agg.TotalJobs == 0 {
		return nil, nil
	}
	return &agg, nil
}

// ListTimestamps returns only the start/finish timestamp pairs for completed
// requests. Used for p95 and peak-concurrent computation without loading full rows.
func (s *Store) ListTimestamps(ctx context.Context, model string, since time.Time) ([]domain.TimestampPair, error) {
	cutoff := float64(since.Unix())
	rows, err := s.db.QueryContext(ctx,
		`SELECT started_at, finished_at FROM requests
		 WHERE model=? AND finished_at IS NOT NULL AND finished_at > ?
		   AND started_at IS NOT NULL`, model, cutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pairs []domain.TimestampPair
	for rows.Next() {
		var p domain.TimestampPair
		if err := rows.Scan(&p.StartedAt, &p.FinishedAt); err != nil {
			return nil, err
		}
		pairs = append(pairs, p)
	}
	return pairs, rows.Err()
}

// ListActiveByBatchID returns active requests for a batch.
func (s *Store) ListActiveByBatchID(ctx context.Context, batchID string) ([]domain.Request, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+selectColumns+` FROM requests
		 WHERE batch_id=? AND status IN ('waiting','running','retrying')`, batchID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRequests(rows)
}

// ListActiveByModel returns active requests for a model.
func (s *Store) ListActiveByModel(ctx context.Context, model string) ([]domain.Request, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+selectColumns+` FROM requests
		 WHERE model=? AND status IN ('waiting','running','retrying')`, model,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRequests(rows)
}

// RunningModels returns all model names that have a running request.
func (s *Store) RunningModels(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT DISTINCT model FROM requests WHERE status=?", domain.StatusRunning,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var models []string
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			return nil, err
		}
		models = append(models, m)
	}
	return models, rows.Err()
}

// ── RequestWriter implementation ──

// InsertRequest inserts a new request and returns its ID.
func (s *Store) InsertRequest(ctx context.Context, req *domain.Request) (int64, error) {
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO requests (model, status, label, prompt_hash, prompt_text, pid, cwd,
		                       created_at, retry_count, batch_id, response_text)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.Model, req.Status, req.Label, req.PromptHash, req.PromptText,
		req.PID, req.Cwd, req.CreatedAt, req.RetryCount, req.BatchID, req.ResponseText,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// validRequestColumns defines the set of columns that may be passed to
// UpdateStatus via the fields map. This prevents accidental SQL injection
// through column name concatenation.
var validRequestColumns = map[string]bool{
	"started_at":       true,
	"finished_at":      true,
	"exit_code":        true,
	"error":            true,
	"response_text":    true,
	"retry_count":      true,
	"pid":              true,
	"prompt_text":      true,
	"tokens_in":        true,
	"tokens_out":       true,
	"tokens_cached":    true,
	"tokens_thoughts":  true,
	"tool_calls":       true,
	"api_latency_ms":   true,
}

// UpdateStatus updates a request's status and optional fields.
func (s *Store) UpdateStatus(ctx context.Context, id int64, status string, fields map[string]any) error {
	setClauses := "status=?"
	args := []any{status}

	for col, val := range fields {
		if !validRequestColumns[col] {
			return fmt.Errorf("UpdateStatus: unknown column %q", col)
		}
		setClauses += ", " + col + "=?"
		args = append(args, val)
	}
	args = append(args, id)

	_, err := s.db.ExecContext(ctx,
		"UPDATE requests SET "+setClauses+" WHERE id=?", args...,
	)
	return err
}

// ── PacingStore implementation ──

// GetPacing returns the pacing state for a model.
func (s *Store) GetPacing(ctx context.Context, model string) (*domain.PacingState, error) {
	row := s.db.QueryRowContext(ctx,
		"SELECT model, min_gap_ms, last_request_at, backoff_ms, consecutive_ok, total_ok, total_rate_limited FROM pacing WHERE model=?",
		model,
	)
	var p domain.PacingState
	err := row.Scan(&p.Model, &p.MinGapMs, &p.LastRequestAt, &p.BackoffMs,
		&p.ConsecutiveOK, &p.TotalOK, &p.TotalRateLimited)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// validPacingColumns defines the set of columns that may be passed to
// UpdatePacing via the fields map.
var validPacingColumns = map[string]bool{
	"min_gap_ms":        true,
	"last_request_at":   true,
	"backoff_ms":        true,
	"consecutive_ok":    true,
	"total_ok":          true,
	"total_rate_limited": true,
}

// UpdatePacing updates pacing fields for a model.
func (s *Store) UpdatePacing(ctx context.Context, model string, fields map[string]any) error {
	setClauses := ""
	args := make([]any, 0, len(fields)+1)
	first := true
	for col, val := range fields {
		if !validPacingColumns[col] {
			return fmt.Errorf("UpdatePacing: unknown column %q", col)
		}
		if !first {
			setClauses += ", "
		}
		setClauses += col + "=?"
		args = append(args, val)
		first = false
	}
	args = append(args, model)

	_, err := s.db.ExecContext(ctx,
		"UPDATE pacing SET "+setClauses+" WHERE model=?", args...,
	)
	return err
}

// SeedPacing inserts pacing rows for all configured models (INSERT OR IGNORE).
func (s *Store) SeedPacing(ctx context.Context, registry *domain.ModelRegistry, cfg *config.Config) error {
	stmt, err := s.db.PrepareContext(ctx,
		"INSERT OR IGNORE INTO pacing (model, min_gap_ms) VALUES (?, ?)",
	)
	if err != nil {
		return err
	}
	defer stmt.Close()

	registry.ForEach(func(alias, model string) {
		gap := cfg.InitialGapForAlias(alias)
		if _, execErr := stmt.ExecContext(ctx, model, gap); execErr != nil {
			s.logger.Warn("seed pacing failed", "model", model, "error", execErr)
		}
	})
	return nil
}

// ── Maintainer implementation ──

// CleanStalePIDs marks running/waiting/retrying requests with dead PIDs as failed.
func (s *Store) CleanStalePIDs(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, pid FROM requests WHERE status IN ('waiting','running','retrying') AND pid IS NOT NULL",
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	var staleIDs []int64
	for rows.Next() {
		var id int64
		var pid int
		if err := rows.Scan(&id, &pid); err != nil {
			return err
		}
		if !isProcessAlive(pid) {
			staleIDs = append(staleIDs, id)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	now := domain.NowUnix()
	for _, id := range staleIDs {
		if _, err := s.db.ExecContext(ctx,
			fmt.Sprintf("UPDATE requests SET status='%s', error='process died (stale PID)', finished_at=? WHERE id=?", domain.StatusFailed),
			now, id,
		); err != nil {
			s.logger.Warn("failed to mark stale PID", "id", id, "error", err)
		}
	}
	return nil
}

// secondsPerDay is the number of seconds in a day, used for cleanup cutoff calculation.
const secondsPerDay = 86400

// CleanupOldRequests deletes completed/failed requests older than the configured days.
func (s *Store) CleanupOldRequests(ctx context.Context) error {
	cutoff := domain.NowUnix() - float64(s.cfg.CLEANUP_DAYS())*secondsPerDay
	_, err := s.db.ExecContext(ctx,
		"DELETE FROM requests WHERE status IN ('done','failed') AND finished_at < ?", cutoff,
	)
	return err
}

// isProcessAlive checks if a PID is still running.
func isProcessAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// ── Row scanning helpers (DRY) ──

type scannable interface {
	Scan(dest ...any) error
}

func scanRequest(row scannable) (*domain.Request, error) {
	var r domain.Request
	var label, promptText, errStr, batchID, responseText sql.NullString
	err := row.Scan(
		&r.ID, &r.Model, &r.Status, &label, &r.PromptHash, &promptText,
		&r.PID, &r.Cwd, &r.CreatedAt, &r.StartedAt, &r.FinishedAt,
		&r.ExitCode, &r.RetryCount, &errStr,
		&r.TokensIn, &r.TokensOut, &r.TokensCached, &r.TokensThoughts,
		&r.ToolCalls, &r.APILatencyMs, &batchID, &responseText,
	)
	if err != nil {
		return nil, err
	}
	r.Label = label.String
	r.PromptText = promptText.String
	r.Error = errStr.String
	r.BatchID = batchID.String
	r.ResponseText = responseText.String
	return &r, nil
}

func scanRequests(rows *sql.Rows) ([]domain.Request, error) {
	var requests []domain.Request
	for rows.Next() {
		r, err := scanRequest(rows)
		if err != nil {
			return nil, err
		}
		requests = append(requests, *r)
	}
	return requests, rows.Err()
}
