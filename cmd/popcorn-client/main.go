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
	// file, err := os.OpenFile(bucketFile, os.O_RDWR|os.O_CREATE, 0666)
	// if err != nil {
	// 	return fmt.Errorf("Can't open bucket file %q: %v", bucketFile, err)
	// }
	// if err := syscall.FcntlFlock(file.Fd(), syscall.F_SETLKW, &syscall.Flock_t{Type: syscall.F_WRLCK}); err != nil {
	// 	file.Close()
	// 	return fmt.Errorf("Can't lock bucket file %q for reading: %v", bucketFile, err)
	// }
	version := flag.Bool("version", false, "Show version and exit.")
	checkServers := flag.Bool("check-servers", false, "Check servers status.")

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
		client.CheckServers(settings, false)
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
