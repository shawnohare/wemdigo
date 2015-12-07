package wemdigo

// evaluate allows for synchronized execution of internal commands.
// We use this to wrap certain functions for export.
type evaluate struct {
	f   func() error
	err chan error
}

func (e *evaluate) eval() {
	e.err = make(chan error)
	go func() {
		e.err <- e.f()
		close(e.err)
	}()
}
