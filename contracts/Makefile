.PHONY: generate-all generate generate-js lint clean install-buf-go install-buf-npm install-js-npm install-all

# Сгенерировать все
generate-all: generate generate-js

# Сгенерировать go файлы
generate:
	buf generate

# Сгенерировать js файлы
generate-js:
	buf generate --template buf.gen.js.yaml

lint:
	buf lint

clean:
	rm -rf ../backend/gen ../frontend/src/gen

install-buf-go:
	CGO_ENABLED=0 go install github.com/bufbuild/buf/cmd/buf@latest

# Альтернатива через npm — работает без Go, нужен только Node.js
install-buf-npm:
	npm install -g @bufbuild/buf

# Глобальная установка JS-плагинов buf — попадают в глобальный bin npm,
# который уже в PATH, поэтому buf их находит без доп. настройки
install-js-npm:
	npm install -g @bufbuild/protoc-gen-es @connectrpc/protoc-gen-connect-es

# Установить всё нужное для генерации, требует npm
install-all: install-buf-npm install-js-npm
