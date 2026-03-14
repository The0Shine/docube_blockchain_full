# =============================================================================
# Docube Blockchain — Single Image
# =============================================================================
# Contains:
#   - Hyperledger Fabric tools (cryptogen, configtxgen, peer, osnadmin)
#   - Go blockchain service binary
#   - Network configs + scripts + chaincode
#
# Push:   docker build -t horob1/docube-blockchain:latest .
#         docker push horob1/docube-blockchain:latest
#
# Run:    docker compose up -d
# =============================================================================

# ── Stage 1: Build Go service ─────────────────────────────────────────────────
FROM golang:1.23-alpine AS go-builder

RUN apk add --no-cache git ca-certificates

WORKDIR /build

COPY service/go.mod service/go.sum ./
RUN go mod download

COPY service/ ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /blockchain-service ./cmd/server

# ── Stage 2: Final image (Fabric tools + Go binary) ───────────────────────────
FROM hyperledger/fabric-tools:2.5

RUN apt-get update && apt-get install -y --no-install-recommends \
    netcat-openbsd \
    jq \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /fabric

# Copy Go binary
COPY --from=go-builder /blockchain-service /usr/local/bin/blockchain-service

# Copy network configs
COPY network/configtx/   ./configtx/
COPY network/config/     ./config/
COPY network/organizations/cryptogen/ ./organizations/cryptogen/
COPY network/compose/docker/peercfg/  ./peercfg/

# Copy chaincode
COPY network/chaincode/  ./chaincode/

# Copy service config
COPY service/config/     ./service-config/

# Create /app directory for service mode
RUN mkdir -p /app/config

# Copy entrypoints
COPY deployments/docker/entrypoint-init.sh    /entrypoint-init.sh
COPY deployments/docker/entrypoint-service.sh /entrypoint-service.sh
RUN chmod +x /entrypoint-init.sh /entrypoint-service.sh

# Copy cryptogen configs to a safe location (not overlaid by volume mount)
COPY network/organizations/cryptogen/ /fabric/cryptogen-configs/

VOLUME ["/fabric/organizations", "/fabric/channel-artifacts"]

# Default: service mode (overridden to init in compose)
ENTRYPOINT ["/entrypoint-service.sh"]
