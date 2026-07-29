package errors

import (
	"errors"
	"fmt"
	"testing"
)

func TestAsUnwrapsWrappedError(t *testing.T) {
	raw := errors.New("boom")
	wrapped := fmt.Errorf("context: %w", Internal(raw))

	e, ok := As(wrapped)
	if !ok {
		t.Fatal("As() = false, want true")
	}
	if e.Code != CodeInternal {
		t.Errorf("Code = %q, want %q", e.Code, CodeInternal)
	}
}

func TestAsFalseForPlainError(t *testing.T) {
	if _, ok := As(errors.New("plain")); ok {
		t.Error("As() = true for a plain error, want false")
	}
}

func TestErrorStringIncludesRaw(t *testing.T) {
	e := NotFound(errors.New("row missing"))
	if got := e.Error(); got != "khong tim thay: row missing" {
		t.Errorf("Error() = %q", got)
	}
}
