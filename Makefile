.PHONY: build clean

build:
	go build -o zmux-server ./cmd/zmux-server

clean:
	rm -f zmux-server
