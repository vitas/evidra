# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS build
WORKDIR /src

COPY go.mod ./
COPY go.sum ./
COPY cmd ./cmd
COPY internal ./internal
COPY db ./db

RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/evidra ./cmd/evidra

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app

COPY --from=build /out/evidra /app/evidra
COPY --from=build /src/db /app/db

ENV EVIDRA_ADDR=:8080
ENV EVIDRA_EXPORT_DIR=/var/evidra/exports

EXPOSE 8080
ENTRYPOINT ["/app/evidra"]
