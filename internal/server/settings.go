package server

// Settings ...
type Settings struct {
	Port *int
	Host *string

	WorkingDir   *string
	LogFileName  *string
	LogVerbosity *int
	LogSeverity  *string
}
