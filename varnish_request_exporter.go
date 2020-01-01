// Copyright 2016-2020 Markus Lindenberg, Stig Bakken
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bufio"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"syscall"

	"github.com/facebookgo/pidfile"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/log"
)

const (
	namespace = "varnish_request"
)
var (
	listenAddress = flag.String("http.port", ":9151", "Host/port for HTTP server")
	metricsPath   = flag.String("http.metricsurl", "/metrics", "Prometheus metrics path")
	httpHost      = flag.String("varnish.host", "", "Virtual host to look for in Varnish logs (defaults to all hosts)")
	mappingsFile  = flag.String("varnish.path-mappings", "", "Name of file with path mappings")
	instance      = flag.String("varnish.instance", "", "Name of Varnish instance")
	beFirstByte   = flag.Bool("varnish.firstbyte", false, "Also export metrics for backend time to first byte")
	userQuery     = flag.String("varnish.query", "", "VSL query override (defaults to one that is generated")
	sizes         = flag.Bool("varnish.sizes", false, "Also export metrics for response size")
)

type pathMapping struct {
	Pattern     *regexp.Regexp
	Replacement string
}

func main() {
	flag.Parse()

	// Listen to signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	err := pidfile.Write()
	if pidfile.IsNotConfigured(err) {
		log.Info("pidfile not configured")
	} else if err != nil {
		log.Fatal(err)
	}

	// Set up 'varnishncsa' pipe
	cmdName := "varnishncsa"
	vslQuery := buildVslQuery()
	varnishFormat := buildVarnishNCSAFormat()
	cmdArgs := buildVarnishNCSAArgs(vslQuery, varnishFormat)
	log.Infof("Running command: %v %v\n", cmdName, cmdArgs)
	cmd := exec.Command(cmdName, cmdArgs...)
	cmdReader, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatal(err)
	}
	scanner := bufio.NewScanner(cmdReader)

	pathMappings, err := parseMappings(*mappingsFile)
	if err != nil {
		log.Fatal(err)
	}

	// Setup metrics
	varnishMessages := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "exporter_log_messages",
		Help:      "Current total log messages received.",
	})
	err = prometheus.Register(varnishMessages)
	if err != nil {
		log.Fatal(err)
	}
	varnishParseFailures := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "exporter_log_parse_failure",
		Help:      "Number of errors while parsing log messages.",
	})
	err = prometheus.Register(varnishParseFailures)
	if err != nil {
		log.Fatal(err)
	}
	var msgs int64

	go func() {
		for scanner.Scan() {
			varnishMessages.Inc()
			content := scanner.Text()
			msgs++
			metrics, labels, err := parseMessage(content, pathMappings)
			if err != nil {
				log.Error(err)
				continue
			}
			for _, metric := range metrics {
				var collector prometheus.Collector
				//collector, err = prometheus.RegisterOrGet(prometheus.NewHistogramVec(prometheus.HistogramOpts{
				collector = prometheus.NewHistogramVec(prometheus.HistogramOpts{
					Namespace: namespace,
					Name:      metric.Name,
					Help:      fmt.Sprintf("Varnish request log value for %s", metric.Name),
				}, labels.Names)
				err := prometheus.Register(collector)
				if err != nil {
					if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
						collector = are.ExistingCollector.(*prometheus.HistogramVec)
					} else {
						log.Error(err)
						continue
					}
				}
				collector.(*prometheus.HistogramVec).WithLabelValues(labels.Values...).Observe(metric.Value)
			}
		}
	}()

	// Setup HTTP server
	http.Handle(*metricsPath, promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html>
             <head><title>Varnish Request Exporter</title></head>
             <body>
             <h1>Varnish Request Exporter</h1>
             <p><a href='` + *metricsPath + `'>Metrics</a></p>
             </body>
             </html>`))
	})

	go func() {
		log.Infof("Starting Server: %s", *listenAddress)
		log.Fatal(http.ListenAndServe(*listenAddress, nil))
	}()

	go func() {
		err = cmd.Start()
		if err != nil {
			log.Fatal(err)
		}
		err = cmd.Wait()
		if err != nil {
			log.Fatal(err)
		}
		log.Infof("varnishncsa command exited")
		log.Infof("Messages received: %d", msgs)
		os.Exit(0)
	}()

	s := <-sigChan
	log.Infof("Received %v, terminating", s)
	log.Infof("Messages received: %d", msgs)

	os.Exit(0)
}

func parseMappings(mappingsFile string) (mappings []pathMapping, err error) {
	mappings = make([]pathMapping, 0)
	if mappingsFile == "" {
		return
	}
	inFile, err := os.Open(mappingsFile)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = inFile.Close() }()
	scanner := bufio.NewScanner(inFile)
	scanner.Split(bufio.ScanLines)
	commentRegexp := regexp.MustCompile("(#.*|^\\s+|\\s+$)")
	splitRegexp := regexp.MustCompile("\\s+")
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := commentRegexp.ReplaceAllString(scanner.Text(), "")
		if line == "" {
			continue
		}
		parts := splitRegexp.Split(line, 2)
		switch len(parts) {
		case 1:
			log.Debugf("mapping strip: %s", parts[0])
			mappings = append(mappings, pathMapping{regexp.MustCompile(parts[0]), ""})
		case 2:
			log.Debugf("mapping replace: %s => %s", parts[0], parts[1])
			mappings = append(mappings, pathMapping{regexp.MustCompile(parts[0]), parts[1]})
		}
	}
	return
}

func buildVslQuery() string {
	query := *userQuery
	if *httpHost != "" {
		if query != "" {
			query += " and "
		}
		query += "ReqHeader:host eq \"" + *httpHost + "\""
	}
	return query
}

func buildVarnishNCSAFormat() string {
	format := "method=\"%m\" status=%s path=\"%U\" cache=\"%{Varnish:hitmiss}x\" host=\"%{host}i\" time:%D"
	if *beFirstByte {
		format += " time_firstbyte:%{Varnish:time_firstbyte}x"
	}
	if *sizes {
		format += " respsize:%b"
	}
	return format
}

func buildVarnishNCSAArgs(vslQuery string, format string) []string {
	args := make([]string, 0)
	args = append(args, "-F", format)
	if vslQuery != "" {
		args = append(args, "-q", vslQuery)
	}
	if *instance != "" {
		args = append(args, "-n", *instance)
	}
	return args
}
