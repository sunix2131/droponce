# DropOnce

DropOnce — настольное приложение на Wails для одноразовой передачи файлов по QR-коду.

Вы выбираете один файл или несколько файлов в ZIP, выбираете режим передачи, срок действия и лимит скачиваний. DropOnce создаёт временную ссылку и QR-код. Получатель сканирует QR телефоном или открывает ссылку на другом устройстве и скачивает файл в браузере.

## Быстрый Запуск

### macOS

```sh
open build/bin/droponce.app
```

Если macOS блокирует запуск локальной сборки:

```sh
xattr -dr com.apple.quarantine build/bin/droponce.app
open build/bin/droponce.app
```

### Из Исходников

```sh
go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0
cd frontend
npm ci
cd ..
wails dev
```

## Режимы Передачи

### В Одной Сети

Для телефона или компьютера в той же Wi-Fi/Ethernet сети.

Как работает:

1. DropOnce запускает локальный HTTP-сервер на выбранном частном IPv4-адресе.
2. В QR попадает ссылка вида `http://<ip>:<port>/d/<secret-token>`.
3. Телефон открывает мобильную страницу скачивания в браузере.
4. Файл стримится напрямую с компьютера отправителя.

Этот режим не загружает файл в облако и не хранит его на сервере.

### CloudPub HTTPS

Для скачивания с телефона из любой сети без отдельного приложения на телефоне.

DropOnce запускает локальный сервер, затем поднимает HTTPS-туннель через CloudPub. В QR попадает публичная ссылка `https://...cloudpub.ru/...`, а файл всё равно стримится с вашего компьютера.

Что нужно:

- аккаунт CloudPub;
- установленная команда `clo` в `PATH`;
- CloudPub token в настройках DropOnce;
- открытое приложение DropOnce во время скачивания.

Команда `clo` сохраняет CloudPub token в `cloudpub/config.toml` внутри директории данных DropOnce. Файл получает права `0600`. Token не должен попадать в GitHub, логи или публичные чаты.

### Через Интернет

Для передачи через отдельный DropOnce relay.

В этом режиме приложение загружает файл на relay-сервер, а получатель скачивает файл с relay. Это удобно, когда компьютер отправителя не должен оставаться доступным напрямую.

Запуск relay локально для теста:

```sh
go run ./cmd/droponce-relay -addr :8088
```

Запуск relay на VPS:

```sh
droponce-relay \
  -addr :8088 \
  -storage /var/lib/droponce-relay \
  -public-url https://relay.example.com \
  -max-upload-gb 50
```

Для QR-ссылок из любой сети relay должен быть доступен по публичному HTTPS-адресу.

Важно: текущий relay-режим временно хранит файл на relay без end-to-end шифрования. Используйте свой relay или сервер, которому доверяете.

### Direct P2P

Для передачи DropOnce-to-DropOnce, когда приложение установлено на обеих сторонах.

Передача шифруется на уровне приложения:

- X25519;
- HKDF-SHA256;
- ChaCha20-Poly1305;
- monotonic nonce/counter для защиты от replay.

Запуск broker:

```sh
go run ./cmd/droponce-broker -addr :8091
```

С лимитами:

```sh
droponce-broker \
  -addr :8091 \
  -max-session-minutes 30 \
  -max-inflight-gb 50
```

Broker считается недоверенным: он пересылает encrypted frames в памяти и не сохраняет файл на диск. Сейчас Direct P2P использует encrypted broker bridge как надёжный fallback. Автоматическое открытие `droponce://` и UDP hole punching можно добавить следующим этапом.

## Большие И Несколько Файлов

Локальный режим и CloudPub HTTPS стримят файл с диска и не имеют отдельного лимита приложения. Практические ограничения:

- свободное место на устройстве получателя;
- скорость сети;
- стабильность соединения;
- срок действия ссылки;
- поведение браузера при очень больших скачиваниях.

Relay по умолчанию принимает до `50 GiB` на файл. Лимит меняется флагом:

