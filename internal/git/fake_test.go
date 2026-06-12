package git

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestFakeAdapter_Status(t *testing.T) {
	f := NewFake()
	f.StatusVal = Status{Branch: "main", Ahead: 2, Entries: []StatusEntry{{Path: "foo", XY: " M", IsUnstaged: true}}}
	got, err := f.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Branch != "main" || got.Ahead != 2 {
		t.Errorf("Status() = %+v", got)
	}
	if f.StatusCalls != 1 {
		t.Errorf("StatusCalls = %d, want 1", f.StatusCalls)
	}
}

func TestFakeAdapter_StatusErr(t *testing.T) {
	f := NewFake()
	boom := errors.New("boom")
	f.StatusErr = boom
	_, err := f.Status(context.Background())
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
}

func TestFakeAdapter_Branches(t *testing.T) {
	f := NewFake()
	f.BranchesVal = []Branch{
		{Name: "main", IsCurrent: true, SHA: "abc1234"},
		{Name: "feature", SHA: "def5678"},
	}
	got, err := f.Branches(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "main" {
		t.Errorf("Branches() = %+v", got)
	}
	if f.BranchesCalls != 1 {
		t.Errorf("BranchesCalls = %d, want 1", f.BranchesCalls)
	}
}

func TestFakeAdapter_CommitRecords(t *testing.T) {
	f := NewFake()
	f.CommitVal = "deadbeef"
	_, _ = f.Commit(context.Background(), "feat: foo", CommitOpts{Signoff: true})
	if len(f.CommitCalls) != 1 {
		t.Fatalf("CommitCalls len = %d, want 1", len(f.CommitCalls))
	}
	c := f.CommitCalls[0]
	if c.Msg != "feat: foo" || !c.Opts.Signoff {
		t.Errorf("recorded call = %+v", c)
	}
}

func TestFakeAdapter_Remotes(t *testing.T) {
	f := NewFake()
	f.RemotesVal = []Remote{{Name: "origin", FetchURL: "u", PushURL: "u"}}
	got, err := f.Remotes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "origin" {
		t.Errorf("Remotes() = %+v", got)
	}
}

func TestFakeAdapter_Log(t *testing.T) {
	f := NewFake()
	f.LogVal = strings.NewReader(`{"sha":"abc"}` + "\n")
	r, err := f.Log(context.Background(), "HEAD~5..HEAD", LogFormatNDJSON)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 64)
	n, _ := r.Read(buf)
	if !strings.Contains(string(buf[:n]), `"sha":"abc"`) {
		t.Errorf("Log output = %q", string(buf[:n]))
	}
	if len(f.LogCalls) != 1 || f.LogCalls[0].RangeStr != "HEAD~5..HEAD" {
		t.Errorf("LogCalls = %+v", f.LogCalls)
	}
}
