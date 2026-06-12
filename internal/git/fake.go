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

	// Stage / Unstage / StageAllTracked. Tests can set Err fields
	// to simulate failure, and inspect Calls for assertion.
	StageErr             error
	StageCalls           [][]string
	UnstageErr           error
	UnstageCalls         [][]string
	StageAllTrackedErr   error
	StageAllTrackedCalls int

	CreateBranchErr   error
	CreateBranchCalls []FakeCreateBranchCall
	DeleteBranchErr   error
	DeleteBranchCalls []FakeDeleteBranchCall

	FetchPruneErr   error
	FetchPruneCalls []FakeFetchCall
	FetchAllErr     error
	FetchAllCalls   int
	FetchAllPrune   bool

	RemoteAddErr      error
	RemoteAddCalls    []FakeRemoteAddCall
	RemoteRemoveErr   error
	RemoteRemoveCalls []FakeRemoteRemoveCall
	RemoteRenameErr   error
	RemoteRenameCalls []FakeRemoteRenameCall
	RemoteSetURLErr   error
	RemoteSetURLCalls []FakeRemoteSetURLCall
	RemotePruneErr    error
	RemotePruneCalls  []FakeFetchCall
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

// FakeCreateBranchCall records arguments to a CreateBranch call.
type FakeCreateBranchCall struct {
	Name       string
	StartPoint string
}

// FakeDeleteBranchCall records arguments to a DeleteBranch call.
type FakeDeleteBranchCall struct {
	Name  string
	Force bool
}

// FakeRemoteAddCall records arguments to a RemoteAdd call.
type FakeRemoteAddCall struct {
	Name string
	URL  string
}

// FakeRemoteRemoveCall records arguments to a RemoteRemove call.
type FakeRemoteRemoveCall struct {
	Name  string
	Force bool
}

// FakeRemoteRenameCall records arguments to a RemoteRename call.
type FakeRemoteRenameCall struct {
	OldName string
	NewName string
}

// FakeRemoteSetURLCall records arguments to a RemoteSetURL call.
type FakeRemoteSetURLCall struct {
	Name    string
	URL     string
	PushURL bool
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

func (f *FakeAdapter) Stage(_ context.Context, paths []string) error {
	// Copy to avoid test-mutation surprises.
	cp := append([]string{}, paths...)
	f.StageCalls = append(f.StageCalls, cp)
	return f.StageErr
}

func (f *FakeAdapter) Unstage(_ context.Context, paths []string) error {
	cp := append([]string{}, paths...)
	f.UnstageCalls = append(f.UnstageCalls, cp)
	return f.UnstageErr
}

func (f *FakeAdapter) StageAllTracked(_ context.Context) error {
	f.StageAllTrackedCalls++
	return f.StageAllTrackedErr
}

func (f *FakeAdapter) CreateBranch(_ context.Context, name, startPoint string) error {
	f.CreateBranchCalls = append(f.CreateBranchCalls, FakeCreateBranchCall{Name: name, StartPoint: startPoint})
	return f.CreateBranchErr
}

func (f *FakeAdapter) DeleteBranch(_ context.Context, name string, force bool) error {
	f.DeleteBranchCalls = append(f.DeleteBranchCalls, FakeDeleteBranchCall{Name: name, Force: force})
	return f.DeleteBranchErr
}

func (f *FakeAdapter) FetchPrune(_ context.Context, remote string) error {
	f.FetchPruneCalls = append(f.FetchPruneCalls, FakeFetchCall{Remote: remote})
	return f.FetchPruneErr
}

func (f *FakeAdapter) FetchAll(_ context.Context, prune bool) error {
	f.FetchAllCalls++
	f.FetchAllPrune = prune
	return f.FetchAllErr
}

func (f *FakeAdapter) RemoteAdd(_ context.Context, name, url string) error {
	f.RemoteAddCalls = append(f.RemoteAddCalls, FakeRemoteAddCall{Name: name, URL: url})
	return f.RemoteAddErr
}

func (f *FakeAdapter) RemoteRemove(_ context.Context, name string, force bool) error {
	f.RemoteRemoveCalls = append(f.RemoteRemoveCalls, FakeRemoteRemoveCall{Name: name, Force: force})
	return f.RemoteRemoveErr
}

func (f *FakeAdapter) RemoteRename(_ context.Context, oldName, newName string) error {
	f.RemoteRenameCalls = append(f.RemoteRenameCalls, FakeRemoteRenameCall{OldName: oldName, NewName: newName})
	return f.RemoteRenameErr
}

func (f *FakeAdapter) RemoteSetURL(_ context.Context, name, url string, pushURL bool) error {
	f.RemoteSetURLCalls = append(f.RemoteSetURLCalls, FakeRemoteSetURLCall{Name: name, URL: url, PushURL: pushURL})
	return f.RemoteSetURLErr
}

func (f *FakeAdapter) RemotePrune(_ context.Context, name string) error {
	f.RemotePruneCalls = append(f.RemotePruneCalls, FakeFetchCall{Remote: name})
	return f.RemotePruneErr
}
