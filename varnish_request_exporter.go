// Copyright 2016 Markus Lindenberg, Stig Bakken
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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
)

const (
	namespace = "varnish_request"
)

type path_mappings struct {
	Pattern     *regexp.Regexp
	Replacement string
}

func main() {
	var (
		listenAddress = flag.String("http.port", ":9151", "Host/port for HTTP server")
		metricsPath   = flag.String("http.metricsurl", "/metrics", "Prometheus metrics path")
		httpHost      = flag.String("varnish.host", "", "Virtual host to look for in Varnish logs (defaults to all hosts)")
		mappingsFile  = flag.String("varnish.path-mappings", "", "Name of file with path mappings")
		instance      = flag.String("varnish.instance", "", "Name of Varnish instance")
		befirstbyte   = flag.Bool("varnish.firstbyte", false, "Also export metrics for backend time to first byte")
		user_query    = flag.String("varnish.query", "", "VSL query override (defaults to one that is generated")
		sizes         = flag.Bool("varnish.sizes", false, "Also export metrics for response size")
	)
	flag.Parse()

	// Listen to signals
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, syscall.SIGTERM, syscall.SIGINT)

	// Set up 'varnishncsa' pipe
	cmdName := "varnishncsa"
	vslQuery := buildVslQuery(*httpHost, *user_query)
	varnishFormat := buildVarnishncsaFormat(*befirstbyte, *sizes, *user_query)
	cmdArgs := buildVarnishncsaArgs(vslQuery, *instance, varnishFormat)
	log.Infof("Running command: %v %v\n", cmdName, cmdArgs)
	cmd := exec.Command(cmdName, cmdArgs...)
	cmdReader, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatal(err)
	}
	scanner := bufio.NewScanner(cmdReader)

	path_mappings, err := parseMappings(*mappingsFile)
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

func parseMappings(mappings_file string) (mappings []path_mappings, err error) {
	mappings = make([]path_mappings, 0)
	if mappings_file == "" {
		return
	}
	in_file, err := os.Open(mappings_file)
	if err != nil {
		log.Fatal(err)
	}
	scanner := bufio.NewScanner(in_file)
	scanner.Split(bufio.ScanLines)
	comment_re := regexp.MustCompile("(#.*|^\\s+|\\s+$)")
	split_re := regexp.MustCompile("\\s+")
	line_no := 0
	for scanner.Scan() {
		line_no++
		line := comment_re.ReplaceAllString(scanner.Text(), "")
		if line == "" {
			continue
		}
		parts := split_re.Split(line, 2)
		switch len(parts) {
		case 1:
			log.Debugf("mapping strip: %s", parts[0])
			mappings = append(mappings, path_mappings{regexp.MustCompile(parts[0]), ""})
		case 2:
			log.Debugf("mapping replace: %s => %s", parts[0], parts[1])
			mappings = append(mappings, path_mappings{regexp.MustCompile(parts[0]), parts[1]})
		}
	}
	in_file.Close()
	return
}

func buildVslQuery(httpHost string, user_query string) (query string) {
	query = user_query
	if httpHost != "" {
		if query != "" {
			query += " and "
		}
		query += "ReqHeader:host eq \"" + httpHost + "\""
	}
	return
}

func buildVarnishncsaFormat(befirstbyte bool, sizes bool, user_format string) (format string) {
	if user_format != "" {
		format = user_format + " "
	}
	format += "method=\"%m\" status=%s path=\"%U\" cache=\"%{Varnish:hitmiss}x\" host=\"%{host}i\" time:%D"
	if befirstbyte {
		format += " time_firstbyte:%{Varnish:time_firstbyte}x"
	}
	if sizes {
		format += " respsize:%b"
	}
	return
}

func buildVarnishncsaArgs(vsl_query string, instance string, format string) (args []string) {
	args = make([]string, 0, 6)
	args = append(args, "-F")
	args = append(args, format)
	if vsl_query != "" {
		args = append(args, "-q")
		args = append(args, vsl_query)
	}
	if instance != "" {
		args = append(args, "-n", instance)
	}
	return
}
