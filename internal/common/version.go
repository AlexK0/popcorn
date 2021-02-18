package common

var version string

// GetVersion ...
func GetVersion() string {
	if len(version) == 0 {
		return "Unknown"
	}
	return version
}
