package server

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"runtime"
	"sync/atomic"
	"time"
)

// AtomicStat ...
type AtomicStat struct {
	counter int64
}

// Increment ...
func (s *AtomicStat) Increment() {
	atomic.AddInt64(&s.counter, 1)
}

// Set ...
func (s *AtomicStat) Set(v int64) {
	atomic.StoreInt64(&s.counter, v)
}

// AddDuration ...
func (s *AtomicStat) AddDuration(d time.Duration) {
	atomic.AddInt64(&s.counter, int64(d))
}

// Get ...
func (s *AtomicStat) Get() int64 {
	return atomic.LoadInt64(&s.counter)
}

// GetAsSeconds ...
func (s *AtomicStat) GetAsSeconds() float64 {
	return time.Duration(s.Get()).Seconds()
}

// RPCCallStats ...
type RPCCallStats struct {
	Calls          AtomicStat
	Errors         AtomicStat
	ProcessingTime AtomicStat
}

// RPCCallObserver ...
type RPCCallObserver struct {
	start time.Time
	stat  *RPCCallStats
}

// StartRPCCall ...
func (c *RPCCallStats) StartRPCCall() RPCCallObserver {
	c.Calls.Increment()
	return RPCCallObserver{time.Now(), c}
}

// Finish ...
func (o RPCCallObserver) Finish() error {
	o.stat.ProcessingTime.AddDuration(time.Since(o.start))
	return nil
}

// FinishWithError ...
func (o RPCCallObserver) FinishWithError(err error) error {
	o.stat.Errors.Increment()
	o.stat.ProcessingTime.AddDuration(time.Since(o.start))
	return err
}

// CompilationServerStats ...
type CompilationServerStats struct {
	SendingHeadersReceived AtomicStat

	StartCompilationSession RPCCallStats
	TransferFile            RPCCallStats
	CompileSource           RPCCallStats
	CloseSession            RPCCallStats

	statsdConnection net.Conn
	statsBuffer      bytes.Buffer
}

// MakeServerStats ...
func MakeServerStats(statsdHostPort string) (*CompilationServerStats, error) {
	if len(statsdHostPort) == 0 {
		return &CompilationServerStats{}, nil
	}

	conn, err := net.Dial("udp", statsdHostPort)
	if err != nil {
		return nil, err
	}
	return &CompilationServerStats{
		statsdConnection: conn,
	}, nil
}

func (cs *CompilationServerStats) writeStat(statName string, value int64) {
	fmt.Fprintf(&cs.statsBuffer, "popcorn.%s:%d|g\n", statName, value)
}

func (cs *CompilationServerStats) writeFloatStat(statName string, value float64) {
	fmt.Fprintf(&cs.statsBuffer, "popcorn.%s:%.9f|g\n", statName, value)
}

func (cs *CompilationServerStats) writeAtomicStat(statName string, statValue *AtomicStat) {
	cs.writeStat(statName, statValue.Get())
}

func (cs *CompilationServerStats) writeRPCCallStat(rpcCallName string, statValue *RPCCallStats) {
	fmt.Fprintf(&cs.statsBuffer, "popcorn.rpc.%s.calls:%d|g\n", rpcCallName, statValue.Calls.Get())
	fmt.Fprintf(&cs.statsBuffer, "popcorn.rpc.%s.errors:%d|g\n", rpcCallName, statValue.Errors.Get())
	fmt.Fprintf(&cs.statsBuffer, "popcorn.rpc.%s.processing_time:%.9f|g\n", rpcCallName, statValue.ProcessingTime.GetAsSeconds())
}

