package multiagent

// TaskMode describes whether a plan should run sequentially or in parallel.
type TaskMode string

const (
	TaskModeSequential TaskMode = "sequential"
	TaskModeParallel   TaskMode = "parallel"
)

// Task is one delegated subtask inside a multi-agent plan.
type Task struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Goal            string   `json:"goal"`
	Context         string   `json:"context,omitempty"`
	HistoryWindow   int      `json:"history_window,omitempty"`
	AllowedTools    []string `json:"allowed_tools,omitempty"`
	AllowedPrefixes []string `json:"allowed_prefixes,omitempty"`
	WriteScopes     []string `json:"write_scopes,omitempty"`
}

// Plan is the validated execution plan built for a multi-agent objective.
type Plan struct {
	Objective       string   `json:"objective"`
	Mode            TaskMode `json:"mode"`
	MaxConcurrent   int      `json:"max_concurrent"`
	ParentSessionID int64    `json:"parent_session_id,omitempty"`
	Tasks           []Task   `json:"tasks"`
}

// ResultStatus describes the terminal state of one delegated task.
type ResultStatus string

const (
	ResultCompleted ResultStatus = "completed"
	ResultFailed    ResultStatus = "failed"
	ResultSkipped   ResultStatus = "skipped"
)

// Result is the final output of one delegated task run.
type Result struct {
	TaskID         string       `json:"task_id"`
	ChildSessionID int64        `json:"child_session_id,omitempty"`
	Status         ResultStatus `json:"status"`
	Summary        string       `json:"summary"`
	Trace          []TraceStep  `json:"trace,omitempty"`
	FilesChanged   []string     `json:"files_changed,omitempty"`
	Risks          []string     `json:"risks,omitempty"`
	NextActions    []string     `json:"next_actions,omitempty"`
}

// TraceStep is one structured step in a child-agent trajectory.
type TraceStep struct {
	Iteration         int            `json:"iteration"`
	Type              string         `json:"type"`
	Tool              string         `json:"tool,omitempty"`
	Input             map[string]any `json:"input,omitempty"`
	Output            map[string]any `json:"output,omitempty"`
	Snapshot          map[string]any `json:"snapshot,omitempty"`
	Verified          bool           `json:"verified,omitempty"`
	Verifier          string         `json:"verifier,omitempty"`
	VerificationClass string         `json:"verification_class,omitempty"`
	Error             string         `json:"error,omitempty"`
	Note              string         `json:"note,omitempty"`
}

// Aggregate summarizes the outcome of an entire multi-agent plan.
type Aggregate struct {
	ParentSessionID int64    `json:"parent_session_id,omitempty"`
	Completed       int      `json:"completed"`
	Failed          int      `json:"failed"`
	Skipped         int      `json:"skipped"`
	Summaries       []string `json:"summaries"`
	Risks           []string `json:"risks"`
	NextActions     []string `json:"next_actions"`
	FilesChanged    []string `json:"files_changed"`
}
