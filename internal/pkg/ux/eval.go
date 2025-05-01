package ux

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/jedib0t/go-pretty/v6/progress"
)

type UxEvalT struct {
	mux      sync.Mutex
	Rules    uint32
	Problems uint32
	Lines    atomic.Int64
	Bytes    progress.Tracker
	done     chan struct{}
}

func NewUxEval() *UxEvalT {
	return &UxEvalT{
		done: make(chan struct{}),
	}
}

func (u *UxEvalT) StartRuleTracker() {
}

func (u *UxEvalT) StartProblemsTracker() {
}

func (u *UxEvalT) IncrementRuleTracker(c int64) {
	u.mux.Lock()
	defer u.mux.Unlock()
	u.Rules++
}

func (u *UxEvalT) IncrementProblemsTracker(c int64) {
	u.mux.Lock()
	defer u.mux.Unlock()
	u.Problems++
}

func (u *UxEvalT) IncrementLinesTracker(c int64) {
}

func (u *UxEvalT) MarkRuleTrackerDone() {
}

func (u *UxEvalT) MarkProblemsTrackerDone() {
}

func (u *UxEvalT) MarkLinesTrackerDone() {
}

func (u *UxEvalT) StartLinesTracker(lines *atomic.Int64, killCh chan struct{}) {
	go func() {

	LOOP:
		for {
			select {
			case <-killCh:
				break LOOP
			}
		}

		u.Lines.Store(lines.Load())

		close(u.done)
	}()
}

func (u *UxEvalT) NewBytesTracker(src string) (*progress.Tracker, error) {
	u.Bytes = newBytesTracker(src)
	return &u.Bytes, nil
}

func (u *UxEvalT) MarkBytesTrackerDone() {
}

func (u *UxEvalT) FinalStats() (StatsT, error) {

	timeout := time.NewTimer(10 * time.Second)
	defer timeout.Stop()

LOOP:
	for {
		select {
		case <-timeout.C:
			break LOOP
		case <-u.done:
			break LOOP
		}
	}

	return StatsT{
		"rules":    u.Rules,
		"problems": u.Problems,
		"lines":    u.Lines.Load(),
		"bytes":    u.Bytes.Value(),
	}, nil
}
