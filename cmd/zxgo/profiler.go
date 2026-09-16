package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"time"
)

// Profiler manages CPU, memory, and execution trace profiling.
type Profiler struct {
	cpuFile   *os.File
	memFile   *os.File
	traceFile *os.File
	startTime time.Time
}

// StartProfiling initializes all profiling outputs.
func StartProfiling() (*Profiler, error) {
	p := &Profiler{
		startTime: time.Now(),
	}

	// CPU profile
	cpuFile, err := os.Create("cpu.prof")
	if err != nil {
		return nil, fmt.Errorf("creating CPU profile: %w", err)
	}
	p.cpuFile = cpuFile

	if err := pprof.StartCPUProfile(cpuFile); err != nil {
		cpuFile.Close()
		return nil, fmt.Errorf("starting CPU profile: %w", err)
	}

	// Memory profile placeholder (written on stop)
	memFile, err := os.Create("mem.prof")
	if err != nil {
		pprof.StopCPUProfile()
		cpuFile.Close()
		return nil, fmt.Errorf("creating memory profile: %w", err)
	}
	p.memFile = memFile

	// Execution trace
	traceFile, err := os.Create("trace.out")
	if err != nil {
		pprof.StopCPUProfile()
		cpuFile.Close()
		memFile.Close()
		return nil, fmt.Errorf("creating trace file: %w", err)
	}
	p.traceFile = traceFile

	if err := trace.Start(traceFile); err != nil {
		pprof.StopCPUProfile()
		cpuFile.Close()
		memFile.Close()
		traceFile.Close()
		return nil, fmt.Errorf("starting trace: %w", err)
	}

	fmt.Println("Profiling started:")
	fmt.Println("  CPU profile:    cpu.prof")
	fmt.Println("  Memory profile: mem.prof")
	fmt.Println("  Trace:          trace.out")
	fmt.Println("  Goroutines:     goroutine.prof")
	fmt.Println("  Heap:           heap.prof")
	fmt.Println("  Allocs:         allocs.prof")
	fmt.Println("  Block:          block.prof")
	fmt.Println("  Mutex:          mutex.prof")

	return p, nil
}

// StopProfiling writes all profiling data to disk.
func (p *Profiler) StopProfiling() error {
	elapsed := time.Since(p.startTime)

	// Stop CPU profiling
	pprof.StopCPUProfile()
	p.cpuFile.Close()

	// Stop execution trace
	trace.Stop()
	p.traceFile.Close()

	// Write memory profile
	runtime.GC() // Force GC to get accurate memory stats
	if err := pprof.WriteHeapProfile(p.memFile); err != nil {
		return fmt.Errorf("writing memory profile: %w", err)
	}
	p.memFile.Close()

	// Write goroutine profile
	goroutineFile, err := os.Create("goroutine.prof")
	if err != nil {
		return fmt.Errorf("creating goroutine profile: %w", err)
	}
	defer goroutineFile.Close()
	if err := pprof.Lookup("goroutine").WriteTo(goroutineFile, 0); err != nil {
		return fmt.Errorf("writing goroutine profile: %w", err)
	}

	// Write heap profile (detailed memory allocations)
	heapFile, err := os.Create("heap.prof")
	if err != nil {
		return fmt.Errorf("creating heap profile: %w", err)
	}
	defer heapFile.Close()
	if err := pprof.Lookup("heap").WriteTo(heapFile, 0); err != nil {
		return fmt.Errorf("writing heap profile: %w", err)
	}

	// Write allocs profile (all allocations, not just live)
	allocsFile, err := os.Create("allocs.prof")
	if err != nil {
		return fmt.Errorf("creating allocs profile: %w", err)
	}
	defer allocsFile.Close()
	if err := pprof.Lookup("allocs").WriteTo(allocsFile, 0); err != nil {
		return fmt.Errorf("writing allocs profile: %w", err)
	}

	// Write block profile (blocking operations)
	runtime.SetBlockProfileRate(1)
	blockFile, err := os.Create("block.prof")
	if err != nil {
		return fmt.Errorf("creating block profile: %w", err)
	}
	defer blockFile.Close()
	if err := pprof.Lookup("block").WriteTo(blockFile, 0); err != nil {
		return fmt.Errorf("writing block profile: %w", err)
	}

	// Write mutex profile (lock contention)
	runtime.SetMutexProfileFraction(1)
	mutexFile, err := os.Create("mutex.prof")
	if err != nil {
		return fmt.Errorf("creating mutex profile: %w", err)
	}
	defer mutexFile.Close()
	if err := pprof.Lookup("mutex").WriteTo(mutexFile, 0); err != nil {
		return fmt.Errorf("writing mutex profile: %w", err)
	}

	fmt.Printf("\nProfiling stopped (duration: %v)\n", elapsed.Round(time.Millisecond))
	fmt.Println("\nAnalyze with:")
	fmt.Println("  go tool pprof cpu.prof")
	fmt.Println("  go tool pprof mem.prof")
	fmt.Println("  go tool pprof heap.prof")
	fmt.Println("  go tool pprof allocs.prof")
	fmt.Println("  go tool pprof goroutine.prof")
	fmt.Println("  go tool pprof block.prof")
	fmt.Println("  go tool pprof mutex.prof")
	fmt.Println("  go tool trace trace.out")

	return nil
}
