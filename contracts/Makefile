.PHONY: generate generate-js lint clean check-plugins check-plugins-grpc check-plugins-js install-buf

generate: check-plugins
	buf generate

generate-js: check-plugins-js
	buf generate --template buf.gen.js.yaml

lint:
	buf lint

clean:
	rm -rf ../backend/gen ../frontend/src/gen

check-plugins: check-plugins-grpc check-plugins-js
	@command -v protoc-gen-connect-go >/dev/null || { echo "go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest"; exit 1; }

check-plugins-grpc:
	@command -v buf >/dev/null || { echo "install buf: https://buf.build/docs/installation"; exit 1; }
	@command -v protoc-gen-go >/dev/null || { echo "go install google.golang.org/protobuf/cmd/protoc-gen-go@latest"; exit 1; }

check-plugins-js:
	@command -v buf >/dev/null || { echo "install buf: https://buf.build/docs/installation"; exit 1; }
	@[ -x ../frontend/node_modules/.bin/protoc-gen-es ] || (cd ../frontend && npm install)

install-buf:
	go install github.com/bufbuild/buf/cmd/buf@latest
