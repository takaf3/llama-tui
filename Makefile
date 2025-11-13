# Makefile for llama-tui

BINARY_NAME ?= llama-tui
BUILD_DIR ?= bin
INSTALL_DIR ?= $(HOME)/.local/bin

.PHONY: build install uninstall run clean

build:
	@mkdir -p $(BUILD_DIR)
	@go build -ldflags "-s -w" -o $(BUILD_DIR)/$(BINARY_NAME) .
	@echo "Built $(BINARY_NAME) to $(BUILD_DIR)/$(BINARY_NAME)"

install: build
	@mkdir -p $(INSTALL_DIR)
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "Installed $(BINARY_NAME) to $(INSTALL_DIR)/$(BINARY_NAME)"
	@echo "Make sure $(INSTALL_DIR) is in your PATH"

uninstall:
	@rm -f $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "Uninstalled $(BINARY_NAME) from $(INSTALL_DIR)"

run:
	@go run .

clean:
	@rm -rf $(BUILD_DIR)
	@echo "Cleaned $(BUILD_DIR) directory"

