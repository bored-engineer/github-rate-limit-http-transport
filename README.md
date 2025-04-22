# GitHub Rate Limit HTTP Transport [![Go Reference](https://pkg.go.dev/badge/github.com/bored-engineer/github-rate-limit-http-transport.svg)](https://pkg.go.dev/github.com/bored-engineer/github-rate-limit-http-transport)
A Golang [http.RoundTripper](https://pkg.go.dev/net/http#RoundTripper) for monitoring the [GitHub rate-limit responses](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api?apiVersion=2022-11-28) from GitHub's REST API.

## Example
Demonstrates how to use [ghratelimit.Transport](https://pkg.go.dev/github.com/bored-engineer/github-rate-limit-http-transport#Transport) with [go-github](github.com/google/go-github) and [Prometheus](github.com/prometheus/client_golang) to monitor GitHub rate limits:
```go
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	ghratelimit "github.com/bored-engineer/github-rate-limit-http-transport"
	"github.com/google/go-github/v71/github"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// Register Prometheus metrics
	RateLimitRemaining = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:      "rate_limit_remaining",
			Help:      "Number of requests remaining in the current rate limit window",
			Subsystem: "github",
		},
		[]string{"resource"},
	)
	RateLimitReset = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:      "rate_limit_reset",
			Help:      "Unix timestamp when the current rate limit window resets",
			Subsystem: "github",
		},
		[]string{"resource"},
	)
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Serve the Prometheus metrics
	go http.ListenAndServe("127.0.0.1:1971", promhttp.Handler())

	// Create a new Transport to observe rate limits
	transport := &ghratelimit.Transport{
		Base: http.DefaultTransport,
		Limits: ghratelimit.Limits{
			Notify: func(resource ghratelimit.Resource, rate *ghratelimit.Rate) {
				RateLimitRemaining.WithLabelValues(resource.String()).Set(float64(rate.Remaining))
				RateLimitReset.WithLabelValues(resource.String()).Set(float64(rate.Reset))
			},
		},
	}

	// The rate-limits will be updated as HTTP responses are received from GitHub by the *ghratelimit.Transport
	// However, it is useful to refresh the rate-limits periodically via the /rate_limits endpoint
	go transport.Poll(ctx, time.Minute, nil)

	// Create a GitHub client using the Transport
	client := github.NewClient(&http.Client{
		Transport: transport,
	})

	// Perform a request to GitHub
	user, _, err := client.Users.Get(ctx, "bored-engineer")
	if err != nil {
		log.Fatalf("(*github.UsersService).Get failed: %v", err)
	}
	fmt.Printf("bio: %s\n", user.GetBio())

	// Demonstrate that the rate-limits can be manually fetched
	rate := transport.Limits.Load(ghratelimit.ResourceCore)
	fmt.Printf("remaining: %d\n", rate.Remaining)
	fmt.Printf("reset: %d\n", rate.Reset)

	// Wait for the context to be cancelled before exiting
	<-ctx.Done()
}
```

Additionally, the [ghratelimit.BalancingTransport](https://pkg.go.dev/github.com/bored-engineer/github-rate-limit-http-transport#BalancingTransport) can be used to automatically balance requests across multiple [ghratelimit.Transport](https://pkg.go.dev/github.com/bored-engineer/github-rate-limit-http-transport#Transport) instances (presumably backed by different GitHub credentials) based on whichever transport has the highest remaining GitHub rate-limit.
