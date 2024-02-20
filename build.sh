GOOS=linux   GOARCH=amd64 go build -o releem-agent-x86_64
GOOS=linux   GOARCH=amd64 go build -o releem-agent-amd64
GOOS=linux   GOARCH=arm64 go build -o releem-agent-aarch64
GOOS=linux     GOARCH=386 go build -o releem-agent-i686
GOOS=freebsd GOARCH=amd64 go build -o releem-agent-freebsd-amd64
