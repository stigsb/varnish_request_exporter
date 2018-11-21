PROGRAMS=varnish-request-exporter
all: $(PROGRAMS)

varnish-request-exporter: $(shell echo *.go)
	go build -o $@ $^

clean:
	rm -f $(PROGRAMS)

