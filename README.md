# VaultDrop

VaultDrop now operates as a full PDF text extraction pipeline: the API streams uploads into MinIO, enqueues background jobs through Redis/Asynq, a worker extracts text from PDFs, stores the structured output in Postgres, and publishes processed `.txt` artifacts back to MinIO. Everything runs with a single `docker compose up`.

## What’s inside

- **cmd/api** – HTTP service that validates uploads, stores PDFs, inserts metadata, and enqueues extraction jobs.
- **cmd/worker** – Asynq worker that downloads PDFs from MinIO, extracts text via `ledongthuc/pdf`, stores the text to Postgres, and uploads `.txt` results.
- **Postgres** – Persists document metadata, status, extracted text, and error messages.
- **Redis + Asynq** – Lightweight job queue for background extraction.
- **MinIO** – S3-compatible object storage that holds both raw and processed artifacts.

Legacy demo code (`cmd/server` and related packages) still exists for reference but the docker-compose stack exercises the new architecture.

## Quick start (one command demo)

```bash
docker compose up --build
```

Services started locally:

- API: http://localhost:8080
- Postgres: localhost:5432 (`vaultdrop` / `vaultdrop`)
- Redis: localhost:6379
- MinIO S3 API: http://localhost:9000 (console: http://localhost:9001)

The containers automatically provision buckets/tables on boot. Stop everything with `Ctrl+C`.

## Upload & processing flow

1. Upload a PDF (only `application/pdf` is accepted):

   ```bash
   curl -F "file=@resume.pdf" http://localhost:8080/documents
   # => {"id":"<uuid>","status":"queued"}
   ```

2. Poll the document to track status (`queued` → `processing` → `completed`):

   ```
   GET /documents/{id}
   ```

3. Retrieve the extracted text directly:

   ```
   GET /documents/{id}/text
   ```

4. Or request a short-lived signed URL to download the `.txt` artifact stored in MinIO:

   ```
   GET /documents/{id}/processed-url
   ```

## API surface

| Method + Path | Description |
| --- | --- |
| `GET /healthz` | Service heartbeat |
| `POST /documents` | Multipart upload (`file` field) of a PDF |
| `GET /documents/{id}` | Metadata: filename, status, timestamps, error info |
| `GET /documents/{id}/text` | Raw extracted text (200 when complete, 202 otherwise) |
| `GET /documents/{id}/processed-url` | Signed URL pointing at the processed `.txt` object in MinIO |

## Web UI

A React + TypeScript frontend (no build system required) lives under `frontend/`:

1. Start the stack: `docker compose up --build`.
2. Open http://localhost:4173 to access the interface.
3. Upload PDFs, monitor processing states, read extracted text, and download the processed `.txt` artifacts directly from the browser.

The UI loads React/ReactDOM from esm.sh at runtime, so you only need to rebuild the TypeScript bundle when editing the `frontend/src` files:

```bash
tsc -p frontend
```

This command emits `frontend/dist/main.js`, which the Nginx container serves. The UI talks to the API over HTTP with CORS enabled and caches uploaded IDs locally so you can refresh or revisit the dashboard later.

## Configuration

The API/worker share the same env vars (defaults shown):

| Variable | Description | Default |
| --- | --- | --- |
| `VAULTDROP_ADDRESS` | API listen address | `:8080` |
| `VAULTDROP_MAX_FILE_BYTES` | Maximum upload size | `26214400` (25 MiB) |
| `VAULTDROP_ALLOWED_TYPES` | Allowed MIME types | `application/pdf,image/png,image/jpeg,text/plain` |
| `VAULTDROP_DATABASE_URL` | Postgres DSN | `postgres://vaultdrop:vaultdrop@localhost:5432/vaultdrop?sslmode=disable` |
| `VAULTDROP_REDIS_ADDR` | Redis host:port | `localhost:6379` |
| `VAULTDROP_REDIS_DB` | Redis DB index | `0` |
| `VAULTDROP_S3_ENDPOINT` | MinIO/S3 endpoint | `localhost:9000` |
| `VAULTDROP_S3_ACCESS_KEY` | S3 access key | `minioadmin` |
| `VAULTDROP_S3_SECRET_KEY` | S3 secret key | `minioadmin` |
| `VAULTDROP_S3_RAW_BUCKET` | Bucket for raw PDFs | `vaultdrop-raw` |
| `VAULTDROP_S3_PROCESSED_BUCKET` | Bucket for `.txt` output | `vaultdrop-processed` |
| `VAULTDROP_SIGNED_TTL` | Signed URL TTL | `5m` |
| `VAULTDROP_WORKERS` | Worker concurrency | `2` |
| `NGROK_AUTHTOKEN` | Optional ngrok auth token for public tunnels | unset |

Override them in `docker-compose.yml` or via your shell.

## Development notes

- `go run ./cmd/api` launches the API if Postgres, Redis, and MinIO are already running locally.
- `go run ./cmd/worker` starts the extractor worker (expects the same backing services).
- Run `go test ./...` after `go mod tidy` to sync dependencies locally (the CLI environment here cannot run `go` tooling).
- The `internal` packages contain reusable building blocks:
  - `internal/database` – pgx connection helpers + schema bootstrap.
  - `internal/repository` – document CRUD/status updates.
  - `internal/s3storage` – MinIO helpers (uploads/downloads/presigned URLs).
  - `internal/queue` – Asynq task definitions.
  - `internal/pdf` – Plain-text extraction from PDFs.
  - `internal/api` / `internal/worker` – HTTP and background logic.

## VaultDrop CLI

A Go-based CLI (`cmd/vaultdrop`) automates common workflows.

### Install or run ad-hoc

```bash
# Run without installing
go run ./cmd/vaultdrop --help

# Install locally (puts binary in GOPATH/bin or GOBIN)
go install ./cmd/vaultdrop
```

### Common commands

| Command | Purpose |
| --- | --- |
| `vaultdrop build` | `docker compose build` (use `--no-cache` if needed) |
| `vaultdrop up` | `docker compose up --build -d` to start the full stack |
| `vaultdrop down -v` | Stop stack and optionally drop volumes |
| `vaultdrop logs -f api worker` | Tail logs from selected services |
| `vaultdrop test` | Run `go test ./...` (add `--race`/`--cover` if desired) |
| `vaultdrop run api` | Execute `go run ./cmd/api` outside Docker |
| `vaultdrop run worker` | Execute `go run ./cmd/worker` outside Docker |

All commands honor `--compose-file`/`-f` if you need to target a different Compose file.

Once the worker finishes processing, you can view the resulting `.txt` inside MinIO (bucket `vaultdrop-processed`) or via the API endpoints above. This makes for a simple but convincing “resume parsing” style demo you can show off with a single compose command.

### ngrok tunneling

For remote demos you can expose the API with ngrok (already included in the compose file). Set your token and start the service:

```bash
export NGROK_AUTHTOKEN=your_token
docker compose up ngrok
```

ngrok forwards the API container (`api:8080`) to a public URL and exposes the web inspector at http://localhost:4040. Without `NGROK_AUTHTOKEN`, the tunnel still runs but may be short-lived or limited in features.
