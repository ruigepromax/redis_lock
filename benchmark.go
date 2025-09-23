package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/redis/go-redis/v9"
	"log"
	"sort"
	"sync"
	"time"
)

// =============================================================================
// Benchmark code starts here.
// =============================================================================

// result structure for a single lock/unlock operation.
type result struct {
	err      error
	duration time.Duration
}

func main() {
	// 1. Define command-line flags.
	goroutines := flag.Int("goroutines", 50, "Number of concurrent goroutines")
	totalOps := flag.Int("ops", 2000, "Total number of lock/unlock operations")
	redisAddr := flag.String("addr", "localhost:6379", "Redis server address")
	lockName := flag.String("lock-name", "my-redis-bench-lock", "Name of the distributed lock")
	flag.Parse()

	if *goroutines <= 0 || *totalOps <= 0 {
		log.Fatal("Number of goroutines and operations must be positive")
	}

	// 2. Initialize Redis client.
	rdb := redis.NewClient(&redis.Options{
		Addr: *redisAddr,
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer rdb.Close()

	// Use our lock client wrapper
	client := NewClient(rdb)

	// 3. Prepare for the benchmark.
	var wg sync.WaitGroup
	results := make(chan result, *totalOps)
	opsPerGoroutine := *totalOps / *goroutines

	fmt.Printf("Starting benchmark with:\n")
	fmt.Printf("  Goroutines: %d\n", *goroutines)
	fmt.Printf("  Total Operations: %d\n", *totalOps)
	fmt.Printf("  Redis Address: %s\n", *redisAddr)
	fmt.Println("-----------------------------------------")

	startTime := time.Now()

	// 4. Start concurrent worker goroutines.
	for i := 0; i < *goroutines; i++ {
		wg.Add(1)
		go worker(client, *lockName, opsPerGoroutine, &wg, results)
	}

	// 5. Wait for all workers to finish and close the results channel.
	wg.Wait()
	close(results)

	// 6. Process results and print the performance report.
	processResults(results, startTime, *totalOps)
}

// worker is the function executed by each goroutine.
func worker(client *Client, lockName string, opsCount int, wg *sync.WaitGroup, results chan<- result) {
	defer wg.Done()

	for i := 0; i < opsCount; i++ {
		opStartTime := time.Now()

		lock := client.NewLock(lockName)

		// Context with a timeout for acquiring the lock.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		err := lock.Lock(ctx)
		if err != nil {
			results <- result{err: fmt.Errorf("failed to acquire lock: %w", err), duration: 0}
			cancel()
			continue
		}

		// --- Simulate some work (optional) ---
		// time.Sleep(5 * time.Millisecond)

		err = lock.Unlock(ctx)
		if err != nil {
			results <- result{err: fmt.Errorf("failed to unlock: %w", err), duration: 0}
		} else {
			results <- result{err: nil, duration: time.Since(opStartTime)}
		}

		cancel()
	}
}

// processResults is identical to the one from the etcd benchmark.
func processResults(results <-chan result, startTime time.Time, totalOps int) {
	totalDuration := time.Since(startTime)
	var durations []time.Duration
	var errors int

	for r := range results {
		if r.err != nil {
			// Don't flood the log, just count them for the report
			errors++
		} else {
			durations = append(durations, r.duration)
		}
	}

	successOps := len(durations)
	if successOps == 0 {
		fmt.Println("\nNo successful operations to report.")
		return
	}

	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })

	throughput := float64(successOps) / totalDuration.Seconds()

	var totalLatency time.Duration
	for _, d := range durations {
		totalLatency += d
	}
	avgLatency := totalLatency / time.Duration(successOps)

	minLatency := durations[0]
	maxLatency := durations[len(durations)-1]
	p50Latency := durations[int(float64(successOps)*0.5)]
	p90Latency := durations[int(float64(successOps)*0.9)]
	p99Latency := durations[int(float64(successOps)*0.99)]

	fmt.Println("\n--- Benchmark Report ---")
	fmt.Printf("Total Time: %.2f s\n", totalDuration.Seconds())
	fmt.Printf("Total Operations: %d\n", totalOps)
	fmt.Printf("Successful Operations: %d\n", successOps)
	fmt.Printf("Failed Operations: %d\n", errors)
	fmt.Println("-----------------------------------------")
	fmt.Printf("Throughput (ops/sec): %.2f\n", throughput)
	fmt.Println("-----------------------------------------")
	fmt.Println("Latency per Lock/Unlock cycle:")
	fmt.Printf("  Average: %s\n", avgLatency)
	fmt.Printf("  Min:     %s\n", minLatency)
	fmt.Printf("  Max:     %s\n", maxLatency)
	fmt.Printf("  P50 (Median): %s\n", p50Latency)
	fmt.Printf("  P90:     %s\n", p90Latency)
	fmt.Printf("  P99:     %s\n", p99Latency)
	fmt.Println("-----------------------------------------")
}
