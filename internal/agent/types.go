// Package agent is the extensibility seam: everything agent-specific (how to
// launch it, its options, where its conversations live and how to resume,
// rename and delete them) sits behind Adapter.
package agent

// Session is one conversation in an agent's store.
type Session struct {
	Agent string
	// ID is store-specific: a file stem, a file path, or a database key.
	ID    string
	Cwd   string
	Title string
	// When is the last activity, seconds since the epoch.
	When float64
	// Prompts are the first few things the user typed.
	Prompts []string
	// Path is the backing file, for file stores.
	Path string
	// Context is the tokens the model last saw (input + cache read + cache
	// write of the latest turn); 0 when the store doesn't record usage.
	Context int
}

// Option is a per-launch choice in the launch sheet, mapped to CLI flags.
type Option struct {
	Key   string
	Label string
	// Choices: the first is the agent's own default (no flag).
	Choices []string
	Help    string
	// ResumeOnly options are only offered when resuming a conversation.
	ResumeOnly bool
	// Args for a chosen value (never called for the default).
	Args func(value string) []string
}

// Store reads (and optionally edits) an agent's conversations.
type Store interface {
	// List returns up to limit sessions, newest first.
	List(limit int) []Session
	Get(id string) (Session, bool)
	ResumeArgs(id string) []string
}

// Optional capabilities a Store may implement.
type (
	Renamer interface {
		Rename(s Session, title string) error
	}
	Deleter interface{ Delete(s Session) error }
	// Liver reports sessions the agent has open right now: id -> pid.
	Liver interface{ Live() map[string]int }
)

type Adapter struct {
	ID    string
	Label string
	// Bin is the executable name.
	Bin string
	// DefaultArgs go on every launch and resume (an app's "args" overrides).
	DefaultArgs []string
	// LaunchArgs are what omarchy-agent would add, used when launching directly.
	LaunchArgs []string
	Options    []Option
	Sessions   Store
}
