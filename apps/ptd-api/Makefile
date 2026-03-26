REPO_ROOT := $(shell git rev-parse --show-toplevel)
EMBED_DIR := cmd/ptd-api/_embed

.PHONY: embed-sync embed-web build build-windows test clean

embed-sync:
	rm -rf $(EMBED_DIR)
	mkdir -p $(EMBED_DIR)
	printf 'placeholder\n' > $(EMBED_DIR)/placeholder.txt
	mkdir -p $(EMBED_DIR)/files/dashboard
	cp -r $(REPO_ROOT)/files/dashboard/contracts $(EMBED_DIR)/files/dashboard/
	cp -r $(REPO_ROOT)/files/dashboard/catalog $(EMBED_DIR)/files/dashboard/

embed-web:
	mkdir -p $(EMBED_DIR)
	printf 'placeholder\n' > $(EMBED_DIR)/placeholder.txt
	mkdir -p $(EMBED_DIR)/web
	cp -r $(REPO_ROOT)/apps/ptd-web/dist/. $(EMBED_DIR)/web/

build: embed-sync embed-web
	GOTOOLCHAIN=local go build -o ptd-api ./cmd/ptd-api

build-windows: embed-sync embed-web
	GOOS=windows GOARCH=amd64 GOTOOLCHAIN=local go build -o ptd-api.exe ./cmd/ptd-api

test:
	GOTOOLCHAIN=local go test ./...

clean:
	rm -rf $(EMBED_DIR)
	mkdir -p $(EMBED_DIR)
	printf 'placeholder\n' > $(EMBED_DIR)/placeholder.txt
	rm -f ptd-api ptd-api.exe
