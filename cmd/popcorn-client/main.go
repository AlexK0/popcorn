package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/AlexK0/popcorn/internal/client"
	"github.com/AlexK0/popcorn/internal/common"
)

func main() {
	runtime.GOMAXPROCS(2)
	version := flag.Bool("version", false, "Show version and exit.")
	checkServers := flag.Bool("check-servers", false, "Check servers status.")
	checkCompiler := flag.String("compiler", "gcc", "Check if the compiler available on the servers.")

	flag.Parse()

	if *version {
		fmt.Println(common.GetVersion())
		os.Exit(0)
	}

	settings := client.ReadClientSettings()
	if err := common.LoggerInit("popcorn-client", settings.LogFileName, settings.LogSeverity); err != nil {
		common.LogFatal("Can't init logger", err)
	}

	if *checkServers {
		client.CheckServers(settings, *checkCompiler)
		os.Exit(0)
	}

	if len(os.Args) < 3 {
		common.LogFatal("Compiler line expected")
	}

	retCode, stdout, stderr := client.PerformCompilation(os.Args[1:], settings)
	os.Stdout.Write(stdout)
	os.Stderr.Write(stderr)
	os.Exit(retCode)
}
