package server

import (
	"sync"
	"sync/atomic"
	"time"
)

// Cron ...
type Cron struct {
	wg       sync.WaitGroup
	stopFlag int32
	Server   *CompilationServer
}

func (c *Cron) doCron() {
	for atomic.LoadInt32(&c.stopFlag) == 0 {
		cronStartTime := time.Now()
		c.Server.Stats.SendStats(c.Server)

		c.Server.HeaderFileCache.PurgeLastElementsIfRequired()

		sleepTime := time.Second - time.Since(cronStartTime)
		if sleepTime > 0 {
			time.Sleep(sleepTime)
		}
	}

	c.wg.Done()
}

// Start ...
func (c *Cron) Start() {
	c.wg.Add(1)
	go c.doCron()
}

// Stop ...
func (c *Cron) Stop() {
	atomic.StoreInt32(&c.stopFlag, 1)
	c.wg.Wait()
}
