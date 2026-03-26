package hew

// Event is a sealed interface for agent events.
// Only types in this package can implement it (unexported marker method).
// The compiler enforces the seal; the exhaustive linter catches missing switch cases.
type Event interface{ event() }

// EventResponse fires when the model returns a response.
type EventResponse struct {
	Message Message
	Usage   Usage
}

// EventCommandStart fires before running a command.
type EventCommandStart struct {
	Command string
	Dir     string
}

// EventCommandDone fires after a command finishes.
type EventCommandDone struct {
	Command  string
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}

// EventProtocolFailure fires when the model response fails protocol validation.
type EventProtocolFailure struct {
	Reason  string // machine-readable: "invalid_json", "missing_command", etc.
	Message string // human-readable explanation
}

// EventDebug carries internal diagnostic messages.
type EventDebug struct {
	Message string
}

func (EventResponse) event()        {}
func (EventCommandStart) event()    {}
func (EventCommandDone) event()     {}
func (EventProtocolFailure) event() {}
func (EventDebug) event()           {}

// StepResult is the outcome of one Step call.
type StepResult struct {
	Response Response // the LLM response
	Turn     Turn     // parsed protocol turn (zero value on protocol failure)
	Output   string   // formatted command output payload(s), "" if no command ran
	ExecErr  error    // nil if command succeeded or didn't run
}
