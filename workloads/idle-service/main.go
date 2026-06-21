package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	uptimeSeconds = promauto.NewCounter(prometheus.CounterOpts{
		Name: "reconcilyx_uptime_seconds_total",
		Help: "The total uptime in seconds",
	})
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	go func() {
		for {
			time.Sleep(1 * time.Second)
			uptimeSeconds.Inc()
		}
	}()

	http.Handle("/metrics", promhttp.Handler())

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Idle service running")
	})

	fmt.Printf("Idle Service starting on port %s\n", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		fmt.Printf("Server failed: %v\n", err)
	}
}
