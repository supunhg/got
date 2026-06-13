// Package eventbus is the in-process publish/subscribe bus used by GOT.
//
// Every domain event (RepositoryOpened, CommitCreated, BranchCreated,
// WorkspaceCreated, ...) flows through the bus before it reaches a
// subscriber. Subscribers are typically in-process Go callbacks
// (e.g. the log session, the future snapshot engine, a plugin's
// subscriber goroutine) but the bus is structured so an external
// plugin process can attach by reading the durable event log directly.
//
// Three guarantees are exposed to callers:
//
//  1. Delivery: Publish synchronously persists the event to the SQLite
//     events table (the replay log) and only then dispatches it to
//     subscribers. Subscribers therefore never see an event that is
//     missing from the log; if the process dies after Publish returns,
//     the event is durable.
//
//  2. Fan-out: Publish returns once every synchronous subscriber has
//     been invoked; asynchronous subscribers are detached and the bus
//     does not wait for them. A panicking subscriber does not affect
//     other subscribers or the publisher.
//
//  3. Replay: any caller can ask for the last N events of a topic
//     (or all topics) from the durable log. This is how the
//     `got event list` / `got event show` commands and any future
//     catch-up subscriber bootstrap themselves.
//
// The bus is not a distributed system. It is a process-local
// convenience layer over the SQLite events table; durability is
// provided by the store, and concurrency is bounded by the number of
// in-process subscribers. Future external plugins will read the same
// log table directly or via a future IPC bridge (not part of v0.4).
//
// See docs/EVENT_BUS.md for the user-facing story and the list of
// topics that the bus knows about.
package eventbus
