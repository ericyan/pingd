package main

import (
	"bufio"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/ericyan/ping"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	bind     = flag.String("bind", "0.0.0.0", "interface to bind")
	port     = flag.Int("port", 9344, "port to listen on for HTTP requests")
	interval = flag.Int("interval", 3, "seconds to wait between sending each packet")
	dstList  = flag.String("list", "./dst.list", "path to destination list")
)

var (
	rttHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ping_rtt_seconds",
			Help:    "Ping round-trip time in seconds.",
			Buckets: []float64{0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.2, 0.3, 0.5, 1},
		},
		[]string{"src", "dst"},
	)
	totalRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ping_requests_total",
			Help: "Total number of ping requests sent.",
		},
		[]string{"src", "dst"},
	)
)

func init() {
	prometheus.MustRegister(rttHistogram)
	prometheus.MustRegister(totalRequests)
}

func main() {
	flag.Parse()

	f, err := os.Open(*dstList)
	defer f.Close()
	if err != nil {
		log.Fatalln(err)
	}

	var dsts []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		dsts = append(dsts, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		log.Fatalln(err)
	}

	pinger, err := ping.NewPinger()
	if err != nil {
		log.Fatalln(err)
	}
	defer pinger.Close()

	for _, dst := range dsts {
		ip := net.ParseIP(dst)
		if ip == nil {
			log.Fatalln("invlid destination IP:", dst)
		}

		go func(ip net.IP) {
			for range time.Tick(time.Duration(*interval) * time.Second) {
				rtt, err := pinger.Ping(&net.IPAddr{IP: ip})
				if err == nil {
					rttHistogram.With(prometheus.Labels{"src": *bind, "dst": ip.String()}).Observe(rtt / 1000)
				}

				totalRequests.With(prometheus.Labels{"src": *bind, "dst": ip.String()}).Inc()
			}
		}(ip)
	}

	http.Handle("/metrics", promhttp.Handler())

	log.Printf("Serving metrics at http://%s:%s/metrics", *bind, strconv.Itoa(*port))
	log.Fatal(http.ListenAndServe(*bind+":"+strconv.Itoa(*port), nil))
}
