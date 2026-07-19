package telemetry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/rewindbpf/rewind/internal/event"
)

// JournalEvent adds a sequence number and a hash-chain link to the compact
// event contract. The kernel stays unaware of these fields; userspace adds
// them immediately before persistence.
type JournalEvent struct {
	event.Event
	Sequence     uint64 `json:"sequence"`
	PreviousHash string `json:"previous_hash,omitempty"`
	Hash         string `json:"hash"`
}

type Chain struct {
	next     uint64
	previous string
}

// Append returns a journal event whose hash commits to the event payload,
// sequence, and previous hash. JSON field order is stable for this struct,
// making the result reproducible for verification.
func (c *Chain) Append(value event.Event) (JournalEvent, error) {
	if c == nil {
		return JournalEvent{}, fmt.Errorf("append journal event: nil chain")
	}
	if err := value.Validate(); err != nil {
		return JournalEvent{}, err
	}
	sequence := c.next + 1
	candidate := JournalEvent{Event: value, Sequence: sequence, PreviousHash: c.previous}
	encoded, err := json.Marshal(candidate)
	if err != nil {
		return JournalEvent{}, err
	}
	digest := sha256.Sum256(encoded)
	candidate.Hash = hex.EncodeToString(digest[:])
	c.next = sequence
	c.previous = candidate.Hash
	return candidate, nil
}

// Verify checks sequence continuity and each event hash in a journal. It is
// intentionally independent of files so callers can stream decoded records.
func Verify(events []JournalEvent) bool {
	var chain Chain
	for _, value := range events {
		if value.Sequence != chain.next+1 || value.PreviousHash != chain.previous {
			return false
		}
		hash := value.Hash
		value.Hash = ""
		encoded, err := json.Marshal(value)
		if err != nil {
			return false
		}
		digest := sha256.Sum256(encoded)
		if hex.EncodeToString(digest[:]) != hash {
			return false
		}
		chain.next = value.Sequence
		chain.previous = hash
	}
	return true
}
