package gerr

import (
	"errors"
	"strings"
	"testing"
)

func TestError_ErrorAndUnwrap(t *testing.T) {
	e := &Error{Code: CodeGeneric, Message: "boom"}
	if got, want := e.Error(), "boom"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}

	cause := errors.New("inner")
	e2 := &Error{Code: CodeGeneric, Message: "outer", Cause: cause}
	if got, want := e2.Error(), "outer: inner"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if !errors.Is(e2, cause) {
		t.Error("errors.Is should find the cause")
	}
}

func TestError_UserMessage(t *testing.T) {
	e := &Error{Message: "msg", Hint: "try this"}
	if got, want := e.UserMessage(), "msg\n  try this"; got != want {
		t.Errorf("UserMessage() = %q, want %q", got, want)
	}
	if got := (&Error{Message: "msg"}).UserMessage(); got != "msg" {
		t.Errorf("UserMessage() no hint = %q", got)
	}
}

func TestExitCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, 1},
		{"plain", errors.New("x"), 1},
		{"NotInGitRepo", NotInGitRepo("/x"), int(CodeNotInGitRepo)},
		{"Validation", Validation("bad"), int(CodeValidation)},
		{"wrapped", Wrap(CodeGeneric, errors.New("inner"), "outer"), int(CodeGeneric)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExitCode(tc.err); got != tc.want {
				t.Errorf("ExitCode() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestUserMessage(t *testing.T) {
	if got := UserMessage(errors.New("raw")); got != "raw" {
		t.Errorf("plain err = %q", got)
	}
	e := NotInGitRepo("/path")
	if !strings.Contains(UserMessage(e), "/path") {
		t.Errorf("UserMessage should include path: %q", UserMessage(e))
	}
}

func TestConstructors(t *testing.T) {
	if NotInitialized().Code != CodeNotInitialized {
		t.Error("NotInitialized: wrong code")
	}
	if GitError(errors.New("x"), "status").Cause == nil {
		t.Error("GitError: missing cause")
	}
}

func TestPermissionDenied(t *testing.T) {
	e := PermissionDenied("/var/got/plugins/foo")
	if e.Code != CodeGeneric {
		t.Errorf("Code = %d, want CodeGeneric (%d)", e.Code, CodeGeneric)
	}
	if !strings.Contains(e.Message, "/var/got/plugins/foo") {
		t.Errorf("Message should include the path, got %q", e.Message)
	}
	if !strings.Contains(e.UserMessage(), "permission denied") {
		t.Errorf("UserMessage should contain 'permission denied', got %q", e.UserMessage())
	}
	if e.Hint == "" {
		t.Error("PermissionDenied should carry a hint")
	}
	// ExitCode maps it like any other *Error.
	if got := ExitCode(e); got != int(CodeGeneric) {
		t.Errorf("ExitCode = %d, want %d", got, CodeGeneric)
	}
}

func TestPluginError(t *testing.T) {
	cause := errors.New("exec: not found")
	e := PluginError("github", cause, "manifest probe failed")
	if e.Code != CodePlugin {
		t.Errorf("Code = %d, want CodePlugin (%d)", e.Code, CodePlugin)
	}
	if !strings.Contains(e.Message, "plugin github") {
		t.Errorf("Message should name the plugin, got %q", e.Message)
	}
	if !strings.Contains(e.Message, "manifest probe failed") {
		t.Errorf("Message should include the user message, got %q", e.Message)
	}
	if !errors.Is(e, cause) {
		t.Error("errors.Is should find the wrapped cause")
	}
	if got := ExitCode(e); got != int(CodePlugin) {
		t.Errorf("ExitCode = %d, want %d (64)", got, CodePlugin)
	}
	if got := UserMessage(e); !strings.Contains(got, "plugin github") {
		t.Errorf("UserMessage should be prefixed, got %q", got)
	}
}

func TestPluginError_EmptyName(t *testing.T) {
	// An empty plugin name is allowed (defensive: a future caller may
	// not know the name); the constructor should still produce a
	// usable error with the right code.
	e := PluginError("", nil, "boom")
	if e.Code != CodePlugin {
		t.Errorf("Code = %d, want CodePlugin", e.Code)
	}
	if e.Message != "boom" {
		t.Errorf("Message = %q, want %q (no plugin prefix)", e.Message, "boom")
	}
}
