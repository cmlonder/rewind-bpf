package supervisor

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

const actionChallengeTTL = 2 * time.Minute

type actionChallenge struct {
	action    string
	runID     string
	expiresAt time.Time
}

type actionChallengeStore struct {
	mu      sync.Mutex
	entries map[string]actionChallenge
}

func (s *actionChallengeStore) issue(action, runID string, now time.Time) (ActionChallengeResponse, error) {
	if s == nil {
		return ActionChallengeResponse{}, fmt.Errorf("action challenge store is unavailable")
	}
	if action != "rollback" && action != "recover" && action != "commit" {
		return ActionChallengeResponse{}, fmt.Errorf("action %q does not require a challenge", action)
	}
	bytes := make([]byte, 24)
	if _, err := rand.Read(bytes); err != nil {
		return ActionChallengeResponse{}, fmt.Errorf("generate action challenge: %w", err)
	}
	token := hex.EncodeToString(bytes)
	expiresAt := now.Add(actionChallengeTTL)
	s.mu.Lock()
	if s.entries == nil {
		s.entries = make(map[string]actionChallenge)
	}
	s.entries[token] = actionChallenge{action: action, runID: runID, expiresAt: expiresAt}
	s.mu.Unlock()
	return ActionChallengeResponse{Token: token, Action: action, RunID: runID, ExpiresAt: expiresAt}, nil
}

func (s *actionChallengeStore) consume(token, action, runID string, now time.Time) error {
	if s == nil || token == "" {
		return fmt.Errorf("action challenge is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	challenge, ok := s.entries[token]
	if !ok {
		return fmt.Errorf("action challenge is invalid or already used")
	}
	delete(s.entries, token)
	if !now.Before(challenge.expiresAt) {
		return fmt.Errorf("action challenge expired")
	}
	if challenge.action != action || challenge.runID != runID {
		return fmt.Errorf("action challenge does not match request")
	}
	return nil
}
