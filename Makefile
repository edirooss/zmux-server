.PHONY: build clean

build:
	go build -o zmux-server ./cmd/zmux-server
	go build -o bulk-delete ./cmd/bulk-delete

clean:
	rm -f zmux-server bulk-delete

generate-docs:
	cd devtools/specs && npx apibake channels.b2b.openapi.yaml --title 'Zmux Channels – Client API' --subtitle 'v1.0.0'
