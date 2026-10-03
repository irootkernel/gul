package session

// CreationOutcome exposes only the Gul session identity. An allocation receipt
// cannot substitute for the required public Orchestrated Session aggregate.
type CreationOutcome struct {
	SessionID string
	Unknown   bool
}
