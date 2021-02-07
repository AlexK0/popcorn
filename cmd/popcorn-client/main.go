package main

import (
	"math/rand"
	"os"
	"runtime"
	"time"

	"github.com/AlexK0/popcorn/internal/client"
	"github.com/AlexK0/popcorn/internal/common"
)

func mainWithRetCode() (retCode int, stdout []byte, stderr []byte) {
	settings := client.ReadClientSettings()
	if err := common.LoggerInit("popcorn-client", settings.LogFileName, settings.LogSeverity, settings.LogVerbosity); err != nil {
		common.LogFatal("Can't init logger", err)
	}

	if len(os.Args) < 3 {
		common.LogFatal("Compiler line expected")
	}

	rand.Seed(time.Now().UnixNano())
	return client.PerformCompilation(os.Args[1:], settings)
}

func main() {
	runtime.GOMAXPROCS(2)
	retCode, stdout, stderr := mainWithRetCode()

	os.Stdout.Write(stdout)
	os.Stderr.Write(stderr)
	os.Exit(retCode)
}
