// Package auth implements email OTP login, sessions, and guest accounts.
//
// No passwords: no password storage, no reset flow, no credential stuffing. The
// tradeoff is that email joins the login critical path, which is why sessions
// run 90 days — a player who waits for an email every time they check an
// asynchronous match stops checking (RFC-001 §12).
//
// Guests exist because GDD §17 promises invite links with no mandatory signup.
// A display name is enough for synchronous play, where everyone is present;
// asynchronous play requires an email, because the deadline notification is the
// product (RFC-001 §12.1).
package auth
