package main

import (
	"os"
	"runtime"

	"github.com/AlexK0/popcorn/internal/client"
	"github.com/AlexK0/popcorn/internal/common"
)

func mainWithRetCode(settings *client.Settings) (retCode int, stdout []byte, stderr []byte) {
	return client.PerformCompilation(os.Args[1:], settings)
}

func main() {
	runtime.GOMAXPROCS(2)
	settings := client.ReadClientSettings()
	if err := common.LoggerInit("popcorn-client", settings.LogFileName, settings.LogSeverity, settings.LogVerbosity); err != nil {
		common.LogFatal("Can't init logger", err)
	}

	if len(os.Args) > 1 && os.Args[1] == "-check-servers" {
		client.CheckServers(settings)
		os.Exit(0)
	}

	if len(os.Args) < 3 {
		common.LogFatal("Compiler line expected")
	}

	retCode, stdout, stderr := mainWithRetCode(settings)

	os.Stdout.Write(stdout)
	os.Stderr.Write(stderr)
	os.Exit(retCode)
}
