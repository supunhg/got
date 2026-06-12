package cli

import "time"

// timeNow is the default Deps.Now implementation, used when the caller
// does not supply one. Tests can override Deps.Now directly; this
// variable is package-level so it could in principle be replaced at
// startup, but the Deps injection is the supported path.
var timeNow = time.Now
