// Package controlplane contains protocol control-plane logic (e.g. rekey) without IO.
//
// Rules:
// - No direct reads/writes to transport or TUN.
// - Prefer returning data/actions to be executed by dataplane/sessionplane edges.
package controlplane
