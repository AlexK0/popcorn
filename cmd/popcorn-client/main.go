package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/AlexK0/popcorn/internal/client"
	"github.com/AlexK0/popcorn/internal/common"
)

func mainWithRetCode(settings *client.Settings) (retCode int, stdout []byte, stderr []byte) {
	return client.PerformCompilation(os.Args[1:], settings)
}

func main() {
	version := flag.Bool("version", false, "Show version and exit.")
	checkServers := flag.Bool("check-servers", false, "Check servers status.")
	updateServerNewBinaryPath := flag.String("update-servers", "", "Check servers status.")

	flag.Parse()

	if *version {
		fmt.Println(common.GetVersion())
		os.Exit(0)
	}

	runtime.GOMAXPROCS(2)
	settings := client.ReadClientSettings()
	if err := common.LoggerInit("popcorn-client", settings.LogFileName, settings.LogSeverity, settings.LogVerbosity); err != nil {
		common.LogFatal("Can't init logger", err)
	}

	if *checkServers {
		client.CheckServers(settings)
		os.Exit(0)
	}

	if len(*updateServerNewBinaryPath) != 0 {
		client.UpdateServers(settings, *updateServerNewBinaryPath)
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
