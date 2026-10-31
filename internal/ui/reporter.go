// Package ui provides the Reporter abstraction plus the Bubble Tea and
// plain-text implementations, styles, and table rendering. Business logic
// depends only on the Reporter interface.
package ui

import "time"

// Reporter receives progress events from the installer and other long
// operations. Implementations must be safe for use from a single goroutine.
type Reporter interface {
	// AppStart begins a group of steps for one app (e.g. "wgo 1.2.0").
	AppStart(app, detail string)
	// StepStart begins a named step for an app.
	StepStart(app, step string)
	// StepProgress updates a running step with byte progress; total may be
	// -1 when unknown.
	StepProgress(app, step string, done, total int64)
	// StepDone marks a step successful with its duration.
	StepDone(app, step string, d time.Duration)
	// StepFail marks a step failed.
	StepFail(app, step string, err error)
	// Info prints an informational line outside the step list.
	Info(msg string)
	// Warn prints a warning line.
	Warn(msg string)
	// Close flushes and tears down the reporter.
	Close()
}