```sh
droponce-relay -max-upload-gb 100
```

Одна ссылка передаёт один скачиваемый объект. Чтобы отправить много файлов, нажмите `Несколько файлов в ZIP`: DropOnce создаст временный ZIP-архив и передаст его как один файл.

## Безопасность

DropOnce создаёт 32-байтный криптографически случайный token и хранит в SQLite только `SHA-256(token)`. Сырые token живут только в памяти процесса.

Локальный сервер:

- не биндится на `0.0.0.0`;
- не биндится на localhost;
- не биндится на публичный IPv4;
- принимает только частные RFC1918 IPv4-адреса.

Поддерживаемые локальные диапазоны:

- `10.0.0.0/8`;
- `172.16.0.0/12`;
- `192.168.0.0/16`.

Активные ссылки перестают работать после:

- истечения срока;
- отмены;
- достижения лимита скачиваний;
- закрытия или перезапуска приложения;
- изменения исходного файла после создания ссылки.

## Приватность

DropOnce не хранит:

- raw transfer tokens;
- IP получателей;
- User-Agent;
- полные URL;
- содержимое файлов.

История скрывает старые ссылки и пути к исходным файлам для завершённых передач. Исходный файл после передачи не удаляется.

## Поддерживаемые ОС

- macOS Intel и Apple Silicon;
- Windows 10/11;
- Linux x64.

Пакеты собираются Wails на соответствующих CI runner.

## Разработка

Требования:

- Go из `go.mod`;
- Node.js 24+;
- Wails CLI v2.12.

Установка Wails:

```sh
go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0
```

Установка frontend-зависимостей:

```sh
cd frontend
npm ci
```

Запуск dev-режима:

```sh
wails dev
```

Сборка:

```sh
wails build
```

## Проверка

Быстрая проверка проекта:

```sh
./scripts/verify.sh
```

Ручные команды:

```sh
cd frontend
npm run typecheck
npm run lint
npm test
npm run build

cd ..
go vet $(go list ./... | grep -v '/frontend/node_modules/')
go test $(go list ./... | grep -v '/frontend/node_modules/')
go test -race $(go list ./... | grep -v '/frontend/node_modules/')
staticcheck $(go list ./... | grep -v '/frontend/node_modules/')
govulncheck ./...
```

## Структура Проекта

```text
internal/domain/transfer           доменные типы передач
internal/application               сервис передач, token service, runtime registry
internal/infrastructure/database   SQLite migration и repository
internal/infrastructure/filesystem проверка файлов и SHA-256
internal/infrastructure/network    private IPv4 resolver и HTTP security headers
internal/infrastructure/qr         генерация QR PNG
internal/receiverweb               мобильная web-страница получателя
internal/direct                    crypto и ticket layer для Direct P2P
internal/broker                    encrypted in-memory broker
internal/relay                     HTTP relay-сервер
cmd/droponce-broker                standalone Direct P2P broker
cmd/droponce-relay                 standalone relay binary
frontend/src                       React TypeScript UI
.github/workflows                  CI и release workflows
```

## Почему Ссылка Может Не Открыться

- телефон не в той же сети для локального режима;
- CloudPub-туннель не поднялся или приложение закрыто;
- relay недоступен из интернета;
- срок действия истёк;
- лимит скачиваний уже использован;
- отправитель отменил передачу;
- исходный файл изменился;
- выбранный сетевой интерфейс исчез.

## Текущие Ограничения

- browser-only P2P без HTTPS не реализован: мобильные браузеры ограничивают такие сценарии;
- relay-режим временно хранит файл на relay без E2E-шифрования;
- Direct P2P пока использует encrypted broker bridge fallback;
- `droponce://` ticket можно вставлять в DropOnce вручную, но OS protocol registration ещё не подключён в installers;
- одна ссылка передаёт один файл или один ZIP;
- передача папки напрямую пока не реализована;
- IPv6 намеренно не входит в первый релиз.
