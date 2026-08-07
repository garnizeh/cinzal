// Package render holds the templ components, including the server-rendered SVG
// map. Every component takes a game.PlayerView and renders a self-contained
// fragment; the full page composes the same components, so there is exactly one
// code path per piece of UI whether it arrives as a page load or an HTMX swap.
//
// It never imports internal/rules, and cannot: the match state has no name in
// this package, so a template physically cannot leak what it cannot name
// (RFC-001 §3). A CI gate enforces it over .Imports, .TestImports and
// .XTestImports.
//
// A test in this package that needs a realistic view constructs a
// game.PlayerView directly. It does not reach for the engine to build one —
// that exemption would be the first place the boundary leaks, and the fixtures
// are shared with the bot tests anyway.
package render
