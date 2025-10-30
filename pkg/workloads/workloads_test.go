package workloads

import (
	"errors"
	"testing"
)

// mockWorkload is a test implementation of Workload.
type mockWorkload struct {
	name     string
	executed bool
}

func (m *mockWorkload) Name() string {
	return m.name
}

func (m *mockWorkload) Execute() error {
	m.executed = true
	return nil
}

func TestRegisterPanic(t *testing.T) {
	mock := &mockWorkload{name: "test-duplicate"}

	// Save and restore the registered map.
	oldRegistered := registered
	registered = make(map[string]Workload)
	defer func() {
		registered = oldRegistered
	}()

	// Register once.
	Register(mock)

	// Try to register again - should panic.
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when registering duplicate workload")
		}
	}()
	Register(mock)
}

func TestRun(t *testing.T) {
	mock := &mockWorkload{name: "test-run"}

	// Save and restore the registered map.
	oldRegistered := registered
	registered = make(map[string]Workload)
	defer func() {
		registered = oldRegistered
	}()

	Register(mock)

	err := Run("test-run")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !mock.executed {
		t.Error("workload was not executed")
	}
}

func TestRunNotFound(t *testing.T) {
	// Save and restore the registered map
	oldRegistered := registered
	registered = make(map[string]Workload)
	defer func() {
		registered = oldRegistered
	}()

	err := Run("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent workload")
	}

	var notFoundErr ErrWorkloadNotFoundError
	if !errors.As(err, &notFoundErr) {
		t.Errorf("expected ErrWorkloadNotFoundError, got %T", err)
	}
}

func TestList(t *testing.T) {
	mock1 := &mockWorkload{name: "test-list-1"}
	mock2 := &mockWorkload{name: "test-list-2"}

	// Save and restore the registered map.
	oldRegistered := registered
	registered = make(map[string]Workload)
	defer func() {
		registered = oldRegistered
	}()

	Register(mock1)
	Register(mock2)

	list := List()
	if len(list) != 2 {
		t.Errorf("expected 2 workloads, got %d", len(list))
	}

	// Check both are present.
	found := make(map[string]bool)
	for _, name := range list {
		found[name] = true
	}

	if !found["test-list-1"] || !found["test-list-2"] {
		t.Error("not all workloads were listed")
	}
}
