// Package debug holds the development tooling: the fog inspector, the god view,
// the RNG trace, time travel, order injection, and the speed run.
//
// Every file in this package carrying debug functionality is guarded by
// //go:build debug and is not compiled into the production binary. A runtime
// flag would be one environment-variable mistake away from serving a god view
// of live matches, and that mistake is unrecoverable — you cannot un-leak a map
// (RFC-001 §15.1).
//
// This file deliberately carries no build constraint. It declares the package
// and nothing else, so the package exists to go list in both build
// configurations. Without it the directory would not be a package at all in a
// production build, and the import gates would inspect a package set that
// silently shrank rather than reporting a change. Do not add the build tag
// here; add it to every file that carries actual debug code.
package debug
