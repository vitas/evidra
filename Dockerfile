FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /evidra-mcp ./cmd/evidra-mcp

FROM gcr.io/distroless/static:nonroot
LABEL io.modelcontextprotocol.server.name="io.github.vitas/evidra"
COPY --from=builder /evidra-mcp /usr/local/bin/evidra-mcp
ENTRYPOINT ["/usr/local/bin/evidra-mcp"]
