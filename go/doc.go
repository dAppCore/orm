// Package orm is a typed fluent communications bridge over backend media.
//
// The bridge is intentionally stateless: callers build intent, a Medium
// transports it, and writes are visible at the call site. Backend capability is
// declared by the Medium rather than assumed by the bridge.
package orm
