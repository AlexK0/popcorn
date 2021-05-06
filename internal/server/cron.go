package server

import (
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/AlexK0/popcorn/internal/common"
)

// Cron ...
type Cron struct {
	wg       sync.WaitGroup
	stopFlag int32
	Server   *CompilationServer
	signals  chan os.Signal
}

func (c *Cron) doCron() {
	for atomic.LoadInt32(&c.stopFlag) == 0 {
		cronStartTime := time.Now()
		c.Server.Stats.SendStats(c.Server)

		c.Server.HeaderFileCache.PurgeLastElementsIfRequired()
		c.Server.Clients.PurgeOutdatedUsers()

		sleepTime := time.Second - time.Since(cronStartTime)
		if sleepTime <= 0 {
			sleepTime = time.Nanosecond
		}
		for sleepTime > 0 {
			select {
			case sig := <-c.signals:
				common.LogInfo("Got signal ", sig)
				if sig == syscall.SIGUSR1 {
					if err := common.RotateLogFile(); err != nil {
						common.LogError("Can't rotate log file", err)
					} else {
						common.LogInfo("Log file was rotated")
					}
				} else if sig == syscall.SIGTERM {
					common.LogInfo("Start graceful stop")
					c.Server.GRPCServer.GracefulStop()
				}
			case <-time.After(sleepTime):
				break
			}
			sleepTime = time.Second - time.Since(cronStartTime)
		}
	}

	c.wg.Done()
}

// Start ...
func (c *Cron) Start() {
	c.signals = make(chan os.Signal, 2)
	signal.Notify(c.signals, syscall.SIGUSR1, syscall.SIGTERM)
	c.wg.Add(1)
	go c.doCron()
}

// Stop ...
func (c *Cron) Stop() {
	atomic.StoreInt32(&c.stopFlag, 1)
	c.wg.Wait()
}
