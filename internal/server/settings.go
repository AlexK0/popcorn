package server

// Settings ...
type Settings struct {
	Port int
	Host string

	WorkingDir  string
	LogFileName string
	LogSeverity string

	SrcCacheLimit int64
	ObjCacheLimit int64

	StatsdAddress string
}
