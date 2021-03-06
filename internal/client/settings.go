package client

import (
	"os"
	"strings"

	"github.com/AlexK0/popcorn/internal/common"
)

// Settings ...
type Settings struct {
	Servers     []string
	LogFileName string
	LogSeverity string
	UseObjCache bool
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
			hosts := strings.Split(value, ";")
			settings.Servers = make([]string, 0, len(hosts))
			for _, host := range hosts {
				trimmedHost := strings.TrimSpace(host)
				if len(trimmedHost) != 0 {
					settings.Servers = append(settings.Servers, trimmedHost)
				}
			}
		} else if value := getEnvValue(envVar, "POPCORN_LOG_FILENAME="); len(value) != 0 {
			settings.LogFileName = value
		} else if value := getEnvValue(envVar, "POPCORN_LOG_SEVERITY="); len(value) != 0 {
			settings.LogSeverity = value
		} else if value := getEnvValue(envVar, "POPCORN_OBJ_CACHE="); len(value) != 0 {
			settings.UseObjCache = (value == "1") ||
				strings.EqualFold(value, "yes") || strings.EqualFold(value, "true") ||
				strings.EqualFold(value, "on") || strings.EqualFold(value, "enable")
		}
	}

	return &settings
}
