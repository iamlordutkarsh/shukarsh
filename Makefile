.PHONY: build clean stop start restart test

build:
	go build -o shukarsh-server ./cmd/srv

clean:
	rm -f shukarsh-server

test:
	go test ./...
