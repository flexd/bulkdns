package main

import (
	"bufio"
	"flag"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/beefsack/go-rate"
)

// Input flags
var inputFile string
var rateDuration time.Duration
var statusDuration time.Duration
var rateLimit int
var verbose bool

// Other stuff
var wg sync.WaitGroup
var rl *rate.RateLimiter

// Guards queryCount
var qcMutex sync.RWMutex
var queryCount = 0.0

// Guards startTime
var stMutex sync.RWMutex

func init() {
	flag.StringVar(&inputFile, "input", "source.txt", "File with IPs, \"source\".txt by default")
	flag.DurationVar(&rateDuration, "rateduration", 1*time.Second, "Rate limit duration, e.g 1s")
	flag.DurationVar(&statusDuration, "statusduration", 10*time.Second, "Time between status messages, e.g 10s")
	flag.IntVar(&rateLimit, "ratelimit", 100, "Rate limit per duration")
	flag.BoolVar(&verbose, "verbose", false, "Verbose logging to stdout?")
}
func main() {
	flag.Parse()
	logger := log.New(os.Stdout, "", log.LstdFlags)
	logger.Println("[*] bulkdns firing up!")
	stMutex.Lock()
	startTime := time.Now()
	stMutex.Unlock()
	go func() {
		for {
			qcMutex.RLock()
			qc := queryCount
			qcMutex.RUnlock()

			stMutex.RLock()
			duration := time.Since(startTime)
			stMutex.RUnlock()

			qps := qc / duration.Seconds()
			logger.Println("[+] Processed", qc, "in", duration, "seconds.", qps, "queries per second.")
			time.Sleep(statusDuration)
		}
	}()
	logFile, err := os.OpenFile("lookup.log", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal(err)
	}
	defer logFile.Close()
	// Log to Stdout and file if verbose, otherwise just to file
	if verbose {
		log.SetOutput(io.MultiWriter(os.Stdout, logFile))
	} else {
		log.SetOutput(logFile)
	}
	rl = rate.New(rateLimit, rateDuration)

	logger.Printf("[*] Source file %v\n", inputFile)
	file, err := os.Open(inputFile) // For read access.
	if err != nil {
		log.Fatal(err)
	}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		// Wait for rate limit
		rl.Wait()
		wg.Add(1)
		qcMutex.Lock()
		queryCount += 1
		qcMutex.Unlock()
		go lookupDomain(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		// Die fatally if we can't read the file.
		log.Fatal("[-] Error reading standard input:", err)
	}
	// Wait for everything to finish
	wg.Wait()
	qcMutex.RLock()
	qc := queryCount
	qcMutex.RUnlock()

	stMutex.RLock()
	duration := time.Since(startTime)
	stMutex.RUnlock()

	qps := qc / duration.Seconds()
	logger.Println("[=] Finished in", duration, "seconds.", int(qps), "queries per second.")
}
func lookupDomain(domain string) {
	ips, err := net.LookupHost(domain)
	if err != nil {
		log.Println(err)
	}
	log.Println(domain, ips)
	wg.Done()
}
