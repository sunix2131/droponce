# DropOnce

DropOnce is a Wails desktop app for one-time file transfers by QR code.

Pick one file, or select multiple files and let DropOnce package them into one ZIP, choose local-network, internet relay, or Direct P2P mode, set an expiry and download limit, then show or copy a temporary URL/ticket. In local mode, the receiver opens the QR code in a browser on the same LAN and downloads directly from your computer. In internet relay mode, the sender uploads to a user-provided DropOnce relay and the receiver downloads from that relay. In Direct P2P mode, both sides use DropOnce and file chunks are end-to-end encrypted before crossing the broker.

## How Transfer Works

1. DropOnce validates the selected path as a readable regular file.
2. It creates a 32-byte crypto-random token and stores only `SHA-256(token)` in SQLite.
3. A local HTTP server listens on the chosen private IPv4 address only.
4. The QR code contains `http://<private-ip>:<port>/d/<secret-token>`.
5. The file is streamed only from `/d/{token}/download`.
6. A completed download increments the counter; interrupted downloads do not.
7. Expired, cancelled, consumed, failed, or post-restart transfers return the same `404`.

## Internet Relay Mode

Internet mode is implemented with a relay server that you run or explicitly trust. DropOnce does not use random public servers.

For quick same-network testing you can start a relay locally, but links built with local or private addresses will not work outside your network:

```sh
go run ./cmd/droponce-relay -addr :8088
```

Start a relay on a VPS behind HTTPS:

```sh
droponce-relay -addr :8088 -storage /var/lib/droponce-relay -public-url https://relay.example.com -max-upload-gb 50
```

For QR links that work from any network, run the relay on a public HTTPS endpoint and enter that URL in `Через интернет`. The app uploads the selected file to that relay, receives a temporary QR link, and can cancel the relay item while the desktop app is still running. The relay removes a file after expiry, cancellation, or once the download limit is consumed.

Important: the current relay mode is trust-based transport, not end-to-end encrypted storage. Use your own relay or a relay you trust.

## Direct P2P Mode

Direct P2P mode is for DropOnce-to-DropOnce transfer. It does not require HTTPS for payload confidentiality because the application encrypts each frame with X25519 + HKDF-SHA256 + ChaCha20-Poly1305.

Start the broker:

```sh
go run ./cmd/droponce-broker -addr :8091
```

Or with explicit limits:

```sh
droponce-broker -addr :8091 -max-session-minutes 30 -max-inflight-gb 50
```

Use `Direct P2P` in the sender app, enter the broker URL, then create the QR. The QR contains a `droponce://receive/...` ticket with the broker URL, sender public key, pairing secret, and expiry. The receiver can paste that ticket into DropOnce under `Direct P2P` and press `Принять файл`.

The broker is untrusted: it relays in-memory encrypted messages and does not store file blobs on disk. This implementation uses the encrypted broker bridge as the reliable fallback path. Native OS registration for automatically opening `droponce://` links and UDP hole punching are planned follow-ups.

## Large Files And Many Files

Local mode streams files from disk to the receiver and has no application-level size cap. Practical limits are the sender disk, receiver disk, network speed, browser behavior, and transfer expiry.

Internet relay mode streams upload/download and defaults to `50 GiB` per file. Change that on the relay with:

```sh
droponce-relay -max-upload-gb 100
```

One transfer link still represents one downloadable object. To send many files at once, use `Несколько файлов в ZIP` in the app. DropOnce creates a temporary ZIP archive and sends that one archive by the normal transfer path.

Direct P2P also streams file chunks and supports the same ZIP packaging path for many files.

## Security And Trusted Networks

Local mode is not intended for sending files over the internet.

Use local mode only in a trusted local network. Do not share QR codes with strangers. DropOnce uses a secret temporary token, but local mode does not create a TLS channel. After the app closes, active local links stop working because raw tokens live only in process memory and are not restored from SQLite.

The server refuses to bind to `0.0.0.0`, localhost, public IPv4, link-local IPv4, or IPv6. The first implementation supports only RFC1918 private IPv4 ranges:

- `10.0.0.0/8`
- `172.16.0.0/12`
- `192.168.0.0/16`

## Supported OS

- macOS Intel and Apple Silicon
- Windows 10/11
- Linux x64

Native packages are produced by Wails on matching CI runners.

## Development

Requirements:

- Go matching `go.mod`
- Node.js 24+
- Wails CLI v2.12

Install Wails:

```sh
go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0
```

Install frontend dependencies:

```sh
cd frontend
npm ci
```

Run in development mode:

```sh
wails dev
```

Build:

```sh
wails build
```

## Verification

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

## Project Structure

```text
internal/domain/transfer          domain transfer types
internal/application              transfer service, token service, runtime registry
internal/infrastructure/database  SQLite migration and repository
internal/infrastructure/filesystem file validation and hashing
internal/infrastructure/network   private IPv4 resolver and HTTP headers
internal/infrastructure/qr        QR PNG generation
internal/direct                   Direct P2P crypto and ticket layer
internal/broker                   encrypted in-memory broker
internal/relay                    internet relay HTTP server
cmd/droponce-broker               standalone Direct P2P broker
cmd/droponce-relay                standalone relay binary
frontend/src                      React TypeScript desktop UI
.github/workflows                 CI and release workflows
```

## Privacy

DropOnce does not store raw tokens, receiver IP addresses, User-Agent strings, full URLs, or file contents. History hides old links and source paths for completed transfers. The original file is never deleted after transfer.

## Why A Link May Not Open

- The receiver is not on the same LAN.
- The transfer expired.
- The download limit was consumed.
- The sender cancelled the transfer.
- The app was closed or restarted.
- The source file changed after the link was created.
- The chosen network interface disappeared.

## Current Limitations

- Internet transfer requires a user-provided relay.
- Relay mode currently stores files temporarily on the relay without E2E encryption.
- Relay mode defaults to 50 GiB per uploaded file; the relay operator can change this.
- Direct P2P currently uses the encrypted broker bridge fallback; UDP hole punching is not yet fully implemented.
- `droponce://` tickets can be copied/pasted in DropOnce; OS protocol registration is not yet wired into installers.
- One file per transfer.
- No folder transfer.
- IPv6 is intentionally out of scope for the first implementation.

## License

Add your preferred license before public distribution.
