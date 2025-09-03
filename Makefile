.PHONY: build clean

build:
	go build -o zmux-server ./cmd/zmux-server
	go build -o bulk-delete ./cmd/bulk-delete

clean:
	rm -f zmux-server bulk-delete
