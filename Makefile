.PHONY: serve
serve:
	portman exec --name dashboard --expose -- go run . serve --addr :{}
