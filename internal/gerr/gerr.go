// Package gerr defines typed user-facing errors and an exit-code mapping
// for the got CLI. See got-spec.md §15 for the design.
package gerr

import (
	"errors"
	"fmt"
	"strings"
)

// Code is the exit code the CLI returns when an Error of this kind is
// the last error in the chain.
type Code int

const (
	CodeGeneric        Code = 1
	CodeUsage          Code = 2
	CodeNotInGitRepo   Code = 3
	CodeNotInitialized Code = 4
	CodeValidation     Code = 5
	CodePlugin         Code = 64
)

// Error is a typed user-facing error. It wraps an optional Cause and
// carries a Code plus an optional Hint shown to the user.
type Error struct {
	Code    Code
	Message string
	Hint    string
	Cause   error
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

// Unwrap returns the wrapped Cause so errors.Is / errors.As work.
func (e *Error) Unwrap() error { return e.Cause }

// UserMessage returns a friendly, jargon-free message suitable for
// printing to the user (no stack traces, no "Error:" prefix). If a Hint
// is set, it is appended on its own indented line.
func (e *Error) UserMessage() string {
	if e.Hint != "" {
		return e.Message + "\n  " + e.Hint
	}
	return e.Message
}

// New constructs a new Error with the given code and message.
func New(code Code, msg string) *Error {
	return &Error{Code: code, Message: msg}
}

// Wrap constructs a new Error that wraps cause.
func Wrap(code Code, cause error, msg string) *Error {
	return &Error{Code: code, Message: msg, Cause: cause}
}

// NotInGitRepo returns a friendly error for the case when the current
// directory is not inside a Git repository.
func NotInGitRepo(start string) *Error {
	return &Error{
		Code:    CodeNotInGitRepo,
		Message: fmt.Sprintf("not inside a Git repository (no .git found in %q or any parent)", start),
		Hint:    "To start a new repository:  git init && got init\n  To navigate to one:          cd <path>",
	}
}

// NotInitialized returns a friendly error for the case when a command
// requires .got/ but the user has not run `got init` yet.
func NotInitialized() *Error {
	return &Error{
		Code:    CodeNotInitialized,
		Message: "GOT is not initialized in this repository",
		Hint:    "Run `got init` to set up .got/.",
	}
}

// GitError wraps a non-zero exit from `git`.
func GitError(cause error, args ...string) *Error {
	return &Error{
		Code:    CodeGeneric,
		Message: fmt.Sprintf("git %s failed", strings.Join(args, " ")),
		Cause:   cause,
	}
}

// Validation returns a user-input validation error.
func Validation(msg string) *Error {
	return &Error{Code: CodeValidation, Message: msg}
}

// Usage returns a CLI usage error.
func Usage(msg string) *Error {
	return &Error{Code: CodeUsage, Message: msg}
}

// ExitCode returns the exit code for err, or CodeGeneric if err is not
// an *Error (including nil).
func ExitCode(err error) int {
	var e *Error
	if errors.As(err, &e) {
		return int(e.Code)
	}
	return int(CodeGeneric)
}

// UserMessage returns the friendly message for err, or err.Error() if
// err is not an *Error.
func UserMessage(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.UserMessage()
	}
	return err.Error()
}
