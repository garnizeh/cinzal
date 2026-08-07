// Command replay folds a recorded match log and dumps state at a given round.
//
// It runs the same fold as the server with a null effect sink, so replaying a
// match never re-sends a historical notification (RFC-001 §7.4). The --rebuild
// flag regenerates the derived events and match_summary projections.
package main
