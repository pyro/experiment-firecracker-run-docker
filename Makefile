# Base path used to install.
DESTDIR=/usr/local
BINARY=docker-to-firecracker

.PHONY: clean build install uninstall all

all: clean build

clean: ## removes the binary
	@echo "Cleaning $(BINARY)"
	@rm -f $(BINARY)

build: ## builds the go binary
	@echo "Building $(BINARY)"
	@go build -o $(BINARY) main.go run.go

install: ## install binary
	@echo "Installing $(BINARY) to $(DESTDIR)/bin"
	@mkdir -p $(DESTDIR)/bin
	@mv $(BINARY) $(DESTDIR)/bin

uninstall: ## uninstall binary
	@echo "Uninstalling $(BINARY) from $(DESTDIR)/bin"
	@rm -f $(addprefix $(DESTDIR)/bin/,$(notdir $(BINARY)))
