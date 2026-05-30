.PHONY: build install test clean

# Build binary in current directory
build:
	go build -o daily-report-daemon ./cmd/daily-report-daemon

# Install to system PATH ($GOPATH/bin or /usr/local/bin)
install:
	go install ./cmd/daily-report-daemon

# Run tests
test:
	go test ./... -count=1

# Clean build artifacts
clean:
	rm -f daily-report-daemon
