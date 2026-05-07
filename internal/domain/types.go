package domain

import (
	"math"
	"time"
)

// Request statuses — single source of truth for all status string values.
// Use these constants everywhere instead of bare string literals.
const (
	StatusQueued    = "queued"
	StatusWaiting   = "waiting"
	StatusRunning   = "running"
	StatusRetrying  = "retrying"
	StatusDone      = "done"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
)

// Request represents a gateway job request stored in SQLite.
type Request struct {
	ID             int64    `json:"id"`
	Model          string   `json:"model"`
	Status         string   `json:"status"`
	Label          string   `json:"label,omitempty"`
	PromptHash     string   `json:"prompt_hash"`
	PromptText     string   `json:"prompt_text,omitempty"`
	PID            int      `json:"pid"`
	Cwd            string   `json:"cwd"`
	CreatedAt      float64  `json:"created_at"`
	StartedAt      *float64 `json:"started_at,omitempty"`
	FinishedAt     *float64 `json:"finished_at,omitempty"`
	ExitCode       *int     `json:"exit_code,omitempty"`
	RetryCount     int      `json:"retry_count"`
	Error          string   `json:"error,omitempty"`
	TokensIn       *int     `json:"tokens_in,omitempty"`
	TokensOut      *int     `json:"tokens_out,omitempty"`
	TokensCached   *int     `json:"tokens_cached,omitempty"`
	TokensThoughts *int     `json:"tokens_thoughts,omitempty"`
	ToolCalls      *int     `json:"tool_calls,omitempty"`
	APILatencyMs   *int     `json:"api_latency_ms,omitempty"`
	BatchID        string   `json:"batch_id,omitempty"`
	ResponseText   string   `json:"response_text,omitempty"`
}

// PacingState represents the adaptive pacing state for a model.
type PacingState struct {
	Model            string  `json:"model"`
	MinGapMs         int     `json:"min_gap_ms"`
	LastRequestAt    float64 `json:"last_request_at"`
	BackoffMs        int     `json:"backoff_ms"`
	ConsecutiveOK    int     `json:"consecutive_ok"`
	TotalOK          int     `json:"total_ok"`
	TotalRateLimited int     `json:"total_rate_limited"`
}

// JobStatus represents an active job for the jobs command.
type JobStatus struct {
	ID           int64   `json:"id"`
	Model        string  `json:"model"`
	Status       string  `json:"status"`
	Label        string  `json:"label"`
	RetryCount   int     `json:"retry_count"`
	RunningTimeS *float64 `json:"running_time_s"`
	Created      string  `json:"created"`
}

// ModelStatus represents per-model queue health for the status command.
type ModelStatus struct {
	Running            int    `json:"running"`
	Queued             int    `json:"queued"`
	Retrying           int    `json:"retrying"`
	AvailableConcurrent int   `json:"available_concurrent"`
	AvailableQueue     int    `json:"available_queue"`
	Health             string `json:"health"`
}

// PacingInfo represents pacing state for the pacing command.
type PacingInfo struct {
	MinGapMs         int `json:"min_gap_ms"`
	BackoffMs        int `json:"backoff_ms"`
	ConsecutiveOK    int `json:"consecutive_ok"`
	TotalOK          int `json:"total_ok"`
	TotalRateLimited int `json:"total_rate_limited"`
}

// ModelStats represents historical performance for the stats command.
type ModelStats struct {
	TotalJobs           int      `json:"total_jobs"`
	Succeeded           int      `json:"succeeded,omitempty"`
	Failed              int      `json:"failed,omitempty"`
	Cancelled           int      `json:"cancelled,omitempty"`
	RateLimitedAttempts int      `json:"rate_limited_attempts,omitempty"`
	SuccessRate         float64  `json:"success_rate,omitempty"`
	AvgExecutionS       *float64 `json:"avg_execution_s,omitempty"`
	AvgWaitS            *float64 `json:"avg_wait_s,omitempty"`
	AvgRetries          float64  `json:"avg_retries,omitempty"`
	P95ExecutionS       *float64 `json:"p95_execution_s,omitempty"`
	PeakConcurrent      int      `json:"peak_concurrent,omitempty"`
	Timeouts            int      `json:"timeouts,omitempty"`
	CurrentMinGapMs     *int     `json:"current_min_gap_ms,omitempty"`
}

