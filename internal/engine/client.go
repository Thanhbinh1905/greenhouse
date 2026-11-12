package engine

func TerminalMutation(v Response, op string) Request {
	return Request{Version: MaxVersion, Op: op, RunID: v.RunID, Input: v.Input, Gate: v.Gate, Expected: v.Token, Action: opaque(), Client: "terminal"}
}

func BackendName(backend string) string  { return backendNames[backend] }
func OutcomeLabel(outcome string) string { return outcomeLabels[outcome] }
