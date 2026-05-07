package server

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/midweste/mcp-cli-gateway/internal/domain"
	"github.com/midweste/mcp-cli-gateway/internal/gateway"
)

func (s *Server) registerTools() {
	// ── gateway_dispatch ──
	s.mcp.AddTool(
		mcp.NewTool("gateway_dispatch",
			mcp.WithDescription("Execute a single CLI agent job (auto-selects from Gemini, Codex, or Claude based on availability and load). Returns the agent's response summary."),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				ReadOnlyHint:    boolPtr(false),
				DestructiveHint: boolPtr(false),
				IdempotentHint:  boolPtr(false),
				OpenWorldHint:   boolPtr(true),
			}),
			mcp.WithString("model",
				mcp.Required(),
				mcp.Description("Model tier: lite (lightweight tasks), fast (code gen, tests, refactoring), or deep (architecture, complex reasoning). Provider is auto-selected."),
				mcp.Enum("lite", "fast", "deep"),
			),
			mcp.WithString("prompt",
				mcp.Required(),
				mcp.Description("The prompt text to send to the CLI agent"),
			),
			mcp.WithString("label",
				mcp.Description("Optional human-readable label for this job"),
			),
			mcp.WithString("cwd",
				mcp.Description("Working directory for CLI agent execution"),
			),
			mcp.WithBoolean("sandbox",
				mcp.Description("If true, run in sandbox mode (default: full-auto / yolo)"),
			),
		),
		s.handleDispatch,
	)

	// ── gateway_batch_dispatch ──
	s.mcp.AddTool(
		mcp.NewTool("gateway_batch_dispatch",
			mcp.WithDescription("Execute multiple CLI agent jobs in parallel (auto-selects providers). One goroutine per model slot, cross-provider rebalancing."),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				ReadOnlyHint:    boolPtr(false),
				DestructiveHint: boolPtr(false),
				IdempotentHint:  boolPtr(false),
				OpenWorldHint:   boolPtr(true),
			}),
			mcp.WithArray("jobs",
				mcp.Required(),
				mcp.Description("Array of job objects [{model, prompt, label?, cwd?, sandbox?}]"),
				mcp.Items(map[string]any{
					"type": "object",
					"properties": map[string]any{
						"model": map[string]any{
							"type":        "string",
							"description": "Model tier: lite, fast, or deep",
							"enum":        []string{"lite", "fast", "deep"},
						},
						"prompt": map[string]any{
							"type":        "string",
							"description": "The prompt text to send to the CLI agent",
						},
						"label": map[string]any{
							"type":        "string",
							"description": "Optional human-readable label for this job",
						},
						"cwd": map[string]any{
							"type":        "string",
							"description": "Working directory for CLI agent execution",
						},
						"sandbox": map[string]any{
							"type":        "boolean",
							"description": "If true, run in sandbox mode (default: full-auto / yolo)",
						},
					},
					"required": []string{"model", "prompt"},
				}),
			),
		),
		s.handleBatchDispatch,
	)

	// ── gateway_status ──
	s.mcp.AddTool(
		mcp.NewTool("gateway_status",
			mcp.WithDescription("Queue status per tier with health indicator (ok/busy/slow/saturated)"),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				ReadOnlyHint:    boolPtr(true),
				DestructiveHint: boolPtr(false),
				IdempotentHint:  boolPtr(true),
				OpenWorldHint:   boolPtr(false),
			}),
		),
		s.handleStatus,
	)

	// ── gateway_jobs ──
	s.mcp.AddTool(
		mcp.NewTool("gateway_jobs",
			mcp.WithDescription("List all active jobs (queued, waiting, running, retrying)"),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				ReadOnlyHint:    boolPtr(true),
				DestructiveHint: boolPtr(false),
				IdempotentHint:  boolPtr(true),
				OpenWorldHint:   boolPtr(false),
			}),
		),
		s.handleJobs,
	)

	// ── gateway_pacing ──
	s.mcp.AddTool(
		mcp.NewTool("gateway_pacing",
			mcp.WithDescription("Adaptive pacing state for all models (gap, backoff, streaks)"),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				ReadOnlyHint:    boolPtr(true),
				DestructiveHint: boolPtr(false),
				IdempotentHint:  boolPtr(true),
				OpenWorldHint:   boolPtr(false),
			}),
		),
		s.handlePacing,
	)

	// ── gateway_stats ──
	s.mcp.AddTool(
		mcp.NewTool("gateway_stats",
			mcp.WithDescription("Historical performance stats per model (success rate, timing, retries)"),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				ReadOnlyHint:    boolPtr(true),
				DestructiveHint: boolPtr(false),
				IdempotentHint:  boolPtr(true),
				OpenWorldHint:   boolPtr(false),
			}),
			mcp.WithString("last",
				mcp.Description("Time window, e.g. '1h', '2d', '30m'. Empty = lifetime"),
			),
		),
		s.handleStats,
	)

	// ── gateway_errors ──
	s.mcp.AddTool(
		mcp.NewTool("gateway_errors",
			mcp.WithDescription("Recent failed jobs with error details and retry count"),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				ReadOnlyHint:    boolPtr(true),
				DestructiveHint: boolPtr(false),
				IdempotentHint:  boolPtr(true),
				OpenWorldHint:   boolPtr(false),
			}),
			mcp.WithString("last",
				mcp.Description("Time window, e.g. '1h', '2d'. Empty = lifetime"),
			),
		),
		s.handleErrors,
	)

	// ── gateway_cancel ──
	s.mcp.AddTool(
		mcp.NewTool("gateway_cancel",
			mcp.WithDescription("Cancel jobs by ID, batch ID, or model. Kills running processes."),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				ReadOnlyHint:    boolPtr(false),
				DestructiveHint: boolPtr(true),
				IdempotentHint:  boolPtr(false),
				OpenWorldHint:   boolPtr(false),
			}),
			mcp.WithString("id",
				mcp.Description("Cancel a specific job by numeric ID"),
			),
			mcp.WithString("model",
				mcp.Description("Cancel all active jobs for a model alias (e.g. 'fast')"),
			),
			mcp.WithString("batch_id",
				mcp.Description("Cancel all jobs in a batch"),
			),
		),
		s.handleCancel,
	)

	// ── gateway_retry ──
	s.mcp.AddTool(
		mcp.NewTool("gateway_retry",
			mcp.WithDescription("Retry a failed job using its stored prompt"),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				ReadOnlyHint:    boolPtr(false),
				DestructiveHint: boolPtr(false),
				IdempotentHint:  boolPtr(false),
				OpenWorldHint:   boolPtr(true),
			}),
			mcp.WithNumber("id",
				mcp.Required(),
				mcp.Description("Job ID to retry (from gateway_errors)"),
			),
		),
		s.handleRetry,
	)

	// ── gateway_result ──
	s.mcp.AddTool(
		mcp.NewTool("gateway_result",
			mcp.WithDescription("Get full details of a job by ID, including its response_text if completed"),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				ReadOnlyHint:    boolPtr(true),
				DestructiveHint: boolPtr(false),
				IdempotentHint:  boolPtr(true),
				OpenWorldHint:   boolPtr(false),
			}),
			mcp.WithNumber("id",
				mcp.Required(),
				mcp.Description("Job ID to retrieve"),
			),
		),
		s.handleResult,
	)
}

