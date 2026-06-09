.PHONY: frontend test build package clean

DIST_DIR ?= dist
BINARY ?= birdspy

frontend:
	npm install
	npm run build

test:
	go test ./...

build: frontend test
	mkdir -p $(DIST_DIR)
	go build -trimpath -ldflags="-s -w" -o $(DIST_DIR)/$(BINARY) .

package: build
	cp config.json $(DIST_DIR)/config.json
	find $(DIST_DIR) -mindepth 1 -maxdepth 1 -type f | sort

clean:
	rm -rf $(DIST_DIR) public/build
