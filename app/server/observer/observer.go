// Package observer provides utilities for graceful shutdown.
// It ensures that the server only shuts down after all ongoing processes
// have been completed, helping to avoid data loss.
package observer

import (
	"context"
	"fmt"
	"github.com/shirou/gopsutil/cpu"
	"log/slog"
	"math"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

var allocMemoryBefore uint64
var allocMemoryPeak uint64
var processorLoadPeak float64
var goroutinePeak int

// Observer interface defines methods for tracking ongoing processes.
// It's especially useful for graceful shutdowns, ensuring that the system
// waits for all active tasks to finish.
type Observer interface {
	// StartProcess should be called at the beginning of every request or operation.
	// It registers a new active process that the server must wait for before shutting down.
	StartProcess(uid string, processName string)

	// EndProcess should be called when a process is finished.
	// It unregisters the process so the system knows it's safe to shut down if no more remain.
	EndProcess(uid string)

	// PushSubprocess allows you to track subprocesses within a main process,
	// especially helpful for diagnosing which parts took longest or caused potential deadlocks.
	PushSubprocess(uid string, processName string)

	// WaitingForAllProcessesFinished is a blocking method that halts execution
	// until all registered processes have been completed.
	// It is essential for implementing graceful shutdowns safely.
	WaitingForAllProcessesFinished()
}

type observer struct {
	mu           sync.RWMutex
	allProcesses map[string]*process
}

type process struct {
	name         string
	uuid         string
	insertTime   time.Time
	subprocesses []string
}

func New(ctx context.Context, systemResourceLogging bool) Observer {

	o := &observer{
		allProcesses: make(map[string]*process),
	}

	// start system resource logging only if the systemResourceLogging is true
	if systemResourceLogging {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					stackTrace := debug.Stack()
					slog.Error("caught panic while detecting memory and processor peak", "error", r, "stack", string(stackTrace))
				}
			}()
			o.detectMemoryAndProcessorPeak(ctx)
		}()
	}

	return o
}

func (o *observer) StartProcess(uid string, processName string) {

	o.mu.Lock()
	defer o.mu.Unlock()

	o.allProcesses[uid] = &process{
		name:       processName,
		uuid:       uid,
		insertTime: time.Now(),
	}

}

func (o *observer) EndProcess(uid string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.allProcesses, uid)
}

func (o *observer) PushSubprocess(uid string, processName string) {

	o.mu.Lock()
	defer o.mu.Unlock()

	// if there is no main process we do nothing
	if _, ok := o.allProcesses[uid]; !ok {
		return
	}

	o.allProcesses[uid].subprocesses = append(o.allProcesses[uid].subprocesses, processName)

}

func (o *observer) WaitingForAllProcessesFinished() {
	// waiting for all processes to finish
	for {
		o.mu.RLock()
		if len(o.allProcesses) == 0 {
			slog.Info("all processes finished successfully", "processes", len(o.allProcesses))
			o.mu.RUnlock()
			return
		}

		// log the processes that are still running
		for _, p := range o.allProcesses {
			slog.Debug("waiting for process to finish to shutdown the server....",
				"process", p.name,
				"uuid", p.uuid,
				"subprocesses", strings.Join(p.subprocesses, ", "),
				"elapsedTimeInWaiting", time.Now().Sub(p.insertTime).String())
		}
		o.mu.RUnlock()
		time.Sleep(1 * time.Second)
	}
}

func (o *observer) detectMemoryAndProcessorPeak(ctx context.Context) {

	t := time.NewTicker(10 * time.Second)
	defer t.Stop()

	for {
		select {
		case <-t.C:

			var m runtime.MemStats
			runtime.ReadMemStats(&m)

			if m.Alloc > allocMemoryPeak {
				allocMemoryPeak = m.Alloc
			}

			if runtime.NumGoroutine() > goroutinePeak {
				goroutinePeak = runtime.NumGoroutine()
			}

			percent, err := cpu.Percent(5*time.Second, false)
			if err != nil {
				slog.Error("error while getting cpu percent", "error", err)
				continue
			}

			if len(percent) > 0 && percent[0] > processorLoadPeak {
				processorLoadPeak = math.Round(percent[0]*100) / 100
			}

			o.logMemoryPeak()

		case <-ctx.Done():

			return

		}
	}

}

func (o *observer) logMemoryPeak() {

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// calculate the difference between the memory usage before and after the garbage collector
	var diff uint64
	prefix := "0"
	diff = 0
	if m.Alloc > allocMemoryBefore {
		diff = m.Alloc - allocMemoryBefore
		prefix = "+"
	} else if m.Alloc < allocMemoryBefore {
		diff = allocMemoryBefore - m.Alloc
		prefix = "-"
	}

	if diff < 0 {
		diff = 0
	}

	slog.Info("system resource log",
		slog.String("Act_Alloc", bytesToReadable(m.Alloc)),
		slog.String("Act_Diff", fmt.Sprintf("%s%s", prefix, bytesToReadable(diff))),
		slog.Int("Act_GoRoutines", runtime.NumGoroutine()),
		slog.Int("Peak_Goroutine", goroutinePeak),
		slog.Float64("Peak_ProcessorLoad", processorLoadPeak),
		slog.String("Peak_AllocMemory", bytesToReadable(allocMemoryPeak)),
		slog.Uint64("num_GC", uint64(m.NumGC)),
	)

	allocMemoryBefore = m.Alloc

}

func bytesToReadable(bytes uint64) string {

	const (
		_         = iota // ignore first value by assigning to blank identifier
		KB uint64 = 1 << (10 * iota)
		MB
		GB
		TB
	)

	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.2fTB", float64(bytes)/float64(TB))
	case bytes >= GB:
		return fmt.Sprintf("%.2fGB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.2fMB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.2fKB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}
