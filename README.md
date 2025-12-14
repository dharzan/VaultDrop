# VaultDrop

VaultDrop is a lightweight Go service that demonstrates how to securely accept and process streamed file uploads. It validates file size/type, simulates an anti-virus scan, pushes the upload onto a background processing queue, and exposes short-lived signed download URLs.

## Features

- **Streaming uploads** – multipart file data is streamed directly to disk with strict byte limits to avoid buffering in memory.
- **File validation** – configurable allowed MIME types plus maximum file size checks.
- **Virus scan simulation** – each upload is scanned for a dummy signature before it is accepted.
- **Background processing** – a worker pool dequeues uploads and marks them as processed.
- **Signed URLs** – requesters can mint HMAC-based signed download URLs with configurable TTLs.

## Getting Started

1. Ensure Go 1.21+ is installed.
2. Clone the repository and start the server:

```bash
go run ./cmd/server
```

Environment variables (all optional):

| Variable | Description | Default |
| --- | --- | --- |
| `VAULTDROP_ADDRESS` | HTTP listen address | `:8080` |
| `VAULTDROP_MAX_FILE_BYTES` | Maximum allowed upload size | `26214400` (25 MiB) |
| `VAULTDROP_ALLOWED_TYPES` | Comma-separated list of allowed MIME types | `application/pdf,image/png,image/jpeg,text/plain` |
| `VAULTDROP_SIGNING_SECRET` | HMAC secret used for signed URLs | randomly generated per process |
| `VAULTDROP_SIGNED_TTL` | Duration before signed URLs expire | `5m` |
| `VAULTDROP_WORKERS` | Background processor workers | `2` |

## API Overview

### Health

```
GET /healthz
```

### Upload

```
POST /upload
Content-Type: multipart/form-data
Form field: file=@/path/to/file.pdf
```

Response:

```json
{
  "id": "a1b2c3",
  "status": "queued"
}
```

### File Status

```
GET /files/{id}
```

Shows metadata and lifecycle status (uploaded → scanned → queued → processing → complete).

### Signed Download URL

```
POST /files/{id}/signed-url
```

Returns JSON containing a short-lived `/download` URL. The same endpoint accepts `GET` if a UI prefers a link-style action.

### Download

```
GET /download?file={id}&expires={unix}&signature={hex}
```

The query parameters must come from `/files/{id}/signed-url`. Requests fail when the signature is invalid or the link is expired.

## Testing

Unit tests exist for the signing package:

```bash
go test ./internal/signing
```

Because uploads rely on the standard library HTTP server, they can be manually exercised with curl:

```bash
curl -F "file=@./sample.pdf" http://localhost:8080/upload
```

After the upload returns an ID, poll `/files/{id}` for status changes and call `/files/{id}/signed-url` to obtain a download link once processing completes.
