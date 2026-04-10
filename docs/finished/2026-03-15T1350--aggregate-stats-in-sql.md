# Aggregate Stats in SQL

> Created: 2026-03-15 13:50 (local)
> Status: Debt

## Requirement

### Move Stats computation from Go to SQL aggregate queries

- **What**: `Stats()` in `commands.go` calls `ListCompleted()` which loads every completed request for a model into a Go slice, then iterates in-memory to compute counts, averages, p95, and peak concurrency. For a gateway processing thousands of jobs, this could consume significant memory and CPU.
- **Where**: [commands.go:122-199](file:///home/eric/websites/codecide/dotai/.agent/mcp/mcp-gemini-gateway/internal/gateway/commands.go#L122-L199), [store.go ListCompleted](file:///home/eric/websites/codecide/dotai/.agent/mcp/mcp-gemini-gateway/internal/database/store.go#L242-L254)
- **Why**: Current approach is O(n) memory where n = completed requests in the time window. At scale (10K+ jobs), this causes unnecessary allocation pressure. SQLite can compute COUNT, AVG, and percentile aggregations server-side, returning only the final numbers.
- **How**: Replace `ListCompleted` + in-Go aggregation with a single SQL query per model:
  ```sql
  SELECT
    COUNT(*) AS total_jobs,
    SUM(CASE WHEN status='done' THEN 1 ELSE 0 END) AS succeeded,
    SUM(CASE WHEN status='failed' THEN 1 ELSE 0 END) AS failed,
    SUM(CASE WHEN status='cancelled' THEN 1 ELSE 0 END) AS cancelled,
    SUM(retry_count) AS rate_limited_attempts,
    AVG(finished_at - started_at) AS avg_execution_s,
    AVG(started_at - created_at) AS avg_wait_s,
    SUM(CASE WHEN exit_code=-1 THEN 1 ELSE 0 END) AS timeouts
  FROM requests
  WHERE model=? AND finished_at IS NOT NULL AND finished_at > ?
  ```
  P95 and peak concurrency are harder in pure SQL. Options: (a) use SQLite window functions for p95, (b) keep those two metrics as Go-side computation on a smaller result set (only timestamps), or (c) accept approximate values.
  Add a `StatsAggregate(ctx, model, since) (*ModelStats, error)` method to `Store`.
- **Priority**: Low
- **Effort**: Medium — SQL rewrite + test updates, but no behavioral change
