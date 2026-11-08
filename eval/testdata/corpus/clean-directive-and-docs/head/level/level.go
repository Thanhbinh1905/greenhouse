package level

//go:generate stringer -type=Level

// Level is a log severity. Higher values are more severe.
type Level int

const (
	Debug Level = iota
	Info
	Warn
)
