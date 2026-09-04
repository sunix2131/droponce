# DropOnce

DropOnce is a desktop file sender built with Go, Wails and React. It creates an expiring link and QR code for one file, or for a ZIP assembled from several selected files.

The project has four transfer paths with different trust and availability requirements. They are explicit in the interface because there is no single transport that is both browser-compatible, serverless and reachable from every network.

## Transfer paths

| Mode | Receiver | Where the bytes travel | What must stay online |
| --- | --- | --- | --- |
| Local network | Any browser | Directly from the sender over a private IPv4 network | Sender app |
| CloudPub | Any browser | Through a CloudPub HTTPS tunnel to the sender | Sender app and `clo` process |
| Relay | Any browser | Uploaded to a selected DropOnce relay, then downloaded from it | Relay |
| Direct | DropOnce app | Encrypted frames through a broker | Both apps and broker |

Local mode binds only to an explicitly selected RFC1918 address. It does not listen on `0.0.0.0`, loopback or a public address. The source file is streamed from disk and is not copied into application storage.

CloudPub mode exposes the same local HTTP endpoint through the user-installed `clo` client. DropOnce never downloads or executes a helper binary on its own. A CloudPub token entered in the UI is not saved in the SQLite settings row; the CLI writes its own `cloudpub/config.toml` file with mode `0600` when a tunnel is started.

Relay mode is for a receiver outside the sender's network. The current relay stores the file without end-to-end encryption, so it must be operated by the user or another trusted party. The desktop client accepts only a public HTTPS relay URL.

Direct mode is DropOnce-to-DropOnce transfer over an encrypted broker bridge. It is not a browser transport and it is not UDP hole punching. The broker retains encrypted frames in memory until the session expires.

## Link lifecycle

Local and relay links use 32-byte random URL-safe tokens. The desktop SQLite database stores a SHA-256 token hash rather than the raw link token. A link stops working after its expiry, cancellation or configured download count. Local transfers also stop after an application restart or when the source file's size or modification time changes.

Download counts are reserved atomically, so concurrent requests cannot exceed the configured limit. Interrupted downloads release their reservation. Range requests are rejected because a partial request cannot be counted as a completed one-time download.

The relay accepts files up to 50 GiB by default and removes them after the last permitted download, cancellation or expiry. Its limit is configurable. Direct receivers reject negative, oversized, truncated and overlong payloads; incomplete output files are removed on failure, cancellation or expiry.

For several files, the application creates one temporary ZIP with sanitized, unique entry names. Direct folder transfer is not implemented.

## Direct encryption

Each direct session uses:

- ephemeral X25519 key pairs;
- a 32-byte pairing secret carried in the ticket;
- HKDF-SHA256 for directional session keys;
- ChaCha20-Poly1305 for metadata, chunks and the completion frame;
- strictly increasing counters to reject replayed or reordered frames.

The broker can observe session timing and encrypted message sizes. Possession of the complete `droponce://` ticket is sufficient to join the session, so the ticket must be handled like a password.

More detail is in [docs/security.md](docs/security.md).

## Run from source

Requirements are Go from `go.mod`, Node.js 24 or newer, Wails CLI 2.12 and the platform packages required by Wails.

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0
npm ci --prefix frontend
wails dev
```

Build the desktop application with:

```bash
wails build
```

CloudPub mode additionally requires a separately installed `clo` executable in `PATH`, `tools/cloudpub/clo`, or the DropOnce application-data directory.

## Self-hosted services

Run the relay behind an HTTPS reverse proxy and set its public URL:

```bash
go run ./cmd/droponce-relay \
  -addr :8088 \
  -storage /var/lib/droponce-relay \
  -public-url https://relay.example.com \
  -max-upload-gb 50
```

Run the direct broker with a bounded session lifetime and encrypted-message budget:

```bash
go run ./cmd/droponce-broker \
  -addr :8091 \
  -max-session-minutes 30 \
  -max-inflight-gb 50
```

These binaries do not include user accounts, quotas shared across sessions or an administration API. A public deployment therefore needs authentication and traffic limits at the reverse proxy or network edge.

## Verification

The repository check runs frontend compilation and tests, Go vet, Go tests and the race detector. It also runs `staticcheck` and `govulncheck` when they are installed.

```bash
./scripts/verify.sh
```

GitHub Actions runs the same core checks on every push and pull request. Tags matching `v*` trigger Wails builds on macOS, Windows and Linux and publish the binaries as workflow artifacts.

## Layout

```text
cmd/droponce-relay       temporary-file relay server
cmd/droponce-broker      in-memory encrypted-frame broker
internal/application     transfer lifecycle and desktop use cases
internal/direct          ticket parsing and session cryptography
internal/relay           relay HTTP API and download accounting
internal/broker          broker HTTP API and session bounds
internal/infrastructure  SQLite, filesystem, network and QR adapters
internal/receiverweb     browser download page
frontend/src             React and TypeScript desktop interface
```
