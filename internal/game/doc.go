// Package game holds the vocabulary shared between the engine and everything
// outside it: PlayerView, Order, OrderDraft, Config, Event, and the identifier
// types. It is the only package that crosses the fog boundary.
//
// Three constraints make this package load-bearing rather than merely
// convenient. All three are decided in docs/decisions/D01-package-layout.md.
//
// It imports nothing outside the standard library, and in particular it never
// imports internal/rules. That is what lets internal/render and internal/web
// import this package while remaining unable to name the match state.
//
// It declares no field of type any, no interface{}, and no unconstrained type
// parameter. The import rule proves that render and web cannot *name* the match
// state; it does not by itself stop a value travelling inside a type they can
// name. A package that imports nothing cannot name a state type, so forbidding
// dynamic containers here makes smuggling one impossible rather than unlikely.
//
// There is no game.State and there must never be one. The match state lives in
// internal/rules and has no name outside it. game.PlayerView is the only
// state-derived type that crosses, and it is produced by exactly one function,
// rules.Project.
//
// The split to keep stable: game is the vocabulary, rules is the behaviour.
// Data that crosses layers belongs here; anything that computes belongs there.
package game