// ── Tool handler methods ──

func (s *Server) handleDispatch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	dr := gateway.DispatchRequest{
		Model:  argStr(args, "model"),
		Prompt: argStr(args, "prompt"),
		Label:  argStr(args, "label"),
		Cwd:    argStr(args, "cwd"),
	}
	if v, ok := args["sandbox"].(bool); ok {
		dr.Sandbox = v
	}

	if dr.Prompt == "" {
		return mcp.NewToolResultError("prompt is required and must not be empty"), nil
	}

	if err := validateCwdAgainstRoots(ctx, s.mcp, dr.Cwd); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	result, err := s.gateway.Dispatch(ctx, dr)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(toJSON(result)), nil
}

func (s *Server) handleBatchDispatch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	jobsRaw, ok := args["jobs"].([]any)
	if !ok {
		return mcp.NewToolResultError("jobs must be an array of objects"), nil
	}

	jobs := make([]gateway.DispatchRequest, 0, len(jobsRaw))
	for i, j := range jobsRaw {
		jm, ok := j.(map[string]any)
		if !ok {
			return mcp.NewToolResultError(
				fmt.Sprintf("jobs[%d] must be an object with {model, prompt, ...}, got %T", i, j),
			), nil
		}
		dr := gateway.DispatchRequest{
			Model:  argStr(jm, "model"),
			Prompt: argStr(jm, "prompt"),
			Label:  argStr(jm, "label"),
			Cwd:    argStr(jm, "cwd"),
		}
		if v, ok := jm["sandbox"].(bool); ok {
			dr.Sandbox = v
		}
		jobs = append(jobs, dr)
	}

	// Validate all prompts and cwds
	for i, j := range jobs {
		if j.Prompt == "" {
			return mcp.NewToolResultError(
				fmt.Sprintf("jobs[%d]: prompt is required and must not be empty", i),
			), nil
		}
		if err := validateCwdAgainstRoots(ctx, s.mcp, j.Cwd); err != nil {
			return mcp.NewToolResultError(
				fmt.Sprintf("jobs[%d]: %v", i, err),
			), nil
		}
	}

	results, err := s.gateway.RunBatch(ctx, jobs)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(toJSON(results)), nil
}

