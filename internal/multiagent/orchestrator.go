package multiagent

import "context"

// Runner executes one delegated task and returns its result.
type Runner func(ctx context.Context, task Task) Result

// Orchestrator owns planning, validation, execution ordering, and aggregation.
type Orchestrator struct {
	policy     Policy
	planner    *Planner
	aggregator *Aggregator
}

// NewOrchestrator creates an orchestrator with default planner and aggregator helpers.
func NewOrchestrator(policy Policy) *Orchestrator {
	return &Orchestrator{
		policy:     policy,
		planner:    NewPlanner(policy),
		aggregator: NewAggregator(),
	}
}

// BuildPlan validates and returns a multi-agent execution plan.
func (o *Orchestrator) BuildPlan(objective string, tasks []Task) (Plan, error) {
	return o.planner.Build(objective, tasks)
}

// Run executes the supplied plan using the provided task runner.
func (o *Orchestrator) Run(ctx context.Context, plan Plan, runner Runner) ([]Result, Aggregate, error) {
	if err := o.policy.Validate(plan); err != nil {
		return nil, Aggregate{}, err
	}
	results := make([]Result, 0, len(plan.Tasks))
	if plan.Mode == TaskModeParallel {
		type item struct {
			index  int
			result Result
		}
		ch := make(chan item, len(plan.Tasks))
		sem := make(chan struct{}, plan.MaxConcurrent)
		for i, task := range plan.Tasks {
			i, task := i, task
			go func() {
				sem <- struct{}{}
				defer func() { <-sem }()
				ch <- item{index: i, result: runner(ctx, task)}
			}()
		}
		results = make([]Result, len(plan.Tasks))
		for range plan.Tasks {
			select {
			case <-ctx.Done():
				return nil, Aggregate{}, ctx.Err()
			case item := <-ch:
				results[item.index] = item.result
			}
		}
	} else {
		for _, task := range plan.Tasks {
			select {
			case <-ctx.Done():
				return nil, Aggregate{}, ctx.Err()
			default:
				results = append(results, runner(ctx, task))
			}
		}
	}
	return results, o.aggregator.Aggregate(results), nil
}
