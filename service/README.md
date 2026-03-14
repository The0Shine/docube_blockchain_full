# Docube Blockchain Backend Service

Enterprise-grade Go backend service for integrating with Hyperledger Fabric network.

---

## 📋 Project Overview

This service acts as a **gateway** between UI applications and the Docube Hyperledger Fabric network. It:

- Runs **OUTSIDE** the Fabric network
- Connects to Fabric via **SDK + Connection Profile**
- Manages **multi-organization identities** (AdminOrg, UserOrg)
- Exposes **REST/gRPC APIs** for document management

### Target Fabric Network

| Component | Value |
|-----------|-------|
| Channel | `docubechannel` |
| Chaincode | `document_nft_cc` v5.0 |
| Organizations | AdminOrgMSP, UserOrgMSP |
| Orderer | orderer.docube.com:7050 |
| State DB | CouchDB (rich queries) |

---

## 🗂️ Project Structure

```
docube_blockchain_service/
├── cmd/
│   └── server/
│       └── main.go                    # Service entrypoint
│
├── config/
│   ├── app.yaml                       # Application settings
│   ├── fabric.yaml                    # Fabric SDK config
│   └── logging.yaml                   # Logging config
│
├── internal/
│   ├── fabric/                        # Fabric SDK integration
│   │   ├── sdk/                       # SDK initialization
│   │   ├── identity/                  # Wallet & identity management
│   │   ├── channel/                   # Channel client operations
│   │   ├── event/                     # Chaincode event listener
│   │   └── ledger/                    # Ledger query operations
│   │
│   ├── service/                       # Business logic layer
│   ├── repository/                    # Off-chain persistence
│   ├── transport/                     # API layer
│   │   ├── http/                      # REST endpoints
│   │   └── grpc/                      # gRPC endpoints
│   └── middleware/                    # Cross-cutting concerns
│
├── pkg/                               # Reusable libraries
│   ├── errors/                        # Custom error types
│   ├── logger/                        # Structured logging
│   └── config/                        # Config loading
│
├── api/
│   └── openapi/                       # OpenAPI specifications
│
├── scripts/
│   ├── dev/                           # Development scripts
│   └── deploy/                        # Deployment scripts
│
├── test/
│   └── integration/                   # Fabric integration tests
│
├── deployments/
│   ├── docker/                        # Docker configuration
│   └── k8s/                           # Kubernetes manifests
│
├── docs/                              # Documentation
│
├── .env.example                       # Environment template
├── .gitignore                         # Git ignore rules
├── go.mod                             # Go module definition
└── README.md                          # This file
```

---

## 📁 Folder Explanations

### `cmd/server/`

**Purpose:** Application entry point

| Aspect | Detail |
|--------|--------|
| **Fabric Relation** | Initializes SDK, loads connection profile |
| **SDK Role** | Creates SDK instance, starts event listeners |
| **Future API** | Starts HTTP/gRPC servers |

---

### `config/`

**Purpose:** External configuration files

| File | Description |
|------|-------------|
| `app.yaml` | Server port, environment, timeouts |
| `fabric.yaml` | Channel name, chaincode name, org settings |
| `logging.yaml` | Log level, format, output destinations |

**Fabric Considerations:**
- Connection profiles are **environment-specific** (dev/staging/prod)
- Peer endpoints differ per deployment
- TLS certificates are environment-dependent

---

### `internal/fabric/`

**Purpose:** All Hyperledger Fabric SDK integration code

> ⚠️ **IMPORTANT:** The SDK runs OUTSIDE the Fabric network. It connects via gRPC to peer nodes.

#### `internal/fabric/sdk/`

| Aspect | Detail |
|--------|--------|
| **Purpose** | Initialize and manage Fabric SDK instance |
| **Uses** | `github.com/hyperledger/fabric-sdk-go` |
| **Connects to** | `docube.com` network via connection profile |

#### `internal/fabric/identity/`

| Aspect | Detail |
|--------|--------|
| **Purpose** | Wallet and identity management |
| **Stores** | X.509 certificates and private keys |
| **Supports** | AdminOrgMSP, UserOrgMSP identities |

> 🔒 **Security:** Identities are EXTERNALIZED from Fabric network. Private keys must be securely stored.

#### `internal/fabric/channel/`

| Aspect | Detail |
|--------|--------|
| **Purpose** | Execute transactions on docubechannel |
| **Chaincode** | `document_nft_cc` |
| **Contracts** | `document:*` and `access:*` functions |

> ⚠️ **Note:** Chaincode permissions are enforced ON-CHAIN. This layer only submits transactions.

#### `internal/fabric/event/`

| Aspect | Detail |
|--------|--------|
| **Purpose** | Subscribe to chaincode events |
| **Events** | DocumentCreated, AccessGranted, AdminAction, etc. |
| **Use Case** | Real-time UI updates, audit logging |

#### `internal/fabric/ledger/`

