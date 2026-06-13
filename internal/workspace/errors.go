package workspace

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"time"
)

// ErrNotFound is returned by Store.Get, Store.Delete, and other
// lookups when no workspace matches the given idOrName. Callers
// can use errors.Is(err, workspace.ErrNotFound) for a portable
// check; the CLI maps it to a "no such workspace" message and
// exit code 5 (CodeValidation).
var ErrNotFound = errors.New("workspace: not found")

// ErrInvalidName is returned by Store.Create and Store.Update when
// the supplied name doesn't match ValidName. The Name field is
// included in the error so the CLI can render a clean validation
// message and so errors.Is / errors.As still work.
type ErrInvalidName struct{ Name string }

func (e *ErrInvalidName) Error() string {
	return fmt.Sprintf("workspace: invalid name %q (must match [a-z][a-z0-9_-]{0,62})", e.Name)
}

// ErrNameTaken is returned by Store.Create when the name already
// exists. Update handles the same case by overwriting the matching
// row, so this is a Create-only error. The CLI translates it to
// exit code 5 with a "pick a different name" hint.
var ErrNameTaken = errors.New("workspace: name already in use")

// ErrInvalidState is returned by Store.Update when a state value
// is not in the known set (see State.Valid).
type ErrInvalidState struct{ State State }

func (e *ErrInvalidState) Error() string {
	return fmt.Sprintf("workspace: invalid state %q (must be open or archived)", e.State)
}

// ErrInvalidDecisionStatus is returned by Decision.Update and
// Decision.Add when the supplied status is not in the known set
// (see DecisionStatus.Valid).
type ErrInvalidDecisionStatus struct{ Status DecisionStatus }

func (e *ErrInvalidDecisionStatus) Error() string {
	return fmt.Sprintf("workspace: invalid decision status %q", e.Status)
}

// ErrEmptyTitle is returned by Store.Create when the supplied title
// is empty. The slug (name) is the database key, but the title is
// what the user sees in lists and detail views, so we reject empty
// titles with a clear error rather than inserting a blank row.
var ErrEmptyTitle = errors.New("workspace: title is required")

// idRe matches the ID format we generate. We use this only for
// validation in Get's "try as ID first" path; non-matching strings
// fall through to the name lookup. The regex is intentionally
// lenient (10 hex chars of time, 8 hex chars of random) so a
// future bump to 16 random bytes doesn't break old code.
var idRe = regexp.MustCompile(`^[0-9a-f]{13}-[0-9a-f]{16,}$`)

// looksLikeID reports whether s matches the shape of an ID we
// generate. Used by Get / Delete / Show to disambiguate "did the
// user pass an ID or a name?" without a round trip to the DB.
func looksLikeID(s string) bool {
	return idRe.MatchString(s)
}

// newID returns a time-sortable, random-suffixed identifier. The
// format is "<13 hex chars of unix-ms>-<16 hex chars of crypto/rand>",
// which gives a 30-char ID that is lexicographically sortable by
// creation time and unique enough for any single repo. We do not
// use github.com/oklog/ulid to keep the dependency surface small;
// the spec calls these "ULID-like" and the format is good enough
// for our needs (8 bytes of randomness is 2^64 per millisecond).
func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%013x-%x", time.Now().UTC().UnixMilli(), hex.EncodeToString(b[:]))
}
