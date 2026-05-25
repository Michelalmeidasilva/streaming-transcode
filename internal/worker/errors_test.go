package worker

import (
	"errors"
	"testing"
)

func TestTerminalErrorMethods(t *testing.T) {
	base := errors.New("invalid payload")

	err := terminal(base)
	if err == nil {
		t.Fatalf("terminal() error = nil, want wrapped error")
	}
	if err.Error() != base.Error() {
		t.Fatalf("terminal().Error() = %q, want %q", err.Error(), base.Error())
	}
	if !errors.Is(err, base) {
		t.Fatalf("terminal() does not unwrap original error")
	}

	terminalMarker, ok := err.(interface{ Terminal() bool })
	if !ok {
		t.Fatalf("terminal() error does not expose Terminal()")
	}
	if !terminalMarker.Terminal() {
		t.Fatalf("Terminal() = false, want true")
	}
}

func TestTerminalNil(t *testing.T) {
	if err := terminal(nil); err != nil {
		t.Fatalf("terminal(nil) = %v, want nil", err)
	}
}
