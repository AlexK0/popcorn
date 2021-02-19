package server

// Settings ...
type Settings struct {
	Port     int
	Host     string
	Password string

	WorkingDir  string
	LogFileName string
	LogSeverity string
}
