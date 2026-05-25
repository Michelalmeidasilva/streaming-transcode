package worker

type terminalError struct {
	err error
}

func (e terminalError) Error() string {
	return e.err.Error()
}

func (e terminalError) Unwrap() error {
	return e.err
}

func (e terminalError) Terminal() bool {
	return true
}

func terminal(err error) error {
	if err == nil {
		return nil
	}
	return terminalError{err: err}
}
