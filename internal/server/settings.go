package server

// Settings ...
type Settings struct {
	Port int
	Host string

	WorkingDir  string
	LogFileName string
	LogSeverity string

	HeaderCacheLimit int64

	StatsdAddress string
}
