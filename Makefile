SERVE_PIDFILE := .portman-serve.pid

.PHONY: serve
serve: serve-stop
	@nohup portman exec --name dashboard --expose -- go run . serve --addr :{} > /tmp/portman-serve.log 2>&1 & echo $$! > $(SERVE_PIDFILE)
	@echo "portman serve started (PID $$(cat $(SERVE_PIDFILE))), log: /tmp/portman-serve.log"

.PHONY: serve-stop
serve-stop:
	@if [ -f $(SERVE_PIDFILE) ] && kill -0 $$(cat $(SERVE_PIDFILE)) 2>/dev/null; then \
		echo "Stopping old serve process (PID $$(cat $(SERVE_PIDFILE)))..."; \
		kill $$(cat $(SERVE_PIDFILE)); \
		sleep 1; \
	fi
	@rm -f $(SERVE_PIDFILE)
