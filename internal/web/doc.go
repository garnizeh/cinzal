// Package web holds the HTTP handlers, routing, and the SSE hub.
//
// It never imports internal/rules. Everything it needs from the engine comes
// through internal/match as game types, and a CI gate enforces this over
// .Imports, .TestImports and .XTestImports
// (docs/decisions/D01-package-layout.md).
//
// The wire format is HTML, not JSON, and that is a security property rather
// than a style preference: a JSON endpoint is a second surface that must
// independently be kept fog-safe, and it is far easier to over-return one field
// in JSON than in hand-written HTML (RFC-001 §3).
//
// The seat comes from the session on every request and never from the payload.
package web
