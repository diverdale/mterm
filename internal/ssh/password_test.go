package ssh

import (
	"strings"
	"testing"
)

func TestPasswordProviderEmptyIsNoOp(t *testing.T) {
	methods, cleanup, err := NewPasswordProvider("").Methods()
	if err != nil {
		t.Fatalf("empty password should not error: %v", err)
	}
	if methods != nil {
		t.Fatalf("empty password should yield nil methods, got %d", len(methods))
	}
	if cleanup != nil {
		t.Fatal("empty password should not need cleanup")
	}
}

func TestPasswordProviderReturnsBothMethods(t *testing.T) {
	methods, _, err := NewPasswordProvider("hunter2").Methods()
	if err != nil {
		t.Fatal(err)
	}
	if len(methods) != 2 {
		t.Fatalf("want 2 methods (password + keyboard-interactive), got %d", len(methods))
	}
}

func TestPasswordCommandProviderEmptyIsNoOp(t *testing.T) {
	methods, _, err := NewPasswordCommandProvider("").Methods()
	if err != nil {
		t.Fatalf("empty cmdline should not error: %v", err)
	}
	if methods != nil {
		t.Fatal("empty cmdline should yield nil methods")
	}
}

func TestPasswordCommandProviderRunsCommandAndTrims(t *testing.T) {
	methods, _, err := NewPasswordCommandProvider("printf '  hunter2 \\n\\n'").Methods()
	if err != nil {
		t.Fatalf("running command: %v", err)
	}
	if len(methods) != 2 {
		t.Fatalf("want 2 methods, got %d", len(methods))
	}
}

func TestPasswordCommandProviderEmptyOutputErrors(t *testing.T) {
	_, _, err := NewPasswordCommandProvider("printf ''").Methods()
	if err == nil {
		t.Fatal("empty stdout should produce an error")
	}
	if !strings.Contains(err.Error(), "empty output") {
		t.Fatalf("error %q didn't mention empty output", err.Error())
	}
}

func TestPasswordCommandProviderFailingCommandErrors(t *testing.T) {
	_, _, err := NewPasswordCommandProvider("exit 1").Methods()
	if err == nil {
		t.Fatal("non-zero exit should produce an error")
	}
}
