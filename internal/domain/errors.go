// Package domain holds the entities, enums, and errors that describe the job
// tracking model. It depends on nothing outside the standard library plus
// google/uuid, so every other layer can import it freely.
package domain

import (
	"errors"
	"fmt"
	"strings"
)

// Sentinel errors. Every layer wraps these rather than defining its own, so a
// single mapping table in each transport translates them to HTTP statuses or
// MCP tool errors.
var (
	ErrNotFound         = errors.New("not found")
	ErrInvalidArgument  = errors.New("invalid argument")
	ErrConflict         = errors.New("conflict")
	ErrUnauthenticated  = errors.New("unauthenticated")
	ErrPermissionDenied = errors.New("permission denied")
	ErrRateLimited      = errors.New("rate limited")
	// ErrUnavailable means a dependency the request needs did not answer —
	// the identity provider during authentication, say. The request may
	// succeed unchanged if retried; the caller's input is not at fault.
	ErrUnavailable = errors.New("unavailable")
)

// ValidationError collects field-level problems so a caller learns everything
// wrong with a payload in one round trip instead of one issue at a time.
//
// It unwraps to ErrInvalidArgument.
type ValidationError struct {
	problems []string
}

// NewValidationError returns an error carrying the given problems, or nil when
// there are none. Callers can therefore always `return err` unconditionally.
func NewValidationError(problems ...string) error {
	if len(problems) == 0 {
		return nil
	}
	return &ValidationError{problems: problems}
}

// Add records a formatted problem.
func (e *ValidationError) Add(format string, args ...any) {
	e.problems = append(e.problems, fmt.Sprintf(format, args...))
}

// Addf is an alias for Add, provided for call sites that read better with the
// f suffix.
func (e *ValidationError) Addf(format string, args ...any) { e.Add(format, args...) }

// OrNil returns e when it holds problems and nil otherwise, letting callers
// accumulate into a zero-valued ValidationError and return the result directly.
func (e *ValidationError) OrNil() error {
	if e == nil || len(e.problems) == 0 {
		return nil
	}
	return e
}

// Details returns the individual problems.
func (e *ValidationError) Details() []string { return e.problems }

func (e *ValidationError) Error() string {
	if len(e.problems) == 1 {
		return "invalid argument: " + e.problems[0]
	}
	return fmt.Sprintf("invalid argument: %d problems: %s", len(e.problems), strings.Join(e.problems, "; "))
}

// Unwrap ties ValidationError to the ErrInvalidArgument sentinel.
func (e *ValidationError) Unwrap() error { return ErrInvalidArgument }

// PermissionError reports that a principal's scopes did not cover what it
// tried to do.
//
// Error() is deliberately generic: every 4xx error's message is returned to
// the caller verbatim (REST) or into the model's context (MCP), and this one
// used to embed the key's label and its full scope list — handing a 403 body
// back to whoever made the request. The detail is still available on the
// struct for anything that wants to log it structurally, just not in the
// string that reaches the caller.
type PermissionError struct {
	KeyLabel      string
	HeldScopes    []Scope
	RequiredScope Scope
}

// NewPermissionError returns an error reporting that a key with heldScopes
// tried to use requiredScope.
func NewPermissionError(keyLabel string, heldScopes []Scope, requiredScope Scope) error {
	return &PermissionError{KeyLabel: keyLabel, HeldScopes: heldScopes, RequiredScope: requiredScope}
}

func (e *PermissionError) Error() string {
	return fmt.Sprintf("this credential does not hold the %q scope", e.RequiredScope)
}

// Unwrap ties PermissionError to the ErrPermissionDenied sentinel.
func (e *PermissionError) Unwrap() error { return ErrPermissionDenied }

// NotFoundError names the entity that was missing.
type NotFoundError struct {
	Kind string
	ID   string
}

// NewNotFoundError returns an error reporting that kind/id does not exist.
func NewNotFoundError(kind, id string) error { return &NotFoundError{Kind: kind, ID: id} }

func (e *NotFoundError) Error() string { return fmt.Sprintf("%s %q not found", e.Kind, e.ID) }

// Unwrap ties NotFoundError to the ErrNotFound sentinel.
func (e *NotFoundError) Unwrap() error { return ErrNotFound }
