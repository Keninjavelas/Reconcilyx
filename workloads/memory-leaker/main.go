package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	allocatedBytes = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "reconcilyx_memory_allocated_bytes",
		Help: "The total amount of memory currently simulated as allocated",
	})
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.Handle("/metrics", promhttp.Handler())

	http.HandleFunc("/leak", func(w http.ResponseWriter, r *http.Request) {
		allocatedBytes.Add(1024 * 1024) // Simulate 1MB leak
		fmt.Fprintf(w, "Memory leak simulated")
	})

	fmt.Printf("Memory Leaker starting on port %s\n", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		fmt.Printf("Server failed: %v\n", err)
	}
}
