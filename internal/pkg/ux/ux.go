package ux

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/Masterminds/semver"
	"github.com/fatih/color"
	"github.com/jedib0t/go-pretty/v6/progress"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/prequel-dev/detection-engine/internal/pkg/verz"
)

var (
	ErrNotImplemented = errors.New("not implemented")
)

const (
	AppName             = "prequel"
	AppDesc             = "Prequel is the open and community-driven problem detector for Common Reliability Enumerations (CREs)."
	ErrorCategoryRules  = "Rules"
	ErrorCategoryData   = "Data"
	ErrorCategoryConfig = "Config"
	ErrorCategoryAuth   = "Auth"
)

const (
	versionTmpl        = "%s %s %s %s/%s %s\n%s\n\n"
	rulesVersionTmpl   = "Current rules release: %s %s"
	lineRefer          = "Learn more at https://github.com/prequel-dev/prequel"
	lineCopyright      = "Copyright 2025 Prequel Software, Inc. (https://prequel.dev)"
	emailVerifyTitle   = "\nYou're one step away! Please verify your email\n"
	emailVerifyBodyFmt = "It looks like your email (%s) has not been verified yet. Check your inbox for a verification link from "
	emailVerifyFooter  = " and click it to activate your account. If you do not see the email, check your spam folder.\n\nSee https://docs.prequel.dev/updates for more information.\n\n"
	emailVerifyFrom    = "updates@prequel.dev"
)

type UxFactoryI interface {
	NewBytesTracker(src string) (*progress.Tracker, error)
	StartRuleTracker()
	StartProblemsTracker()
	StartLinesTracker(lines *atomic.Int64, killCh chan struct{})
	IncrementRuleTracker(c int64)
	IncrementProblemsTracker(c int64)
	IncrementLinesTracker(c int64)
	MarkRuleTrackerDone()
	MarkProblemsTrackerDone()
	MarkLinesTrackerDone()
	FinalStats() (map[string]any, error)
}

func PrintVersion(configDir, currRulesPath string, currRulesVer *semver.Version) {
	rulesOutput := fmt.Sprintf(rulesVersionTmpl, currRulesVer.String(), currRulesPath)
	fmt.Printf(versionTmpl, AppName, verz.Semver(), verz.Githash, runtime.GOOS, runtime.GOARCH, verz.Date, rulesOutput)
	fmt.Println(lineRefer)
	fmt.Println(lineCopyright)
}

func NewProgressWriter(nTrackers int) progress.Writer {
	pw := progress.NewWriter()
	pw.SetAutoStop(true)
	pw.SetMessageLength(24)
	pw.SetNumTrackersExpected(nTrackers)
	pw.SetSortBy(progress.SortByNone)
	pw.SetStyle(progress.StyleDefault)
	pw.SetTrackerLength(25)
	pw.SetTrackerPosition(progress.PositionRight)
	pw.SetUpdateFrequency(time.Millisecond * 100)
	pw.Style().Colors = progress.StyleColorsExample
	pw.Style().Options.PercentFormat = "%4.1f%%"
	pw.Style().Visibility.ETA = true
	pw.Style().Visibility.Percentage = true
	pw.Style().Visibility.Speed = true
	pw.Style().Visibility.Time = true
	return pw
}

func RootProgress(scrollbar bool) progress.Writer {

	pw := NewProgressWriter(3)

	colors := progress.StyleColors{
		Message: text.Colors{text.FgHiWhite},
		Pinned:  text.Colors{text.FgBlue, text.Bold},
		Stats:   text.Colors{text.FgHiBlue, text.Bold},
		Time:    text.Colors{text.FgHiMagenta, text.Bold},
	}
	pw.Style().Visibility.Percentage = false
	pw.Style().Options.Separator = ""
	pw.Style().Visibility.Tracker = scrollbar
	pw.Style().Options.TimeDonePrecision = time.Millisecond
	pw.Style().Visibility.Pinned = false
	pw.Style().Colors = colors
	pw.SetAutoStop(false)
	pw.SetOutputWriter(os.Stdout)
	pw.SetUpdateFrequency(time.Millisecond * 200)

	return pw
}

func NewRuleTracker() progress.Tracker {
	return progress.Tracker{
		Message:            "Parsing rules",
		RemoveOnCompletion: false,
		Total:              0,
		Units: progress.Units{
			Notation:         " rules",
			NotationPosition: progress.UnitsNotationPositionAfter,
			Formatter:        progress.FormatNumber,
		},
	}
}

func NewProblemsTracker() progress.Tracker {
	return progress.Tracker{
		Message:            "Problems detected",
		RemoveOnCompletion: false,
		Total:              0,
		Units:              progress.UnitsDefault,
	}
}

func newBytesTracker(src string) progress.Tracker {
	return progress.Tracker{
		Message:            fmt.Sprintf("Reading %s", src),
		RemoveOnCompletion: false,
		Total:              0,
		Units:              progress.UnitsBytes,
	}

}

func NewLineTracker() progress.Tracker {
	return progress.Tracker{
		Message:            "Matching lines",
		RemoveOnCompletion: false,
		Total:              0,
		Units: progress.Units{
			Notation:         " lines",
			NotationPosition: progress.UnitsNotationPositionAfter,
			Formatter:        progress.FormatNumber,
		},
	}
}

func NewDownloadTracker(totalSize int64) progress.Tracker {
	return progress.Tracker{
		Message: "Downloading update",
		Total:   totalSize,
		Units:   progress.UnitsBytes,
	}
}

func PrintEmailVerifyNotice(email string) {

	title := color.New(color.FgHiBlue).Add(color.Bold)
	title.Fprintf(os.Stderr, emailVerifyTitle)

	fmt.Fprintf(os.Stderr, emailVerifyBodyFmt, email)

	emailStr := color.New(color.FgHiWhite).Add(color.Underline)
	emailStr.Fprintf(os.Stderr, emailVerifyFrom)

	fmt.Fprint(os.Stderr, emailVerifyFooter)
}

func RulesError(err error) error {
	return CategoryError(ErrorCategoryRules, err)
}

func DataError(err error) error {
	return CategoryError(ErrorCategoryData, err)
}

func ConfigError(err error) error {
	return CategoryError(ErrorCategoryConfig, err)
}

func AuthError(err error) error {
	return CategoryError(ErrorCategoryAuth, err)
}

func CategoryError(category string, err error) error {
	fmt.Fprintf(os.Stderr, "%s error: %v\n", category, err)
	return err
}

func Error(err error) error {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	return err
}

func ErrorMsg(err error, msg string) error {
	fmt.Fprintf(os.Stderr, "%s\n", msg)
	return err
}
