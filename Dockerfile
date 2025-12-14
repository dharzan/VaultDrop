# syntax=docker/dockerfile:1

FROM golang:1.21 AS base
WORKDIR /src

COPY go.mod go.sum* ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -o /bin/api ./cmd/api
RUN CGO_ENABLED=0 go build -o /bin/worker ./cmd/worker

FROM gcr.io/distroless/base-debian11 AS api
COPY --from=base /bin/api /usr/local/bin/api
ENTRYPOINT ["/usr/local/bin/api"]

FROM gcr.io/distroless/base-debian11 AS worker
COPY --from=base /bin/worker /usr/local/bin/worker
ENTRYPOINT ["/usr/local/bin/worker"]
