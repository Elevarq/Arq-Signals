package tests

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestCommitErrorIsChecked verifies that tx.Commit(ctx) in collectTarget
// has its error return value captured and checked. A bare tx.Commit(ctx)
// without error handling would allow downstream persistence to proceed
// after a failed PostgreSQL transaction.
//
// Traces: ARQ-SIGNALS-R021 / TC-SIG-036
func TestCommitErrorIsChecked(t *testing.T) {
	root := repoRoot(t)
	src := readFileString(t, filepath.Join(root, "internal", "collector", "collector.go"))

	// The commit call must be in an error-checking pattern.
	// Correct:   if err := tx.Commit(ctx); err != nil {
	// Incorrect: tx.Commit(ctx)   (bare call, error discarded)
	if strings.Contains(src, "\ttx.Commit(ctx)\n") {
		t.Fatal("tx.Commit(ctx) is called without checking the returned error — " +
			"downstream persistence may proceed after a failed PostgreSQL transaction")
	}

	if !strings.Contains(src, "tx.Commit(ctx); err != nil") {
		t.Fatal("tx.Commit(ctx) error is not checked with 'if err := tx.Commit(ctx); err != nil'")
	}
}

// TestCommitFailureBlocksDownstreamPersistence verifies that a commit
// failure causes the function to return before reaching InsertQueryRunBatch
// and InsertSnapshot. This ensures no contradictory state is created in
// SQLite when the PostgreSQL transaction fails.
//
// Traces: ARQ-SIGNALS-R021 / TC-SIG-036
func TestCommitFailureBlocksDownstreamPersistence(t *testing.T) {
	root := repoRoot(t)
	src := readFileString(t, filepath.Join(root, "internal", "collector", "collector.go"))

	// Find the positions of the commit error handling and downstream persistence.
	commitReturn := strings.Index(src, `return fmt.Errorf("commit tx for`)
	insertBatch := strings.Index(src, "InsertQueryRunBatch")
	insertSnap := strings.Index(src, "InsertSnapshot")

	if commitReturn < 0 {
		t.Fatal("collector.go does not return an error on commit failure — " +
			"missing 'return fmt.Errorf(\"commit tx for...'")
	}
	if insertBatch < 0 {
		t.Fatal("collector.go does not call InsertQueryRunBatch")
	}
	if insertSnap < 0 {
		t.Fatal("collector.go does not call InsertSnapshot")
	}

	// The commit-error return must appear BEFORE the downstream persistence calls.
	// This guarantees that if commit fails, the function exits before writing to SQLite.
	if commitReturn > insertBatch {
		t.Error("commit error return appears AFTER InsertQueryRunBatch — " +
			"SQLite writes may proceed after failed PostgreSQL commit")
	}
	if commitReturn > insertSnap {
		t.Error("commit error return appears AFTER InsertSnapshot — " +
			"SQLite writes may proceed after failed PostgreSQL commit")
	}
}
