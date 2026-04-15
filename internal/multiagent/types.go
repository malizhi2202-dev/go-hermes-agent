package multiagent

type TaskMode string

const (
	TaskModeSequential TaskMode = "sequential"
	TaskModeParallel   TaskMode = "parallel"
)

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

type Plan struct {
	Objective       string   `json:"objective"`
	Mode            TaskMode `json:"mode"`
	MaxConcurrent   int      `json:"max_concurrent"`
	ParentSessionID int64    `json:"parent_session_id,omitempty"`
	Tasks           []Task   `json:"tasks"`
}

type ResultStatus string

const (
	ResultCompleted ResultStatus = "completed"
	ResultFailed    ResultStatus = "failed"
	ResultSkipped   ResultStatus = "skipped"
)

type Result struct {
	TaskID         string       `json:"task_id"`
	ChildSessionID int64        `json:"child_session_id,omitempty"`
	Status         ResultStatus `json:"status"`
	Summary        string       `json:"summary"`
	FilesChanged   []string     `json:"files_changed,omitempty"`
	Risks          []string     `json:"risks,omitempty"`
	NextActions    []string     `json:"next_actions,omitempty"`
}

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
