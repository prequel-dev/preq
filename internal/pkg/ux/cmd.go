package ux

import (
	"sync/atomic"
	"time"

	"github.com/jedib0t/go-pretty/v6/progress"
)

type UxCmdT struct {
	Pw       progress.Writer
	Rules    progress.Tracker
	Problems progress.Tracker
	Lines    progress.Tracker
	Bytes    progress.Tracker
}

func NewUxCmd(pw progress.Writer) *UxCmdT {

	ux := &UxCmdT{
		Pw:       pw,
		Rules:    NewRuleTracker(),
		Problems: NewProblemsTracker(),
	}

	if ux.Pw != nil {
		ux.Pw.AppendTracker(&ux.Rules)
		ux.Pw.AppendTracker(&ux.Problems)
	}

	return ux
}

func (u *UxCmdT) StartRuleTracker() {
	u.Rules.Start()
}

func (u *UxCmdT) StartProblemsTracker() {
	u.Problems.Start()
}

func (u *UxCmdT) IncrementRuleTracker(c int64) {
	u.Rules.Increment(c)
}

func (u *UxCmdT) IncrementProblemsTracker(c int64) {
	u.Problems.Increment(c)
}

func (u *UxCmdT) IncrementLinesTracker(c int64) {
	u.Lines.Increment(c)
}

func (u *UxCmdT) MarkRuleTrackerDone() {
	u.Rules.MarkAsDone()
}

func (u *UxCmdT) MarkProblemsTrackerDone() {
	u.Problems.MarkAsDone()
}

func (u *UxCmdT) MarkLinesTrackerDone() {
	u.Lines.MarkAsDone()
}

func (u *UxCmdT) StartLinesTracker(lines *atomic.Int64, killCh chan struct{}) {

	u.Lines = NewLineTracker()
	if u.Pw != nil {
		u.Pw.AppendTracker(&u.Lines)
	}

	u.Lines.Start()

	go func() {
		defer u.Lines.MarkAsDone()
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()

	LOOP:
		for {
			select {
			case <-killCh:
				break LOOP
			case <-tick.C:
				u.Lines.SetValue(lines.Load())
			}
		}

		u.Lines.SetValue(lines.Load())
	}()
}

func (u *UxCmdT) NewBytesTracker(src string) (*progress.Tracker, error) {
	bt := newBytesTracker(src)
	if u.Pw != nil {
		u.Pw.AppendTracker(&bt)
	}
	bt.Start()
	return &bt, nil
}

func (u *UxCmdT) UpdateBytesTotal(n int64) {
}

func (u *UxCmdT) MarkBytesTrackerDone() {
	u.Bytes.MarkAsDone()
}

func (u *UxCmdT) FinalStats() (StatsT, error) {
	return nil, ErrNotImplemented
}
