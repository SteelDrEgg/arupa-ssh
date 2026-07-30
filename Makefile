PLUGIN_BINARY := ssh
PLUGIN_NAME := $(shell awk -F': *' '$$1 == "Name" { print $$2; exit }' info.yaml)
PLUGIN_VERSION := $(shell awk -F': *' '$$1 == "Version" { print $$2; exit }' info.yaml)
UI_DIR := ui
DIST_DIR := $(CURDIR)/dist
PLUGIN_DIR := $(CURDIR)/plugins
PACKAGE_WORKDIR := $(DIST_DIR)/$(PLUGIN_NAME)_pkg
PLUGIN_PACKAGE := $(PLUGIN_DIR)/$(PLUGIN_NAME).plg

.PHONY: build package check clean frontend-build frontend-check

build: frontend-build $(PLUGIN_BINARY)

$(PLUGIN_BINARY): go.mod go.sum info.yaml $(shell find core -type f -name '*.go')
	$(if $(PLUGIN_VERSION),,$(error missing Version in info.yaml))
	go build -ldflags "-X main.pluginVersion=$(PLUGIN_VERSION)" -o $(PLUGIN_BINARY) ./core

frontend-build: $(UI_DIR)/package-lock.json
	npm --prefix $(UI_DIR) run build

frontend-check: $(UI_DIR)/package-lock.json
	npm --prefix $(UI_DIR) run check

package: build
	$(if $(PLUGIN_NAME),,$(error missing Name in info.yaml))
	rm -rf $(PACKAGE_WORKDIR)
	mkdir -p $(PACKAGE_WORKDIR)/Content/$(UI_DIR) $(PLUGIN_DIR)
	cp $(PLUGIN_BINARY) $(PACKAGE_WORKDIR)/Content/$(PLUGIN_BINARY)
	cp -R $(UI_DIR)/build $(PACKAGE_WORKDIR)/Content/$(UI_DIR)/build
	cp info.yaml $(PACKAGE_WORKDIR)/info.yaml
	rm -f $(PLUGIN_PACKAGE)
	cd $(PACKAGE_WORKDIR) && zip -qr $(PLUGIN_PACKAGE) .
	rm -rf $(PACKAGE_WORKDIR)

check: frontend-check
	go test ./core

$(UI_DIR)/package-lock.json: $(UI_DIR)/package.json
	npm --prefix $(UI_DIR) install

clean:
	rm -rf $(PLUGIN_BINARY) $(UI_DIR)/build $(UI_DIR)/.svelte-kit $(PACKAGE_WORKDIR) $(PLUGIN_PACKAGE)
