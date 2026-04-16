package execution

import "fmt"

// Policy describes whether the high-risk execution surface is enabled.
type Policy struct {
	Enabled bool
	Reason  string
}

// DefaultPolicy returns the default disabled execution policy.
func DefaultPolicy() Policy {
	return Policy{
		Enabled: false,
		Reason:  "high-risk dynamic execution is disabled by default pending sandboxing and manual review",
	}
}

// Check returns an error when execution is disabled.
func (p Policy) Check() error {
	if p.Enabled {
		return nil
	}
	return fmt.Errorf("%s", p.Reason)
}