func (cs *CompilationServerStats) feedBufferWithStats(compilationServer *CompilationServer) {
	cs.writeFloatStat("server.uptime", time.Since(compilationServer.StartTime).Seconds())
	cs.writeStat("server.goroutines", int64(runtime.NumGoroutine()))

	cs.writeStat("sessions.active", compilationServer.UserSessions.ActiveSessions())

	cs.writeStat("caches.clients.count", compilationServer.Clients.Count())
	cs.writeStat("caches.clients.random_client_cache_size", compilationServer.Clients.GetRandomClientCacheSize())

	cs.writeStat("caches.system_headers.count", compilationServer.SystemHeaders.GetSystemHeadersCount())

	cs.writeStat("caches.user_headers.count", compilationServer.HeaderFileCache.GetFilesCount())
	cs.writeStat("caches.user_headers.purged", compilationServer.HeaderFileCache.GetPurgedFiles())
	cs.writeStat("caches.user_headers.disk_bytes", compilationServer.HeaderFileCache.GetBytesOnDisk())

	cs.writeStat("sending_headers.in_progress", compilationServer.UploadingHeaders.SendingHeadersCount())
	cs.writeAtomicStat("sending_headers.received", &cs.SendingHeadersReceived)

	cs.writeRPCCallStat("start_compilation_session", &cs.StartCompilationSession)
	cs.writeRPCCallStat("transfer_file", &cs.TransferFile)
	cs.writeRPCCallStat("compile_source", &cs.CompileSource)
	cs.writeRPCCallStat("close_session", &cs.CloseSession)

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	cs.writeStat("memory.alloc", int64(mem.Alloc))
	cs.writeStat("memory.total_alloc", int64(mem.TotalAlloc))
	cs.writeStat("memory.sys", int64(mem.Sys))
	cs.writeStat("memory.lookups", int64(mem.Lookups))
	cs.writeStat("memory.mallocs", int64(mem.Mallocs))
	cs.writeStat("memory.frees", int64(mem.Frees))
	cs.writeStat("memory.heap_alloc", int64(mem.HeapAlloc))
	cs.writeStat("memory.heap_sys", int64(mem.HeapSys))
	cs.writeStat("memory.heap_idle", int64(mem.HeapIdle))
	cs.writeStat("memory.heap_inuse", int64(mem.HeapInuse))
	cs.writeStat("memory.heap_released", int64(mem.HeapReleased))
	cs.writeStat("memory.heap_objects", int64(mem.HeapObjects))
	cs.writeStat("memory.stack_inuse", int64(mem.StackInuse))
	cs.writeStat("memory.stack_sys", int64(mem.StackSys))
	cs.writeStat("memory.mspan_inuse", int64(mem.MSpanInuse))
	cs.writeStat("memory.mspan_sys", int64(mem.MSpanSys))
	cs.writeStat("memory.mcache_inuse", int64(mem.MCacheInuse))
	cs.writeStat("memory.mcache_sys", int64(mem.MCacheSys))
	cs.writeStat("memory.buck_hash_sys", int64(mem.BuckHashSys))
	cs.writeStat("memory.gc_sys", int64(mem.GCSys))
	cs.writeStat("memory.other_sys", int64(mem.OtherSys))

	cs.writeStat("gc.next", int64(mem.NextGC))
	cs.writeStat("gc.last", int64(mem.LastGC))
	cs.writeStat("gc.cycles", int64(mem.NumGC))
	cs.writeStat("gc.forced_cycles", int64(mem.NumForcedGC))
	cs.writeFloatStat("gc.pause_total", time.Duration(mem.PauseTotalNs).Seconds())
	cs.writeFloatStat("gc.cpu_fraction", mem.GCCPUFraction)
}

// SendStats ...
func (cs *CompilationServerStats) SendStats(compilationServer *CompilationServer) {
	if cs.statsdConnection == nil {
		return
	}

	cs.feedBufferWithStats(compilationServer)

	_, _ = io.Copy(cs.statsdConnection, &cs.statsBuffer)
	cs.statsBuffer.Reset()
}

// GetStatsRawBytes ...
func (cs *CompilationServerStats) GetStatsRawBytes(compilationServer *CompilationServer) []byte {
	cs.feedBufferWithStats(compilationServer)
	result := cs.statsBuffer.Bytes()
	cs.statsBuffer.Reset()
	return result
}

// Close ...
func (cs *CompilationServerStats) Close() {
	if cs.statsdConnection != nil {
		cs.statsdConnection.Close()
	}
	cs.statsdConnection = nil
}
