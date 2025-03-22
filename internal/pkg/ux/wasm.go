package ux

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/jedib0t/go-pretty/v6/progress"
)

type UxWasmT struct {
	mux      sync.Mutex
	Rules    uint32
	Problems uint32
	Lines    atomic.Int64
	Bytes    progress.Tracker
	done     chan struct{}
}

func NewUxWasm() *UxWasmT {
	return &UxWasmT{
		done: make(chan struct{}),
	}
}

func (u *UxWasmT) StartRuleTracker() {
}

func (u *UxWasmT) StartProblemsTracker() {
}

func (u *UxWasmT) IncrementRuleTracker(c int64) {
	u.mux.Lock()
	defer u.mux.Unlock()
	u.Rules++
}

func (u *UxWasmT) IncrementProblemsTracker(c int64) {
	u.mux.Lock()
	defer u.mux.Unlock()
	u.Problems++
}

func (u *UxWasmT) IncrementLinesTracker(c int64) {
}

func (u *UxWasmT) MarkRuleTrackerDone() {
}

func (u *UxWasmT) MarkProblemsTrackerDone() {
}

func (u *UxWasmT) MarkLinesTrackerDone() {
}

func (u *UxWasmT) StartLinesTracker(lines *atomic.Int64, killCh chan struct{}) {
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

func (u *UxWasmT) NewBytesTracker(src string) (*progress.Tracker, error) {
	u.Bytes = newBytesTracker(src)
	return &u.Bytes, nil
}

func (u *UxWasmT) MarkBytesTrackerDone() {
}

func (u *UxWasmT) FinalStats() (map[string]any, error) {

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

	return map[string]any{
		"rules":    u.Rules,
		"problems": u.Problems,
		"lines":    u.Lines.Load(),
		"bytes":    u.Bytes.Value(),
	}, nil
}
