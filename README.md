# Varnish Request Exporter for Prometheus

This is a [Prometheus](https://prometheus.io/) exporter for [Varnish](https://varnish-cache.org/) requests. 

In contrast to existing exporters varnishncsa_exporter does *not* scrape [varnishstat output](https://www.varnish-cache.org/docs/trunk/reference/varnishstat.html) but records statistics for HTTP requests.

## Installation

```
go get github.com/stigsb/varnishncsa_exporter
```

## Configuration

All configuration is done with command-line parameters:

```
Usage of varnishncsa_exporter:
  -http.metricsurl string
        Prometheus metrics path (default "/metrics")
  -http.port string
        Host/port for HTTP server (default ":9169")
  -log.format value
        If set use a syslog logger or JSON logging. Example: logger:syslog?appname=bob&local=7 or logger:stdout?json=true. Defaults to stderr.
  -log.level value
        Only log messages with the given severity or above. Valid levels: [debug, info, warn, error, fatal]. (default info)
  -varnish.host string
        Virtual host to look for in Varnish logs (defaults to all hosts)
  -varnish.path-mappings string
        Path mappings formatted like this: 'regexp->replace regex2->replace2'
  -varnish.instance string
        Name of Varnish instance. Defaults to hostname.
```



## Log format

The `varnishncsa` format being used is `time:%D method="%m" status=%s path="%U" host="%{host}i"` if the `--varnish.host` flag is not specified, or
`time:%D method="%m" status=%s path="%U"` if `--varnish.host` is specified.

The Prometheus metrics exported are:

`varnish_request_exporter_log_messages` - the number of varnishncsa log messages processed

`varnish_request_exporter_log_parse_failure` - the number of parse errors from varnishncsa output

`varnish_request_time` - histogram of request processing time in seconds, with the following labels:
 * `method` - HTTP request method
 * `status` - HTTP status code
 * `path` - HTTP request URI (normalized using [path mappings](#path-mappings), without query string)
 * `host` - HTTP Host: header (only when `--varnish.host` is not specified)
 
## Path Mappings

If your URLs (not query string) contain request parameters, you will get a lot of noise in the `path` label.  You can
normalize the paths by using the `--varnish.path-mappings` flag.

This example changes normalizes paths in the following way:
* replace _/number/_ with _/ID/_
* remove numbers at the end of the path
* remove tailing slashes
* remove duplicated slashes

```
varnishncsa_exporter --varnish.path-mappings '/\d+/->/ID/ \d+$-> /$-> //->'
```

## Attributions

Thanks to Markus Lindenberg for the [nginx_request_exporter](https://github.com/markuslindenberg/nginx_request_exporter),
from which we have borrowed some code.
