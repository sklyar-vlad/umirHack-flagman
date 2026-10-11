package errors

// RevisionConflict exposes only the current optimistic concurrency version.
type RevisionConflict struct{ CurrentRevision int }

func (e *RevisionConflict) Error() string { return "revision conflict" }

func (e *RevisionConflict) Unwrap() error { return ErrConflict }
