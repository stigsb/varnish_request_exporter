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
Usage of varnish_request_exporter:
  -http.metricsurl string
    	Prometheus metrics path (default "/metrics")
  -http.port string
    	Host/port for HTTP server (default ":9151")
  -log.format value
    	If set use a syslog logger or JSON logging. Example: logger:syslog?appname=bob&local=7 or logger:stdout?json=true. Defaults to stderr.
  -log.level value
    	Only log messages with the given severity or above. Valid levels: [debug, info, warn, error, fatal]. (default info)
  -varnish.firstbyte
    	Also export metrics for backend time to first byte
  -varnish.host string
    	Virtual host to look for in Varnish logs (defaults to all hosts)
  -varnish.instance string
    	Name of Varnish instance
  -varnish.path-mappings string
    	Name of file with path mappings
  -varnish.query string
    	VSL query override (defaults to one that is generated
  -varnish.sizes
    	Also export metrics for response size
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

If your URLs (not query string) contain request parameters, you will
get a lot of noise in the `path` label, causing unneccessay load on
Prometheus, as well as making it harder to run queries by path.  You
can normalize the paths by pointing to a mapping file with the
`--varnish.path-mappings` flag.

The mappings file is a very simple config file with one or two
whitespace-separated columns; the regexp and replacement string. If
the replacement is missing, the regexp will be removed from the path.
Use # for comments.

This is an example path mappings file:
* replace _/number/_ with _/ID/_
* remove numbers at the end of the path
* remove tailing slashes
* remove duplicated slashes

```
# normalize /number to /ID
/\d+             /ID

# make two or more slashes into just one
//+              /

# example of using group matches
/([^\.]+?)\.php  /$1

# remove trailing slash
/$
```

## Attributions

Thanks to Markus Lindenberg for the [nginx_request_exporter](https://github.com/markuslindenberg/nginx_request_exporter),
from which we have borrowed some code.
