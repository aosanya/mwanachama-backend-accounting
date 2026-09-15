// Package routes is mwanachama-backend-accounting's own HTTP surface:
// decode a request, call one [accounting.LedgerRepository] method, encode
// the response — the same shape the root package's Go callers already get,
// just reachable from an HTTP mux. It exists so a route's request/response
// shape and its domain logic are authored and reviewed together, in the
// package that owns the domain, rather than reimplemented a second time in
// whichever process happens to mount this package.
//
// This package stays as vocabulary-neutral as the root package it wraps —
// no member/paybill/contribution concept appears anywhere a request or
// response shape is defined, matching CLAUDE.md's discipline for the root
// package itself. It also carries no capability gate: access control is an
// open question for whoever mounts this (CLAUDE.md's open question 1), not
// a decision made here. [Routes] returns every route as one list;
// [AccountRoutes]/[EntryRoutes] return one model's routes at a time, for a
// mounting process that wraps different groups in different policy.
//
// A route built from this package still needs a caller-identity/capability
// gate wrapped around it before it is safe to serve — this package answers
// "what happens once that gate has passed", never "who may pass it". The
// mounting process supplies that gate by wrapping the http.HandlerFunc this
// package returns, not by this package reaching for a session or a
// capability itself.
package routes
