package server

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/midweste/mcp-cli-gateway/internal/domain"
	"github.com/midweste/mcp-cli-gateway/internal/gateway"
)

func (s *MCPServer) registerTools() {
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
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
		},
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
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
		},
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
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			status, err := s.gateway.Status(ctx)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSON(s.aggregateByTier(status))), nil
		},
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
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			jobs, err := s.gateway.Jobs(ctx)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSON(jobs)), nil
		},
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
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			pacing, err := s.gateway.Pacing(ctx)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSON(pacing)), nil
		},
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
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			last := argStr(req.GetArguments(), "last")
			stats, err := s.gateway.Stats(ctx, last)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSON(stats)), nil
		},
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
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			last := argStr(req.GetArguments(), "last")
			errors, err := s.gateway.Errors(ctx, last)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSON(errors)), nil
		},
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
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
		},
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
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := req.GetArguments()
			idVal, ok := args["id"].(float64)
			if !ok {
				return mcp.NewToolResultError("id must be a numeric job ID. Use gateway_errors to see failed jobs."), nil
			}
			id := int64(idVal)
			result, err := s.gateway.Retry(ctx, id)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSON(result)), nil
		},
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
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := req.GetArguments()
			idVal, ok := args["id"].(float64)
			if !ok {
				return mcp.NewToolResultError("id must be a numeric job ID. Use gateway_jobs to see active jobs."), nil
			}
			id := int64(idVal)
			result, err := s.gateway.Result(ctx, id)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if result == nil {
				return mcp.NewToolResultError("Job not found"), nil
			}
			return mcp.NewToolResultText(toJSON(result)), nil
		},
	)
}

func argStr(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

func boolPtr(b bool) *bool {
	return &b
}

// healthRank maps health strings to severity levels for worst-wins aggregation.
var healthRank = map[string]int{"ok": 0, "busy": 1, "slow": 2, "saturated": 3}

// aggregateByTier collapses alias-keyed status (codex-fast, gemini-fast, ...)
// into tier-keyed status (lite, fast, deep) — the only names the AI sees.
func (s *MCPServer) aggregateByTier(aliasStatus map[string]domain.ModelStatus) map[string]domain.ModelStatus {
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
