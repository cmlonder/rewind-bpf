// Package supervisor defines the authenticated local control-plane boundary.
// The runtime still owns all filesystem and kernel effects; this package only
// validates transport-level intents and serializes their responses.
package supervisor

import (
	"fmt"
	"time"
)

type CredentialLeaseRequest struct {
	Ref    string   `json:"ref"`
	Scopes []string `json:"scopes,omitempty"`
}

type HistoryPruneRequest struct {
	Keep int `json:"keep"`
}

type SessionRequest struct {
	Action string `json:"action"`
	RunID  string `json:"run_id"`
	Owner  string `json:"owner"`
	TTL    int    `json:"ttl_seconds,omitempty"`
}

type Request struct {
	Action       string `json:"action"`
	RunID        string `json:"run_id,omitempty"`
	Workspace    string `json:"workspace,omitempty"`
	Policy       string `json:"policy,omitempty"`
	Confirmation string `json:"confirmation,omitempty"`
	ActionToken  string `json:"action_token,omitempty"`
}

type ActionChallengeRequest struct {
	Action string `json:"action"`
	RunID  string `json:"run_id,omitempty"`
}

type ActionChallengeResponse struct {
	Token     string    `json:"token"`
	Action    string    `json:"action"`
	RunID     string    `json:"run_id,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
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
