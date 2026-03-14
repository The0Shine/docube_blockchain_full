# Docube Blockchain — Hướng dẫn vận hành

## Mục lục

1. [Tổng quan kiến trúc](#1-tổng-quan-kiến-trúc)
2. [Cấu trúc thư mục](#2-cấu-trúc-thư-mục)
3. [Giải thích các Docker containers](#3-giải-thích-các-docker-containers)
4. [Máy mới — Cài đặt lần đầu](#4-máy-mới--cài-đặt-lần-đầu)
5. [Các lần chạy sau](#5-các-lần-chạy-sau)
6. [Chạy bản thường (Native Go binary)](#6-chạy-bản-thường-native-go-binary)
7. [Chạy bản Docker](#7-chạy-bản-docker)
8. [Test API](#8-test-api)
9. [Dừng dịch vụ](#9-dừng-dịch-vụ)
10. [Reset toàn bộ](#10-reset-toàn-bộ)
11. [Troubleshooting](#11-troubleshooting)

---

## 1. Tổng quan kiến trúc

```
docube_blockchain/
├── docker-compose.yaml     ← 1 lệnh khởi động toàn bộ Fabric stack
├── Dockerfile              ← Build image horob1/docube-blockchain:latest
├── service/                ← Go blockchain service
└── network/                ← Hyperledger Fabric network config + crypto
```

```
┌─────────────────────────────────────────────────────────────────┐
│                         Windows Host                            │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  Docker (horob1_docub) — Redis + Kafka                   │   │
│  │   ┌──────────┐   ┌─────────────────┐   ┌─────────────┐  │   │
│  │   │  Redis   │   │      Kafka      │   │  Kafka UI   │  │   │
│  │   │  :6379   │   │  :7092 / :7094  │   │    :7080    │  │   │
│  │   └──────────┘   └─────────────────┘   └─────────────┘  │   │
│  └──────────────────────────────────────────────────────────┘   │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  Docker (docube_network) — Hyperledger Fabric            │   │
│  │                                                          │   │
│  │   ┌───────────────┐   ┌───────────────┐                  │   │
│  │   │    Orderer    │   │  CouchDB x2   │                  │   │
│  │   │    :7050      │   │  :5984/:7984  │                  │   │
│  │   └───────────────┘   └───────────────┘                  │   │
│  │   ┌───────────────┐   ┌───────────────┐                  │   │
│  │   │ Peer AdminOrg │   │  Peer UserOrg │                  │   │
│  │   │    :7051      │   │    :9051      │                  │   │
│  │   └───────────────┘   └───────────────┘                  │   │
│  │   [Chaincode containers — tự spawn bởi peer]              │   │
│  │                                                          │   │
│  │   ┌──────────────────────────────────────────────────┐   │   │
│  │   │  [Docker] blockchain-service  :8080              │   │   │
│  │   └──────────────────────────────────────────────────┘   │   │
│  └──────────────────────────────────────────────────────────┘   │
│                                                                 │
│  ┌──────────────────────────────────────┐                       │
│  │  WSL2 Ubuntu                         │                       │
│  │  [Native] ./service/server  :8081    │                       │
│  └──────────────────────────────────────┘                       │
└─────────────────────────────────────────────────────────────────┘
```

### Ports tóm tắt

| Service | Port | Ghi chú |
|---------|------|---------|
| Blockchain Service (native) | **8081** | Go binary chạy trong WSL |
| Blockchain Service (Docker) | **8080** | Container trong docube_network |
| Peer AdminOrg | 7051 | gRPC endpoint cho service |
| Orderer | 7050 | Xử lý transaction |
| CouchDB AdminOrg | 5984 | Xem ledger state |
| CouchDB UserOrg | 7984 | Xem ledger state |
| Kafka | 7092 | External / 7094 Internal |
| Kafka UI | 7080 | Quản lý topics |
| Redis | 6379 | Cache quyền truy cập |

---

## 2. Cấu trúc thư mục

```
docube_blockchain/
│
├── docker-compose.yaml          # Full stack: Fabric + blockchain service
├── Dockerfile                   # Build horob1/docube-blockchain:latest
│                                # (chứa Fabric tools + Go binary)
│
├── service/                     # Go blockchain service
│   ├── cmd/server/main.go       # Entrypoint
│   ├── config/
│   │   ├── app.yaml             # Config bản thường (WSL native)
│   │   └── fabric.yaml          # Fabric config bản thường
│   ├── internal/
│   │   ├── fabric/client/       # Kết nối Fabric qua gRPC/TLS
│   │   ├── service/             # Business logic (access check, document query)
│   │   ├── cache/               # Redis cache cho access control
│   │   ├── kafka/               # Consumer 4 topics
│   │   └── transport/http/      # HTTP handlers
│   └── deployments/docker/
│       └── Dockerfile           # Build chỉ Go service (không có Fabric tools)
│
└── network/                     # Hyperledger Fabric network
    ├── organizations/           # Crypto material (cert, key, TLS)
    │   ├── peerOrganizations/
    │   └── ordererOrganizations/
    ├── channel-artifacts/       # Genesis block, channel config
    ├── compose/docker/peercfg/  # core.yaml cho peers
    ├── chaincode/               # Smart contract source (document_nft_cc)
    ├── scripts/                 # deployCC.sh, etc.
    └── network.sh               # Script quản lý network
```

### Config files theo môi trường

| File | Bản | Kafka/Redis | Fabric Peer | Crypto path |
|------|-----|-------------|-------------|-------------|
| `service/config/app.yaml` | Native (WSL) | `172.31.16.1:*` | — | — |
| `service/config/fabric.yaml` | Native (WSL) | — | `localhost:7051` | `../network/organizations/...` |
| Env vars trong docker-compose | Docker | `kafka:7094`, `redis:6379` | `peer0.adminorg...:7051` | `/crypto` (volume) |

---

## 3. Giải thích các Docker containers

### Containers Hyperledger Fabric

| Container | Vai trò |
|-----------|---------|
| `orderer.docube.com` | Sắp xếp thứ tự transaction, tạo block, lưu vào ledger |
| `peer0.adminorg.docube.com` | Peer của AdminOrg — endorse transaction, lưu state |
| `peer0.userorg.docube.com` | Peer của UserOrg — endorse transaction, lưu state |
| `couchdb0.adminorg` | State database (key-value) của AdminOrg peer |
| `couchdb0.userorg` | State database (key-value) của UserOrg peer |
| `docube-fabric-init` | Chạy 1 lần: tạo channel + deploy chaincode, rồi exit 0 |
| `docube-blockchain-service` | Go service chạy trong Docker (port 8080) |

### Chaincode containers — tại sao có?

Khi `docker ps` sẽ thấy các container tên dạng:

```
dev-peer0.adminorg.docube.com-document_nft_cc_1.0-f8119989...
dev-peer0.userorg.docube.com-document_nft_cc_1.0-f8119989...
```

**Đây là hành vi hoàn toàn bình thường của Hyperledger Fabric.**

Trong Fabric, chaincode **không chạy bên trong peer container** mà được peer tự động **spawn ra một Docker container riêng** khi có transaction đầu tiên. Pattern tên:

```
dev-{tên peer}-{tên chaincode}_{version}-{package hash}
```

Vì có 2 peer (AdminOrg + UserOrg) cùng cài chaincode → luôn có **2 chaincode containers**. Mỗi peer endorses transaction độc lập thông qua chaincode container của mình.

Những containers này:
- Tự tạo khi peer nhận transaction đầu tiên
- Tự xóa khi Fabric network down
- Không cần quản lý thủ công

---

## 4. Máy mới — Cài đặt lần đầu

### Yêu cầu

| Công cụ | Phiên bản | Ghi chú |
|---------|-----------|---------|
| Docker Desktop | Latest | Bật WSL2 integration |
| WSL2 Ubuntu | — | Có sẵn khi cài Docker Desktop |
| Go | 1.23+ | Chỉ cần cho bản native |

Kiểm tra Go trong WSL:
```bash
# WSL Ubuntu
go version   # phải thấy go1.23.x
```

---

### Bước 1 — Tạo Docker network

```powershell
# PowerShell — chỉ chạy 1 lần, tồn tại vĩnh viễn
docker network create horob1_docub
```

---

### Bước 2 — Start Redis + Kafka

```powershell
cd C:\Users\Public\Documents\code\docube\docube\Redis
docker compose up -d

cd C:\Users\Public\Documents\code\docube\docube\Message-Queue
docker compose up -d
```

Kiểm tra:
```powershell
docker ps --format "table {{.Names}}\t{{.Status}}"
# horob1_redis   Up ...
# kafka          Up ...
# kafka-ui       Up ...
```

---

### Bước 3 — Bootstrap Fabric network *(chỉ 1 lần duy nhất)*

```powershell
cd C:\Users\Public\Documents\code\docube\docube\docube_blockchain
docker compose up -d
```

Lệnh này tự động:
1. Build image `horob1/docube-blockchain:latest` (~3-5 phút lần đầu)
2. Tạo crypto material (cert, key, TLS) → lưu vào Docker volume `docube_blockchain_fabric-crypto`
3. Tạo channel `docubechannel`
4. Deploy chaincode `document_nft_cc v1.0`
5. Start toàn bộ Fabric containers + blockchain service (port 8080)

**Đợi `docube-fabric-init` hoàn thành** (STATUS = `Exited (0)`):
```powershell
docker ps -a --filter "name=docube-fabric-init" --format "table {{.Names}}\t{{.Status}}"
```

---

### Bước 4 — Sync crypto về disk *(chỉ cần cho bản native)*

> Bỏ qua nếu chỉ dùng bản Docker.

Crypto material được `fabric-init` tạo ra và lưu trong **Docker volume** `docube_blockchain_fabric-crypto`. Bản native (Go binary) cần đọc từ disk tại `network/organizations/`. Cần copy 1 lần:

```powershell
# PowerShell
$BASE = "C:\Users\Public\Documents\code\docube\docube\docube_blockchain\network\organizations"

docker cp peer0.adminorg.docube.com:/var/hyperledger/crypto/peerOrganizations/adminorg.docube.com/. `
  "$BASE\peerOrganizations\adminorg.docube.com\"

docker cp peer0.adminorg.docube.com:/var/hyperledger/crypto/ordererOrganizations/. `
  "$BASE\ordererOrganizations\"
```

> **Lưu ý:** Phải làm lại bước này mỗi khi reset Fabric (xóa volumes).

---

### Bước 5 — Build Go binary *(chỉ cần cho bản native)*

```bash
# WSL Ubuntu
cd /mnt/c/Users/Public/Documents/code/docube/docube/docube_blockchain/service
go build -o server ./cmd/server/main.go
```

---

## 5. Các lần chạy sau

Data đã có trong Docker volumes, **không cần bootstrap lại**.

```powershell
# 1. Redis + Kafka
cd C:\Users\Public\Documents\code\docube\docube\Redis && docker compose up -d
cd C:\Users\Public\Documents\code\docube\docube\Message-Queue && docker compose up -d

# 2. Toàn bộ Fabric + blockchain service (1 lệnh)
cd C:\Users\Public\Documents\code\docube\docube\docube_blockchain
docker compose up -d blockchain-service
```

Docker Compose tự động start couchdb → orderer → peers (chờ healthy) → blockchain-service.

Sau đó chạy blockchain service — bản native [mục 6](#6-chạy-bản-thường-native-go-binary) hoặc bản Docker [mục 7](#7-chạy-bản-docker).

---

## 6. Chạy bản thường (Native Go binary)

> Port: **8081** | Chạy trong WSL Ubuntu

### Foreground (xem log trực tiếp)

```bash
# WSL Ubuntu
cd /mnt/c/Users/Public/Documents/code/docube/docube/docube_blockchain/service
./server
```

### Background

```bash
# WSL Ubuntu
cd /mnt/c/Users/Public/Documents/code/docube/docube/docube_blockchain/service
setsid ./server > /mnt/c/Users/Public/Documents/code/docube/docube_blockchain/blockchain_native.log 2>&1 &
echo "PID: $!"

# Xem log
tail -f /mnt/c/Users/Public/Documents/code/docube/docube_blockchain/blockchain_native.log

# Dừng
pkill server
```

### Startup log mong đợi

```
🚀 Docube Blockchain Service Starting...
   App Port: 8081
   Fabric Channel: docubechannel
[FABRIC] ✅ Successfully connected to Fabric network
[REDIS] Connected to 172.31.16.1:6379
[KAFKA] 📥 Listening on topic: docube.document.create
✅ Service is running and ready!
   HTTP API: http://localhost:8081/api/v1/blockchain
```

---

## 7. Chạy bản Docker

> Port: **8080** | Container `docube-blockchain-service`

Bản Docker được khởi động cùng với `docker compose up -d` ở Bước 3. Nếu cần restart riêng:

```powershell
# Dừng và xóa container service (không ảnh hưởng Fabric)
docker stop docube-blockchain-service
docker rm docube-blockchain-service

# Start lại toàn bộ (Fabric containers không bị recreate vì đã có)
cd C:\Users\Public\Documents\code\docube\docube\docube_blockchain
docker compose up -d blockchain-service
```

Xem log:
```powershell
docker logs docube-blockchain-service --follow
```

Rebuild sau khi thay đổi code:
```powershell
cd C:\Users\Public\Documents\code\docube\docube\docube_blockchain
docker compose build
docker compose up -d blockchain-service
```

---

## 8. Test API

| Bản | Base URL |
|-----|---------|
| Native | `http://localhost:8081/api/v1/blockchain` |
| Docker | `http://localhost:8080/api/v1/blockchain` |

Các ví dụ bên dưới dùng port **8081** (native). Thay `8081` → `8080` cho bản Docker.

---

### 8.1 Tạo test document

Trước khi test GET, cần có document trên blockchain. Dùng `docker exec` vào peer container:

```powershell
docker exec peer0.adminorg.docube.com sh -c "
CRYPTO=/var/hyperledger/crypto
PEER_TLS=\$CRYPTO/peerOrganizations/adminorg.docube.com/peers/peer0.adminorg.docube.com/tls/ca.crt
ORDERER_CA=\$CRYPTO/ordererOrganizations/docube.com/orderers/orderer.docube.com/msp/tlscacerts/tlsca.docube.com-cert.pem
ADMIN_MSP=\$CRYPTO/peerOrganizations/adminorg.docube.com/users/Admin@adminorg.docube.com/msp

export CORE_PEER_TLS_ENABLED=true
export CORE_PEER_LOCALMSPID=AdminOrgMSP
export CORE_PEER_TLS_ROOTCERT_FILE=\$PEER_TLS
export CORE_PEER_MSPCONFIGPATH=\$ADMIN_MSP
export CORE_PEER_ADDRESS=localhost:7051

peer chaincode invoke \
  -o orderer.docube.com:7050 \
  --ordererTLSHostnameOverride orderer.docube.com \
  --tls --cafile \$ORDERER_CA \
  -C docubechannel -n document_nft_cc \
  --peerAddresses localhost:7051 --tlsRootCertFiles \$PEER_TLS \
  -c '{\"function\":\"document:CreateDocument\",\"Args\":[\"doc-test-001\",\"e3b0c44298fc1c149afbf4c8996fb924\",\"SHA256\",\"user-alice-uuid-001\"]}'
"
```

Đợi 3 giây cho block commit:
```bash
sleep 3
```

---

### 8.2 Health Check

```bash
curl http://localhost:8081/api/v1/blockchain/health
```

**HTTP 200:**
```json
{ "success": true, "data": { "status": "UP" } }
```

---

### 8.3 GET Document

#### Owner truy cập → 200 OK

```bash
curl -H "X-User-Id: user-alice-uuid-001" \
     http://localhost:8081/api/v1/blockchain/documents/doc-test-001
```

**HTTP 200:**
```json
{
  "success": true,
  "data": {
    "assetId": "DOC-doc-test-001",
    "documentId": "doc-test-001",
    "docHash": "e3b0c44298fc1c149afbf4c8996fb924",
    "hashAlgo": "SHA256",
    "systemUserId": "user-alice-uuid-001",
    "version": 1,
    "status": "ACTIVE",
    "createdAt": "...",
    "updatedAt": "..."
  }
}
```

#### Không có X-User-Id → 401 Unauthorized

```bash
curl http://localhost:8081/api/v1/blockchain/documents/doc-test-001
```

**HTTP 401:**
```json
{
  "success": false,
  "error": { "code": "UNAUTHORIZED", "message": "Missing X-User-Id header" }
}
```

#### User không có quyền → 403 Forbidden

```bash
curl -H "X-User-Id: user-bob-999" \
     http://localhost:8081/api/v1/blockchain/documents/doc-test-001
```

**HTTP 403:**
```json
{
  "success": false,
  "error": { "code": "ACCESS_DENIED", "message": "access denied: NOT_GRANTED" }
}
```

#### Document không tồn tại → 404 Not Found

```bash
curl -H "X-User-Id: user-alice-uuid-001" \
     http://localhost:8081/api/v1/blockchain/documents/nonexistent-doc
```

**HTTP 404:**
```json
{
  "success": false,
  "error": { "code": "NOT_FOUND", "message": "document not found: nonexistent-doc" }
}
```

---

### 8.4 GET Document History

#### Owner xem lịch sử → 200 OK

```bash
curl -H "X-User-Id: user-alice-uuid-001" \
     http://localhost:8081/api/v1/blockchain/documents/doc-test-001/history
```

**HTTP 200:**
```json
{
  "success": true,
  "data": [
    {
      "txId": "474004d457eb9e72...",
      "timestamp": "2026-03-13T21:18:01Z",
      "value": {
        "assetId": "DOC-doc-test-001",
        "status": "ACTIVE",
        "version": 1,
        ...
      },
      "isDelete": false
    }
  ]
}
```

#### Thiếu X-User-Id → 400 Bad Request

```bash
curl http://localhost:8081/api/v1/blockchain/documents/doc-test-001/history
```

**HTTP 400:**
```json
{
  "success": false,
  "error": { "code": "INVALID_REQUEST", "message": "Document ID and X-User-Id are required" }
}
```

#### User không có quyền xem history → 403 Forbidden

```bash
curl -H "X-User-Id: user-bob-999" \
     http://localhost:8081/api/v1/blockchain/documents/doc-test-001/history
```

**HTTP 403:**
```json
{
  "success": false,
  "error": { "code": "ACCESS_DENIED", "message": "access denied: NOT_GRANTED" }
}
```

#### Document không tồn tại → 404 Not Found

```bash
curl -H "X-User-Id: user-alice-uuid-001" \
     http://localhost:8081/api/v1/blockchain/documents/nonexistent-doc/history
```

**HTTP 404:**
```json
{
  "success": false,
  "error": { "code": "NOT_FOUND", "message": "document not found: nonexistent-doc" }
}
```

---

### 8.5 Bảng tóm tắt tất cả test cases

| # | Endpoint | X-User-Id | HTTP | Mô tả |
|---|----------|-----------|------|-------|
| 1 | `GET /health` | — | **200** | Service đang chạy |
| 2 | `GET /documents/{id}` | _(thiếu)_ | **401** | Không có header |
| 3 | `GET /documents/{id}` | owner | **200** | Owner đọc tài liệu của mình |
| 4 | `GET /documents/{id}` | stranger | **403** | Không có quyền |
| 5 | `GET /documents/{id}` | anyone | **404** | Document không tồn tại |
| 6 | `GET /documents/{id}/history` | _(thiếu)_ | **400** | Thiếu cả id lẫn header |
| 7 | `GET /documents/{id}/history` | owner | **200** | Xem toàn bộ tx history |
| 8 | `GET /documents/{id}/history` | stranger | **403** | Không có quyền |
| 9 | `GET /documents/{id}/history` | anyone | **404** | Document không tồn tại |

---

### 8.6 Chạy toàn bộ test tự động (bản native)

```bash
# WSL Ubuntu
cd /mnt/c/Users/Public/Documents/code/docube/docube
bash fulltest.sh

# Xem kết quả
cat fulltest.txt
```

---

## 9. Dừng dịch vụ

### Dừng blockchain service

```bash
# Bản native (WSL)
pkill server

# Bản Docker — chỉ dừng service, giữ Fabric
docker stop docube-blockchain-service
```

### Dừng Fabric *(giữ nguyên data)*

```powershell
# Dùng stop — KHÔNG dùng rm hoặc down -v
docker stop peer0.adminorg.docube.com peer0.userorg.docube.com `
             orderer.docube.com couchdb0.adminorg couchdb0.userorg
```

### Dừng Redis + Kafka

```powershell
cd C:\Users\Public\Documents\code\docube\docube\Redis
docker compose down

cd C:\Users\Public\Documents\code\docube\docube\Message-Queue
docker compose down
```

### Dừng tất cả

```powershell
# Service
docker stop docube-blockchain-service

# Fabric
docker stop peer0.adminorg.docube.com peer0.userorg.docube.com `
             orderer.docube.com couchdb0.adminorg couchdb0.userorg

# Infrastructure
cd C:\Users\Public\Documents\code\docube\docube\Redis && docker compose down
cd C:\Users\Public\Documents\code\docube\docube\Message-Queue && docker compose down
```

---

## 10. Reset toàn bộ

> ⚠️ Xóa toàn bộ dữ liệu ledger. Phải bootstrap lại từ Bước 3.

```powershell
# Xóa Fabric containers + volumes
cd C:\Users\Public\Documents\code\docube\docube\docube_blockchain
docker compose down -v

# Xóa chaincode containers và images
docker ps -a --filter "name=dev-peer" -q | ForEach-Object { docker rm $_ }
docker images --filter "reference=dev-peer*" -q | ForEach-Object { docker rmi $_ }
```

Sau đó làm lại từ [Bước 3](#bước-3--bootstrap-fabric-network-chỉ-1-lần-duy-nhất) và [Bước 4](#bước-4--sync-crypto-về-disk-chỉ-cần-cho-bản-native).

---

## 11. Troubleshooting

### TLS: certificate signed by unknown authority

**Nguyên nhân:** Crypto trên disk (`network/organizations/`) cũ hơn crypto trong Docker volume (sau reset).

**Giải pháp:** Chạy lại Bước 4 (sync crypto).

```powershell
$BASE = "C:\Users\Public\Documents\code\docube\docube\docube_blockchain\network\organizations"
docker cp peer0.adminorg.docube.com:/var/hyperledger/crypto/peerOrganizations/adminorg.docube.com/. `
  "$BASE\peerOrganizations\adminorg.docube.com\"
```

---

### listen tcp :8081: address already in use

```bash
# WSL
fuser -k 8081/tcp
```

---

### Kafka: EOF / Cannot get controller

Bình thường khi Kafka vừa khởi động. Service tự retry. Đợi ~10 giây, log phải chuyển sang `📥 Listening on topic`.

---

### fabric-init exit khác 0

```powershell
docker logs docube-fabric-init
```

Nếu lỗi liên quan đến channel/chaincode đã tồn tại → bình thường, không cần xử lý. Nếu lỗi khác → reset và chạy lại từ đầu ([mục 10](#10-reset-toàn-bộ)).

---

### Peer không healthy sau docker start

Đảm bảo CouchDB được start **trước** peer:

```powershell
docker start couchdb0.adminorg couchdb0.userorg
Start-Sleep 5
docker start orderer.docube.com peer0.adminorg.docube.com peer0.userorg.docube.com
```
#   d o c u b e _ f a _ s e r v i c e  
 #   d o c u b e _ f a _ s e r v i c e  
 #   d o c u b e _ f a _ s e r v i c e  
 #   d o c u b e _ f a _ s e r v i c e  
 #   d o c u b e _ f a _ s e r v i c e  
 #   d o c u b e _ b l o c k c h a i n _ f u l l  
 #   d o c u b e _ b l o c k c h a i n _ f u l l  
 #   d o c u b e _ b l o c k c h a i n _ f u l l  
 #   d o c u b e _ b l o c k c h a i n _ f u l l l  
 #   d o c u b e _ b l o c k c h a i n _ f u l l l  
 #   d o c u b e _ b l o c k c h a i n _ f u l l l  
 #   d o c u b e _ b l o c k c h a i n _ f u l l  
 