package access

import (
	"github.com/alby-tomy/gitcollect/v3/internal/api"
	"github.com/alby-tomy/gitcollect/v3/internal/collection"
	"github.com/alby-tomy/gitcollect/v3/internal/output"
)

// SyncCollaborators computes the correct GitHub/GitLab collaborator state
// for every (member, repo) pair in col and drives the API to match it,
// running up to 4 calls concurrently. Partial failures are collected and
// returned as a joined error; pairs that succeeded are still applied. The
// local manifest is not modified by this function.
//
// When showProgress is true a live "[current/total] Syncing collaborator
// access" indicator is written to stderr and erased once all jobs are done.
//
// The concurrency-bounded implementation lives on collection.Collection
// itself (internal/collection/mutation.go), not here: this package already
// imports internal/collection, so internal/collection cannot import this
// package back without a cycle. This is a thin, spec-mandated wrapper
// around that method.
func SyncCollaborators(col *collection.Collection, client api.Client, showProgress bool) (added, removed int, err error) {
	total := len(col.Members) * len(col.Repos)
	var progressFn func(int, int)
	if showProgress && total > 0 {
		progressFn = func(current, total int) {
			output.Progress(current, total, "Syncing collaborator access")
		}
	}
	added, removed, err = col.SyncCollaborators(client, progressFn)
	if showProgress && total > 0 {
		output.ClearProgress()
	}
	return
}
