package execution

import "fmt"

type Policy struct {
	Enabled bool
	Reason  string
}

func DefaultPolicy() Policy {
	return Policy{
		Enabled: false,
		Reason:  "high-risk dynamic execution is disabled by default pending sandboxing and manual review",
	}
}

func (p Policy) Check() error {
	if p.Enabled {
		return nil
	}
	return fmt.Errorf("%s", p.Reason)
}
