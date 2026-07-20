package platform

import (
	"context"
	"fmt"
)

// Backend is the platform-neutral transaction contract consumed by future
// native adapters. Implementations must refuse operations they cannot prove.
type Backend interface {
	Prepare(context.Context, string) (Transaction, error)
	Capabilities() Capability
}

type Transaction interface {
	View() string
	Diff(context.Context) (any, error)
	Discard(context.Context) error
	Accept(context.Context, string) error
	Recover(context.Context) error
}

type UnavailableBackend struct{ Report Capability }

func (b UnavailableBackend) Prepare(context.Context, string) (Transaction, error) {
	return nil, fmt.Errorf("%s backend unavailable: %v", b.Report.Platform, b.Report.Reasons)
}

func (b UnavailableBackend) Capabilities() Capability { return b.Report }