| Aspect | Detail |
|--------|--------|
| **Purpose** | Query blockchain ledger directly |
| **Queries** | Block info, tx history, channel config |
| **State DB** | CouchDB for rich queries |

---

### `internal/service/`

**Purpose:** Business logic layer

| Aspect | Detail |
|--------|--------|
| **Fabric Relation** | Orchestrates calls to `internal/fabric/*` |
| **Future API** | Called by transport layer handlers |
| **Domain** | DocumentService, AccessService, AuditService |

**Docube-Specific Services:**
- `DocumentService`: CreateDocument, UpdateDocument, SoftDelete, Transfer
- `AccessService`: GrantAccess, RevokeAccess, QueryAccess
- `AuditService`: Query AdminAction events

---

### `internal/repository/`

**Purpose:** Off-chain data persistence (future)

| Aspect | Detail |
|--------|--------|
| **Role** | Complement blockchain with off-chain storage |
| **Use Cases** | Caching, search, user preferences |
| **Databases** | PostgreSQL, Redis, Elasticsearch |

> **Note:** Blockchain stores NFT records. Off-chain stores supplementary data.

---

### `internal/transport/`

**Purpose:** API layer (HTTP/gRPC)

#### `internal/transport/http/`

| Endpoint Pattern | Description |
|------------------|-------------|
| `POST /api/v1/documents` | Create document NFT |
| `PUT /api/v1/documents/:id` | Update document |
| `POST /api/v1/documents/:id/access` | Grant access |
| `DELETE /api/v1/documents/:id/access` | Revoke access |

#### `internal/transport/grpc/`

| Aspect | Detail |
|--------|--------|
| **Use Case** | High-performance, internal services |
| **Streaming** | Event subscription via streaming RPC |

---

### `internal/middleware/`

**Purpose:** Cross-cutting concerns

| Middleware | Function |
|------------|----------|
| **Auth** | JWT validation → Fabric identity mapping |
| **Logging** | Request/response logging with tx IDs |
| **Tracing** | OpenTelemetry distributed tracing |
| **Recovery** | Panic recovery |

**Fabric Integration:**
- Maps JWT claims to Fabric wallet identity
- AdminOrgMSP users → admin privileges
- UserOrgMSP users → standard privileges

---

### `pkg/`

**Purpose:** Reusable, domain-agnostic libraries

| Package | Purpose |
|---------|---------|
| `errors/` | Custom error types, chaincode error mapping |
| `logger/` | Structured logging with Fabric context |
| `config/` | Configuration loading utilities |

---

### `api/openapi/`

**Purpose:** API specification (OpenAPI 3.0)

| Aspect | Detail |
|--------|--------|
| **Use** | API documentation, client generation |
| **Models** | DocumentAssetNFT, AccessNFT (from chaincode) |

---

### `scripts/`

| Folder | Purpose |
|--------|---------|
| `dev/` | Local development: run, test, lint |
| `deploy/` | Build, Docker, K8s deployment |

---

### `test/integration/`

**Purpose:** Integration tests against real Fabric network

| Test Category | Description |
|---------------|-------------|
| SDK Connection | Verify network connectivity |
| Document Ops | Create, update, delete, transfer |
| Access Ops | Grant, revoke, query |
| Events | Event subscription and handling |

**Prerequisites:**
- Fabric network running (`docube-network`)
- Chaincode deployed (`document_nft_cc`)

---

### `deployments/`

| Folder | Purpose |
|--------|---------|
| `docker/` | Dockerfile, docker-compose |
| `k8s/` | Kubernetes deployment manifests |

**Fabric Considerations:**
- Mount connection profiles as ConfigMaps
- Mount wallet identities as Secrets
- Separate configs per environment (dev/staging/prod)

---

## 🔐 Security Considerations

| Concern | Solution |
|---------|----------|
| Identity Storage | Wallet with encrypted private keys |
| TLS | All Fabric communication over TLS |
| Authorization | Chaincode enforces OWNER/ADMIN checks |
| Audit | AdminAction events for all admin operations |
| JWT | Map to Fabric identity securely |

---

## 🔗 Related Documentation

- [Network Architecture](../fabric-samples/docs/NETWORK_ARCHITECTURE_VI.md)
- [Code Architecture](../fabric-samples/docs/CODE_ARCHITECTURE_VI.md)
- [Permission Matrix](../fabric-samples/docs/PERMISSION_MATRIX_VI.md)
- [Function Flows](../fabric-samples/docs/FUNCTION_FLOWS_VI.md)

---

## 🚀 Getting Started

```bash
# 1. Copy environment template
cp .env.example .env

# 2. Edit environment variables
vim .env

# 3. Install dependencies
go mod download

# 4. Run the service (after implementation)
go run cmd/server/main.go
```

---

## 📄 License

Proprietary - Docube Team

---

**Document Version:** 1.0  
**Created:** 2026-02-01  
**Author:** Docube Engineering Team
# docube_blockchain_service
