package cli

import "os"

// osUser returns the current user's login name, falling back to
// "unknown" if neither $USER nor $LOGNAME is set. Used as the default
// value for Deps.User so init_user has something useful recorded.
func osUser() string {
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	if u := os.Getenv("LOGNAME"); u != "" {
		return u
	}
	return "unknown"
}
