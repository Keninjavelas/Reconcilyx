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
	burnEvents = promauto.NewCounter(prometheus.CounterOpts{
		Name: "reconcilyx_cpu_burn_events_total",
		Help: "The total number of CPU burn events simulated",
	})
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.Handle("/metrics", promhttp.Handler())

	http.HandleFunc("/burn", func(w http.ResponseWriter, r *http.Request) {
		burnEvents.Inc()
		fmt.Fprintf(w, "CPU burning simulated")
	})

	fmt.Printf("CPU Burner starting on port %s\n", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		fmt.Printf("Server failed: %v\n", err)
	}
}
