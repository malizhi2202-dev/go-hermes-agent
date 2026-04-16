package gateway

import "strings"

// parseMultiAgentCommand extracts a gateway multi-agent objective from a
// slash-style command.
func parseMultiAgentCommand(text string) (string, bool) {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "/multiagent") {
		return "", false
	}
	objective := strings.TrimSpace(strings.TrimPrefix(trimmed, "/multiagent"))
	if objective == "" {
		return "", false
	}
	return objective, true
}
