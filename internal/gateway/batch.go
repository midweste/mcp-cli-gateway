package gateway

import (
	"context"
	"fmt"
	"math/rand"
	"sync"

	"github.com/midweste/mcp-cli-gateway/internal/domain"
)

// Assignment pairs a job index with its assigned model alias.
type Assignment struct {
	Index int
	Alias string
}

// AssignModelsForBatch assigns concrete provider-prefixed aliases to batch jobs for max parallelism.
func (g *Gateway) AssignModelsForBatch(ctx context.Context, jobs []DispatchRequest) []Assignment {
	runningModels, err := g.store.RunningModels(ctx)
	if err != nil {
		g.logger.Warn("batch: running models", "error", err)
	}
	runningSet := make(map[string]bool)
	for _, m := range runningModels {
		runningSet[g.registry.AliasFor(m)] = true
	}
	assigned := make(map[string]bool)
	for k := range runningSet {
		assigned[k] = true
	}

	result := make([]Assignment, 0, len(jobs))

	for i, job := range jobs {
		requested := job.Model
		if requested == "" {
			requested = "fast"
		}

		// If the requested model is a tier name, resolve to a concrete alias first.
		if _, err := g.registry.Resolve(requested); err != nil {
			resolved := g.resolveTier(ctx, requested)
			if resolved != "" {
				requested = resolved
			}
		}

		bucket := FindBucketForModel(g.cfg, requested)

		if bucket != nil && !assigned[requested] {
			assigned[requested] = true
			result = append(result, Assignment{i, requested})
			continue
		}

		if bucket != nil {
			alt := pickBucketAlternative(bucket, requested, assigned)
			if alt != "" {
				assigned[alt] = true
				result = append(result, Assignment{i, alt})
			} else {
				result = append(result, Assignment{i, requested})
			}
		} else {
			result = append(result, Assignment{i, requested})
		}
	}

	return result
}

// RunBatch dispatches multiple jobs with goroutines for parallel model slots.
func (g *Gateway) RunBatch(ctx context.Context, jobs []DispatchRequest) ([]domain.BatchResult, error) {
	assignments := g.AssignModelsForBatch(ctx, jobs)

	// Group by assigned model
	type indexedJob struct {
		index int
		job   DispatchRequest
	}
	modelGroups := make(map[string][]indexedJob)
	for _, a := range assignments {
		jobs[a.Index].Model = a.Alias
		modelGroups[a.Alias] = append(modelGroups[a.Alias], indexedJob{a.Index, jobs[a.Index]})
	}

	batchID := fmt.Sprintf("batch-%08x", rand.Int31())

	// Results collected concurrently — each goroutine writes to a unique index.
	results := make([]domain.BatchResult, len(jobs))
	var wg sync.WaitGroup

	// Limit concurrent goroutines (safety net — practical count is 6-9 aliases).
	const maxBatchGoroutines = 10
	sem := make(chan struct{}, maxBatchGoroutines)

	for _, group := range modelGroups {
		wg.Add(1)
		sem <- struct{}{} // acquire slot
		go func(group []indexedJob) {
			defer func() { <-sem }() // release slot
			defer wg.Done()
			// Jobs within same model run serially
			for _, ij := range group {
				ij.job.BatchID = batchID
				result, err := g.Dispatch(ctx, ij.job)

				var br domain.BatchResult
				br.Label = ij.job.Label
				br.Model = ij.job.Model
				if err != nil {
					br.Status = fmt.Sprintf("error: %v", err)
					br.ExitCode = 1
				} else {
					br.ExitCode = result.ExitCode
					br.ResponseText = result.Output
					if result.ExitCode == 0 {
						br.Status = "ok"
					} else {
						br.Status = fmt.Sprintf("exit=%d", result.ExitCode)
					}
				}

				results[ij.index] = br
			}
		}(group)
	}

	wg.Wait()
	return results, nil
}