func (s *Server) handleStatus(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	status, err := s.gateway.Status(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(toJSON(s.aggregateByTier(status))), nil
}

func (s *Server) handleJobs(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	jobs, err := s.gateway.Jobs(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(toJSON(jobs)), nil
}

func (s *Server) handlePacing(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pacing, err := s.gateway.Pacing(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(toJSON(pacing)), nil
}

func (s *Server) handleStats(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	last := argStr(req.GetArguments(), "last")
	stats, err := s.gateway.Stats(ctx, last)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(toJSON(stats)), nil
}

func (s *Server) handleErrors(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	last := argStr(req.GetArguments(), "last")
	errors, err := s.gateway.Errors(ctx, last)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(toJSON(errors)), nil
}

func (s *Server) handleCancel(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	result, err := s.gateway.Cancel(ctx,
		argStr(args, "id"),
		argStr(args, "model"),
		argStr(args, "batch_id"),
	)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(toJSON(result)), nil
}

func (s *Server) handleRetry(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := argInt64(req.GetArguments(), "id")
	if err != nil {
		return mcp.NewToolResultError("id must be a numeric job ID. Use gateway_errors to see failed jobs."), nil
	}
	result, gErr := s.gateway.Retry(ctx, id)
	if gErr != nil {
		return mcp.NewToolResultError(gErr.Error()), nil
	}
	return mcp.NewToolResultText(toJSON(result)), nil
}

func (s *Server) handleResult(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := argInt64(req.GetArguments(), "id")
	if err != nil {
		return mcp.NewToolResultError("id must be a numeric job ID. Use gateway_jobs to see active jobs."), nil
	}
	result, gErr := s.gateway.Result(ctx, id)
	if gErr != nil {
		return mcp.NewToolResultError(gErr.Error()), nil
	}
	if result == nil {
		return mcp.NewToolResultError("Job not found"), nil
	}
	return mcp.NewToolResultText(toJSON(result)), nil
}

func argStr(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

// argInt64 extracts a float64 argument and converts to int64.
// MCP/JSON transports deliver all numbers as float64.
func argInt64(args map[string]any, key string) (int64, error) {
	v, ok := args[key].(float64)
	if !ok {
		return 0, fmt.Errorf("%s: required (number)", key)
	}
	return int64(v), nil
}

func boolPtr(b bool) *bool {
	return &b
}

// healthRank maps health strings to severity levels for worst-wins aggregation.
var healthRank = map[string]int{"ok": 0, "busy": 1, "slow": 2, "saturated": 3}

// aggregateByTier collapses alias-keyed status (codex-fast, gemini-fast, ...)
// into tier-keyed status (lite, fast, deep) — the only names the AI sees.
func (s *Server) aggregateByTier(aliasStatus map[string]domain.ModelStatus) map[string]domain.ModelStatus {
	tierStatus := make(map[string]domain.ModelStatus)

	for alias, st := range aliasStatus {
		tier, ok := s.aliasToTier[alias]
		if !ok {
			continue
		}
		existing, found := tierStatus[tier]
		if !found {
			tierStatus[tier] = st
			continue
		}
		// Merge: sum counts, take worst health.
		existing.Running += st.Running
		existing.Queued += st.Queued
		existing.Retrying += st.Retrying
		existing.AvailableConcurrent += st.AvailableConcurrent
		existing.AvailableQueue += st.AvailableQueue
		if healthRank[st.Health] > healthRank[existing.Health] {
			existing.Health = st.Health
		}
		tierStatus[tier] = existing
	}

	return tierStatus
}
