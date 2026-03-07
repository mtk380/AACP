package main

import (
	"flag"
	"log"
	"time"

	"aacp/internal/p2p"
)

func main() {
	var (
		moniker  = flag.String("moniker", "relay-0", "relay moniker")
		interval = flag.Duration("announce-interval", 15*time.Second, "announce interval")
	)
	flag.Parse()

	limiter := p2p.NewRateLimiter()
	log.Printf("relay %s started with default relay limit %+v", *moniker, limiter.LimitFor("/aacp/relay/1.0.0"))
	t := time.NewTicker(*interval)
	defer t.Stop()
	for ts := range t.C {
		log.Printf("relay %s heartbeat %s", *moniker, ts.Format(time.RFC3339))
	}
}
