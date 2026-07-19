// Package supervisor defines the future local control-plane boundary. It is a
// data contract only: transport, authentication, and daemon lifetime remain
// intentionally out of the privileged runtime until post-demo hardening.
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
	return nil
}
