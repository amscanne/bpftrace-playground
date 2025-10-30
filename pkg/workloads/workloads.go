package workloads

// Workload is a generic workload.
type Workload interface {
	// Returns the unique workload name.
	Name() string

	// Executes the workload.
	Execute() error
}

var registered = map[string]Workload{}

// Registers the given workload.
//
// Should be called during init; will panic if there is already
// a workload registered with the same name.
func Register(w Workload) {
	if _, exists := registered[w.Name()]; exists {
		panic("workload already registered: " + w.Name())
	}
	registered[w.Name()] = w
}

// Returned if the workload cannot be found.
type ErrWorkloadNotFound struct {
	Name string
}

// Error implements error.Error.
func (e ErrWorkloadNotFound) Error() string {
	return "workload not found: " + e.Name
}

// Run executes the workload with the given name.
func Run(name string) error {
	w, exists := registered[name]
	if !exists {
		return ErrWorkloadNotFound{name}
	}
	return w.Execute()
}

// List returns the set of available workloads.
func List() []string {
	var names []string
	for name := range registered {
		names = append(names, name)
	}
	return names
}
