.EXPORT_ALL_VARIABLES:

GOBIN=$(CURDIR)/bin

build:
	@echo "Building hlf-control-plane binary"
	@go build -tags pkcs11 -o bin/control-plane

build-tools:
	@echo "Installing tools"
	@cd tools && cat tools.go | grep _ | awk -F'"' '{print $$2}' | xargs -tI % go install %
