# DropOnce Security Notes

DropOnce is designed for trusted local networks only. It should not be used on public Wi-Fi, guest networks, airports, cafes, or any network where unknown users may observe or reach the sender's device.

The internet mode is different: it uses an explicit DropOnce relay chosen by the user. Do not use random open servers as relays. A relay can see and store the uploaded file in the current implementation, so use your own relay or a relay you trust.

Security properties implemented in this repository:

- raw transfer tokens are generated with `crypto/rand`;
- tokens are 32 bytes and encoded with base64 URL-safe encoding without padding;
- SQLite stores only `SHA-256(token)`;
- raw tokens are kept only in runtime memory;
- active transfers are marked `ended_after_restart` on app startup;
- HTTP routes are limited to `/d/{token}` and `/d/{token}/download`;
- invalid, expired, cancelled, and consumed links return a generic `404`;
- Range requests return `416`;
- downloads use `Content-Disposition: attachment`;
- receiver IP addresses are used only transiently in the in-memory rate limiter.

Internet relay properties:

- relay transfers use cryptographically random receiver and cancellation tokens;
- relay files are stored with generated internal IDs, not original filesystem paths;
- relay links expire and enforce a download limit;
- `DELETE /v1/transfers/{id}?cancel_token=...` removes a relay transfer;
- relay upload size is capped by `-max-upload-gb`;
- relay uploads stream directly to storage instead of loading the whole file into memory;
- Range requests return `416`.

Direct P2P properties:

- sender and receiver generate X25519 keypairs per session;
- QR tickets include a 32-byte pairing secret and expiry;
- session keys are derived with HKDF-SHA256;
- metadata and file chunks are encrypted with ChaCha20-Poly1305;
- replayed encrypted frame counters are rejected by the receiver;
- the broker relays encrypted messages in memory and does not store file blobs on disk.
