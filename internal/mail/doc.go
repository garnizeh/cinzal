// Package mail holds the outbox, the templates, and the provider adapter.
//
// Email is on the critical path for both login and gameplay, so it gets an
// outbox table and a worker rather than an inline send (RFC-001 §13).
//
// The round_resolved template is generated from the same event stream filtered
// through the same fog as everything else. It is the single easiest place in
// the system to accidentally mail someone the whole board, and it has its own
// test for exactly that reason.
package mail
