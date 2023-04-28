package services

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/a-h/templ/examples/lambda-deployment/db"
	"golang.org/x/exp/slog"
)

type Counts struct {
	Global  int
	Session int
}

type IncrementType int

const (
	IncrementTypeUnknown IncrementType = iota
	IncrementTypeGlobal
	IncrementTypeSession
)

var ErrUnknownIncrementType error = errors.New("unkown increment type")

type Count interface {
	Increment(ctx context.Context, it IncrementType, sessionID string) (counts Counts, err error)
	Get(ctx context.Context, sessionID string) (counts Counts, err error)
}

func NewCount(log *slog.Logger, cs *db.CountStore) Count {
	return countService{
		Log:        log,
		CountStore: cs,
	}
}

type countService struct {
	Log        *slog.Logger
	CountStore *db.CountStore
}

func (cs countService) Increment(ctx context.Context, it IncrementType, sessionID string) (counts Counts, err error) {
	// Work out which operations to do.
	var global, session func(ctx context.Context, id string) (count int, err error)
	switch it {
	case IncrementTypeGlobal:
		global = cs.CountStore.Increment
		session = cs.CountStore.Get
	case IncrementTypeSession:
		global = cs.CountStore.Get
		session = cs.CountStore.Increment
	default:
		return counts, ErrUnknownIncrementType
	}

	// Run the operations in parallel.
	var wg sync.WaitGroup
	wg.Add(2)
	errs := make([]error, 2)
	go func() {
		defer wg.Done()
		counts.Global, errs[0] = global(ctx, "global")
	}()
	go func() {
		defer wg.Done()
		counts.Session, errs[1] = session(ctx, sessionID)
	}()
	wg.Wait()

	return counts, errors.Join(errs...)
}

func (cs countService) Get(ctx context.Context, sessionID string) (counts Counts, err error) {
	globalAndSessionCounts, err := cs.CountStore.BatchGet(ctx, "global", sessionID)
	if err != nil {
		err = fmt.Errorf("countservice: failed to get counts: %w", err)
		return
	}
	if len(globalAndSessionCounts) != 2 {
		err = fmt.Errorf("countservice: unexpected counts returned, expected 2, got %d", len(globalAndSessionCounts))
	}
	counts.Global = globalAndSessionCounts[0]
	counts.Session = globalAndSessionCounts[1]
	return
}
