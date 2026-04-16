package multiagent

import "slices"

// Aggregator merges task results into one top-level outcome.
type Aggregator struct{}

// NewAggregator creates a result aggregator.
func NewAggregator() *Aggregator {
	return &Aggregator{}
}

// Aggregate merges results, de-duplicates risks, and rolls up counts.
func (a *Aggregator) Aggregate(results []Result) Aggregate {
	out := Aggregate{
		Summaries:    make([]string, 0, len(results)),
		Risks:        []string{},
		NextActions:  []string{},
		FilesChanged: []string{},
	}
	for _, result := range results {
		switch result.Status {
		case ResultCompleted:
			out.Completed++
		case ResultFailed:
			out.Failed++
		case ResultSkipped:
			out.Skipped++
		}
		if result.Summary != "" {
			out.Summaries = append(out.Summaries, result.Summary)
		}
		for _, risk := range result.Risks {
			if !slices.Contains(out.Risks, risk) {
				out.Risks = append(out.Risks, risk)
			}
		}
		for _, next := range result.NextActions {
			if !slices.Contains(out.NextActions, next) {
				out.NextActions = append(out.NextActions, next)
			}
		}
		for _, file := range result.FilesChanged {
			if !slices.Contains(out.FilesChanged, file) {
				out.FilesChanged = append(out.FilesChanged, file)
			}
		}
	}
	return out
}
