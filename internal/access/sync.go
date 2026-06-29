package access

import (
	"github.com/alby-tomy/gitcollect/internal/api"
	"github.com/alby-tomy/gitcollect/internal/collection"
)

// SyncCollaborators computes the correct GitHub/GitLab collaborator state
// for every (member, repo) pair in col and drives the API to match it,
// running up to 4 calls concurrently. Partial failures are collected and
// returned as a joined error; pairs that succeeded are still applied. The
// local manifest is not modified by this function.
//
// The concurrency-bounded implementation lives on collection.Collection
// itself (internal/collection/mutation.go), not here: this package already
// imports internal/collection, so internal/collection cannot import this
// package back without a cycle. This is a thin, spec-mandated wrapper
// around that method.
func SyncCollaborators(col *collection.Collection, client api.Client) (added, removed int, err error) {
	return col.SyncCollaborators(client)
}
