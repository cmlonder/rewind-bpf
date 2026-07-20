// Package supervisor defines the authenticated local control-plane boundary.
// The runtime still owns all filesystem and kernel effects; this package only
// validates transport-level intents and serializes their responses.
package supervisor

import "fmt"

type Request struct {
	Action       string `json:"action"`
	RunID        string `json:"run_id,omitempty"`
	Workspace    string `json:"workspace,omitempty"`
	Policy       string `json:"policy,omitempty"`
	Confirmation string `json:"confirmation,omitempty"`
}

type Response struct {
	OK           bool     `json:"ok"`
	State        string   `json:"state,omitempty"`
	Message      string   `json:"message,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Warnings     []string `json:"warnings,omitempty"`
}

func Validate(request Request) error {
	if request.Action == "" {
		return fmt.Errorf("supervisor action is required")
	}
	switch request.Action {
	case "status", "rollback", "recover", "commit":
		return nil
	default:
		return fmt.Errorf("unsupported supervisor action %q", request.Action)
	}
}
