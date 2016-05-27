# Varnish Request Exporter for Prometheus

This is a [Prometheus](https://prometheus.io/) exporter for [Varnish](https://varnish-cache.org/) requests. 

In contrast to existing exporters varnishncsa_exporter does *not* scrape [varnishstat output](https://www.varnish-cache.org/docs/trunk/reference/varnishstat.html) but records statistics for HTTP requests.

This tools is based on code from Markus Lindenberg's
[nginx_request_exporter](https://github.com/markuslindenberg/nginx_request_exporter).

## Installation

```
go get github.com/stigsb/varnishncsa_exporter
```

## Configuration



## Log format

