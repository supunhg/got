package git

import (
	"context"
	"io"
)

// FakeAdapter is an in-memory Adapter used by unit tests. Each method
// returns the value of the corresponding *Val field, or the *Err field
// if set. Each call increments a counter or appends to a slice for
// assertion in tests.
type FakeAdapter struct {
	StatusVal   Status
	StatusErr   error
	StatusCalls int

	CommitVal   SHA
	CommitErr   error
	CommitCalls []FakeCommitCall

	BranchesVal   []Branch
	BranchesErr   error
	BranchesCalls int

	RemoteBranchesVal   []Branch
	RemoteBranchesErr   error
	RemoteBranchesCalls int

	RemotesVal   []Remote
	RemotesErr   error
	RemotesCalls int

	LogVal   io.Reader
	LogErr   error
	LogCalls []FakeLogCall

	CurrentRefVal   string
	CurrentRefErr   error
	CurrentRefCalls int

	CheckoutErr   error
	CheckoutCalls []FakeCheckoutCall
	MergeErr      error
	MergeCalls    []FakeMergeCall
	ResetErr      error
	ResetCalls    []FakeResetCall
	FetchErr      error
	FetchCalls    []FakeFetchCall
	PushErr       error
	PushCalls     []FakePushCall
}

// FakeCommitCall records arguments to a Commit call.
type FakeCommitCall struct {
	Msg  string
	Opts CommitOpts
}

// FakeLogCall records arguments to a Log call.
type FakeLogCall struct {
	RangeStr string
	Format   LogFormat
}

// FakeCheckoutCall records arguments to a Checkout call.
type FakeCheckoutCall struct {
	Ref  string
	Opts CheckoutOpts
}

// FakeMergeCall records arguments to a Merge call.
type FakeMergeCall struct {
	Ref  string
	Opts MergeOpts
}

// FakeResetCall records arguments to a Reset call.
type FakeResetCall struct {
	Target string
	Mode   ResetMode
}

// FakeFetchCall records arguments to a Fetch call.
type FakeFetchCall struct {
	Remote string
}

// FakePushCall records arguments to a Push call.
type FakePushCall struct {
	Remote string
	Branch string
	Opts   PushOpts
}

// NewFake returns a FakeAdapter with safe defaults: an empty Status,
// and empty Branches/RemoteBranches/Remotes slices.
func NewFake() *FakeAdapter {
	return &FakeAdapter{
		StatusVal:         Status{Entries: []StatusEntry{}},
		BranchesVal:       []Branch{},
		RemoteBranchesVal: []Branch{},
		RemotesVal:        []Remote{},
	}
}

// Compile-time check that FakeAdapter satisfies Adapter.
var _ Adapter = (*FakeAdapter)(nil)

func (f *FakeAdapter) Status(_ context.Context) (Status, error) {
	f.StatusCalls++
	return f.StatusVal, f.StatusErr
}

func (f *FakeAdapter) Commit(_ context.Context, msg string, opts CommitOpts) (SHA, error) {
	f.CommitCalls = append(f.CommitCalls, FakeCommitCall{Msg: msg, Opts: opts})
	return f.CommitVal, f.CommitErr
}

func (f *FakeAdapter) Branches(_ context.Context) ([]Branch, error) {
	f.BranchesCalls++
	return f.BranchesVal, f.BranchesErr
}

func (f *FakeAdapter) RemoteBranches(_ context.Context) ([]Branch, error) {
	f.RemoteBranchesCalls++
	return f.RemoteBranchesVal, f.RemoteBranchesErr
}

func (f *FakeAdapter) Remotes(_ context.Context) ([]Remote, error) {
	f.RemotesCalls++
	return f.RemotesVal, f.RemotesErr
}

func (f *FakeAdapter) Checkout(_ context.Context, ref string, opts CheckoutOpts) error {
	f.CheckoutCalls = append(f.CheckoutCalls, FakeCheckoutCall{Ref: ref, Opts: opts})
	return f.CheckoutErr
}

func (f *FakeAdapter) Merge(_ context.Context, ref string, opts MergeOpts) error {
	f.MergeCalls = append(f.MergeCalls, FakeMergeCall{Ref: ref, Opts: opts})
	return f.MergeErr
}

func (f *FakeAdapter) Reset(_ context.Context, target string, mode ResetMode) error {
	f.ResetCalls = append(f.ResetCalls, FakeResetCall{Target: target, Mode: mode})
	return f.ResetErr
}

func (f *FakeAdapter) Fetch(_ context.Context, remote string) error {
	f.FetchCalls = append(f.FetchCalls, FakeFetchCall{Remote: remote})
	return f.FetchErr
}

func (f *FakeAdapter) Push(_ context.Context, remote, branch string, opts PushOpts) error {
	f.PushCalls = append(f.PushCalls, FakePushCall{Remote: remote, Branch: branch, Opts: opts})
	return f.PushErr
}

func (f *FakeAdapter) Log(_ context.Context, rangeStr string, format LogFormat) (io.Reader, error) {
	f.LogCalls = append(f.LogCalls, FakeLogCall{RangeStr: rangeStr, Format: format})
	if f.LogVal == nil {
		return nil, f.LogErr
	}
	return f.LogVal, f.LogErr
}

func (f *FakeAdapter) CurrentRef(_ context.Context) (string, error) {
	f.CurrentRefCalls++
	return f.CurrentRefVal, f.CurrentRefErr
}