// StatsResult represents the typed response from the Stats command.
type StatsResult struct {
	Period string                `json:"period"`
	Models map[string]ModelStats `json:"models"`
}

// StatsAggregate holds SQL-computed aggregate stats for a model.
// Used by Store.StatsAggregate to avoid loading all rows into Go memory.
type StatsAggregate struct {
	TotalJobs           int
	Succeeded           int
	Failed              int
	Cancelled           int
	RateLimitedAttempts int
	Timeouts            int
	AvgExecutionS       *float64
	AvgWaitS            *float64
}

// TimestampPair holds start/finish timestamps for p95 and peak-concurrent
// computation. Much lighter than loading full Request rows.
type TimestampPair struct {
	StartedAt  float64
	FinishedAt float64
}

// ErrorInfo represents a failed job for the errors command.
type ErrorInfo struct {
	ID       int64   `json:"id"`
	Label    string  `json:"label"`
	Model    string  `json:"model"`
	ExitCode *int    `json:"exit_code"`
	Error    string  `json:"error"`
	Retries  int     `json:"retries"`
	ExecS    *float64 `json:"exec_s,omitempty"`
	Finished string  `json:"finished"`
}

// ErrorsResult represents the typed response from the Errors command.
type ErrorsResult struct {
	Count  int         `json:"count"`
	Errors []ErrorInfo `json:"errors"`
}

// CancelResult represents the result of a cancel operation.
type CancelResult struct {
	Cancelled []int64 `json:"cancelled"`
	Count     int     `json:"count"`
	BatchID   string  `json:"batch_id,omitempty"`
	Error     string  `json:"error,omitempty"`
}

// DispatchResult represents the result of a dispatch or batch dispatch.
type DispatchResult struct {
	RequestID int64  `json:"request_id"`
	ExitCode  int    `json:"exit_code"`
	Output    string `json:"output,omitempty"`
	Error     string `json:"error,omitempty"`
}

// BatchResult represents a single job result within a batch.
type BatchResult struct {
	Label        string `json:"label"`
	Model        string `json:"model"`
	Status       string `json:"status"`
	ExitCode     int    `json:"exit_code"`
	ResponseText string `json:"response_text,omitempty"`
}

// FormatTime formats a Unix timestamp for display.
func FormatTime(ts float64) string {
	return time.Unix(int64(ts), 0).Format("2006-01-02 15:04:05")
}

// FormatTimeShort formats a Unix timestamp with time only.
func FormatTimeShort(ts float64) string {
	return time.Unix(int64(ts), 0).Format("15:04:05")
}

// Now returns the current time as a float64 Unix timestamp.
// Override in tests for determinism: domain.Now = func() float64 { return 1234567890.0 }
var Now func() float64 = NowUnix

// NowUnix returns the current time as a float64 Unix timestamp.
// This is the default implementation behind domain.Now.
// Precision: second-level (Unix() truncates to seconds). Sub-second
// precision is not needed for gateway timing; this is documented intent.
func NowUnix() float64 {
	return float64(time.Now().Unix())
}

// ExecDuration returns the execution duration (finishedAt - startedAt) rounded
// to 1 decimal place, or nil if either timestamp is missing.
// Centralizes the repeated duration calculation in Jobs, Errors, and Stats.
func ExecDuration(startedAt, finishedAt *float64) *float64 {
	if startedAt == nil || finishedAt == nil {
		return nil
	}
	v := math.Round((*finishedAt-*startedAt)*10) / 10
	return &v
}
