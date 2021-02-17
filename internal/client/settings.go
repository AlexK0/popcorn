package client

import (
	"os"
	"strconv"
	"strings"

	"github.com/AlexK0/popcorn/internal/common"
)

// Settings ...
type Settings struct {
	Servers      []string
	LogFileName  string
	LogVerbosity int
	LogSeverity  string
}

func getEnvValue(envVar string, key string) string {
	if len(envVar) > len(key) && strings.HasPrefix(envVar, key) {
		return envVar[len(key):]
	}
	return ""
}

// ReadClientSettings ...
func ReadClientSettings() *Settings {
	settings := Settings{
		LogSeverity: common.WarningSeverity,
	}
	for _, envVar := range os.Environ() {
		if value := getEnvValue(envVar, "POPCORN_SERVERS="); len(value) != 0 {
			settings.Servers = strings.Split(value, ";")
			for i, host := range settings.Servers {
				settings.Servers[i] = strings.TrimSpace(host)
			}
		} else if value := getEnvValue(envVar, "POPCORN_LOG_FILENAME="); len(value) != 0 {
			settings.LogFileName = value
		} else if value := getEnvValue(envVar, "POPCORN_LOG_VERBOSITY="); len(value) != 0 {
			settings.LogVerbosity, _ = strconv.Atoi(value)
		} else if value := getEnvValue(envVar, "POPCORN_LOG_SEVERITY="); len(value) != 0 {
			settings.LogSeverity = value
		}
	}

	return &settings
}
