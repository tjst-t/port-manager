BIN := .portman-local
SERVE_PIDFILE := .portman-serve.pid
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/tjst-t/port-manager/cmd.Version=$(VERSION)

.PHONY: serve
serve: serve-stop
	@go build -ldflags "$(LDFLAGS)" -o $(BIN) .
	@PORTMAN_NO_DASHBOARD=1 nohup ./$(BIN) exec --name dashboard --expose -- ./$(BIN) serve --addr :{} > /tmp/portman-serve.log 2>&1 & echo $$! > $(SERVE_PIDFILE)
	@sleep 1
	@echo "portman serve started (PID $$(cat $(SERVE_PIDFILE))), log: /tmp/portman-serve.log"

.PHONY: serve-stop
serve-stop:
	@if [ -f $(SERVE_PIDFILE) ] && kill -0 $$(cat $(SERVE_PIDFILE)) 2>/dev/null; then \
		echo "Stopping old serve process..."; \
		kill $$(cat $(SERVE_PIDFILE)) 2>/dev/null || true; \
		sleep 2; \
		kill -9 $$(cat $(SERVE_PIDFILE)) 2>/dev/null || true; \
	fi
	@rm -f $(SERVE_PIDFILE)
