.PHONY: generate-all generate generate-js lint clean install-buf-go install-buf-npm install-js-npm install-all

# Инструкция, как генерировать файлы из proto
# 1) Вы должны находиться в текущей директории, то есть в contracts/
#	 Если вы не в ней выполните из корня репозитория: cd contracts
#	 	все просто.
# 2) Должна быть установлена утилита npm (для некоторых команд можно иметь go install)
#	 Проверить ее наличие можно командой: npm --version
#    Если ее нет, официальная инструкция по ее установке: https://nodejs.org/en/download
# 3) Установка buf (buf для генерации go кода + плагин к нему protoc-gen-connect-es для генерации js кода)
#    С make можно выполнить: make install-all он установит все нужное через npm.
#    Если make нет (странно что вы тут читаете конечно но ладно), можно просто команды скопировать из make объявлений.
#		То есть ищете install-buf-npm, копируете от туда команду - выполняете, также с install-js-npm.
#	 После у вас установятся нужные утилиты. Баф можно проверить командой: buf --version
# 4) Сама генерация:
#		Чтобы сгенерировать все через make выполните: make generate-all
#		Если вы бедолага и у вас нет make, копируйте команды из generate и generate-js и выполняйте (как делали в пункте 3)
# 5) Если все отлично в /backend/gen/ /frontend/src/gen (от корня репозитория) у вас сгенерируются новые файлики.
#	 На этом все. Если на каких-то этапах ошибки, кидаете ии-шке - она поможет отлично.

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
