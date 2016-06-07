// Copyright 2016 Stig Bakken (based on the works of Markus Lindenberg)
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
	"strings"
	"syscall"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
)

const (
	namespace = "varnish_request"
)

type str_mapping struct {
	Pattern     *regexp.Regexp
	Replacement string
}

func main() {
	var (
		listenAddress = flag.String("http.port", ":9151", "Host/port for HTTP server")
		metricsPath   = flag.String("http.metricsurl", "/metrics", "Prometheus metrics path")
		httpHost      = flag.String("varnish.host", "", "Virtual host to look for in Varnish logs (defaults to all hosts)")
		mappings      = flag.String("varnish.path-mappings", "", "Path mappings formatted like this: 'regexp->replace regex2->replace2'")
		instance      = flag.String("varnish.instance", "", "Name of Varnish instance")
	)
	flag.Parse()

	// Listen to signals
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, syscall.SIGTERM, syscall.SIGINT)

	// Set up 'varnishncsa' pipe
	cmdName := "varnishncsa"
	cmdArgs := buildVarnishncsaArgs(*httpHost, *instance)
	log.Infof("Running command: %v %v\n", cmdName, cmdArgs)
	cmd := exec.Command(cmdName, cmdArgs...)
	cmdReader, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatal(err)
	}
	scanner := bufio.NewScanner(cmdReader)

	path_mappings, err := parseMappings(*mappings)
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
			metrics, labels, err := parseMessage(content, path_mappings)
			if err != nil {
				log.Error(err)
				continue
			}
			for _, metric := range metrics {
				var collector prometheus.Collector
				collector, err = prometheus.RegisterOrGet(prometheus.NewHistogramVec(prometheus.HistogramOpts{
					Namespace: namespace,
					Name:      metric.Name,
					Help:      fmt.Sprintf("Varnish request log value for %s", metric.Name),
				}, labels.Names))
				if err != nil {
					log.Error(err)
					continue
				}
				collector.(*prometheus.HistogramVec).WithLabelValues(labels.Values...).Observe(metric.Value)
			}
		}
	}()

	// Setup HTTP server
	http.Handle(*metricsPath, prometheus.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
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

	s := <-sigchan
	log.Infof("Received %v, terminating", s)
	log.Infof("Messages received: %d", msgs)

	os.Exit(0)
}

func parseMappings(input string) (mappings []str_mapping, err error) {
	mappings = make([]str_mapping, 0)
	str_mappings := strings.Split(input, " ")
	for i := range str_mappings {
		onemapping := str_mappings[i]
		if len(onemapping) == 0 {
			continue
		}
		parts := strings.Split(onemapping, "->")
		if len(parts) != 2 {
			err = fmt.Errorf("URL mapping must have two elements separated by \"->\", got \"%s\"", onemapping)
			return
		}
		mappings = append(mappings, str_mapping{regexp.MustCompile(parts[0]), parts[1]})
	}
	return
}

func buildVarnishncsaArgs(httpHost string, instance string) (args []string) {
	args = make([]string, 0, 6)
	args = append(args, "-F")
	if len(httpHost) == 0 {
		args = append(args, "time:%D method=\"%m\" status=%s path=\"%U\" host=\"%{host}i\"")
	} else {
		args = append(args, "time:%D method=\"%m\" status=%s path=\"%U\"")
		args = append(args, "-q")
		args = append(args, "ReqHeader:host eq \""+httpHost+"\"")
	}
	if instance != "" {
		args = append(args, "-n", instance)
	}
	return
}
