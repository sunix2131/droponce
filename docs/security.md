# Security model

DropOnce provides several transports, not one universal security boundary. The sender chooses the transport and therefore chooses which infrastructure must be trusted.

## Local network

The local HTTP server accepts only an explicitly selected private IPv4 address in `10.0.0.0/8`, `172.16.0.0/12` or `192.168.0.0/16`. It refuses wildcard, loopback and public bindings. This prevents accidental public exposure but does not make an untrusted Wi-Fi network safe: anyone who obtains the link can request the file.

Transfer tokens contain 32 random bytes from `crypto/rand`. SQLite stores their SHA-256 hashes; raw tokens remain in the process registry while a transfer is active. Invalid, expired, cancelled and consumed links all return a generic `404`.

The server rejects range requests and serializes downloads for a local transfer. A completed response increments the counter only after the expected number of bytes has been written. The file's size and modification time are checked before and after streaming.

## CloudPub

CloudPub publishes the local HTTP endpoint through the external `clo` process. DropOnce requires an existing executable and does not download one at runtime.

The token entered in DropOnce is kept in application memory rather than the SQLite settings row. Starting a tunnel passes it to `clo set token`; the resulting `cloudpub/config.toml` belongs to the CloudPub client and is chmodded to `0600`. CloudPub is part of the trusted transport for this mode and can observe connection metadata and relayed traffic.

## Relay

Relay transfers use independent random download and cancellation tokens. Uploaded files are written under generated IDs with mode `0600`, not under caller-provided paths. Filenames are sanitized before being used in HTTP headers.

The relay stores plaintext file contents. It enforces expiry, a per-file size ceiling, download-count reservations and cleanup after cancellation or the final successful download. The built-in service has no accounts or global tenant quota, so an internet deployment needs authentication, rate limits and storage monitoring outside the process.

## Direct

A direct ticket contains the session ID, broker URL, sender public key, expiry and a 32-byte pairing secret. The receiver proves possession of that secret before the sender begins transferring data.

Both sides generate ephemeral X25519 key pairs. They derive directional keys with HKDF-SHA256 and encrypt metadata, 256 KiB chunks and the final marker with ChaCha20-Poly1305. The frame type is authenticated as additional data. Counters must be strictly increasing, which rejects duplicates and reordering without retaining an unbounded replay set.

The receiver accepts one metadata frame, enforces a 50 GiB declared-size ceiling, refuses data before metadata and verifies the exact byte count before completion. Partial output is deleted after protocol errors, expiry or cancellation.

The broker sees session timing and encrypted frame sizes. It keeps frames in memory until session expiry and applies a configurable per-session byte budget. It does not authenticate operators or hide network metadata. Direct mode is therefore an encrypted broker bridge, not anonymous transport or peer-to-peer NAT traversal.

## Out of scope

- protection after an authorized receiver saves a file;
- anonymity from the relay, broker, tunnel provider or network operator;
- malware scanning of transferred content;
- recovery of lost transfer tickets;
- browser-based end-to-end encrypted transfer;
- multi-tenant hardening for public relay or broker hosting.
