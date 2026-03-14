# Docube Blockchain — Giải thích cấu trúc file/folder

```
docube_blockchain/
├── Dockerfile                        # Build image chứa cả fabric-init lẫn blockchain-service
├── docker-compose.yaml               # Khởi động toàn bộ hệ thống bằng 1 lệnh
├── go.work                           # Go workspace: link 2 module (chaincode + service) để dev chung
├── go.work.sum                       # Checksum của go.work
│
├── deployments/docker/
│   ├── entrypoint-init.sh            # Script chạy khi fabric-init start:
│   │                                 #   tạo crypto, channel, install+commit chaincode
│   └── entrypoint-service.sh         # Script chạy khi blockchain-service start:
│                                     #   chờ crypto sẵn sàng rồi exec server binary
│
├── docs/                             # Tài liệu kỹ thuật (đọc thêm nếu cần)
│   ├── BLOCKCHAIN_SERVICE_VI.md      # Tổng quan service bằng tiếng Việt
│   ├── CODE_ARCHITECTURE_*.md        # Giải thích kiến trúc code (EN + VI)
│   ├── FUNCTION_FLOWS_*.md           # Luồng xử lý từng function (EN + VI)
│   ├── NETWORK_ARCHITECTURE_*.md     # Sơ đồ Fabric network (EN + VI)
│   ├── NETWORK_CONFIG_FILES_*.md     # Giải thích các file config mạng (EN + VI)
│   ├── PERMISSION_MATRIX_*.md        # Ma trận phân quyền (EN + VI)
│   ├── REBUILD_AND_TEST_*.md         # Hướng dẫn build và test (EN + VI)
│   ├── HUONG_DAN_TEST.md             # Hướng dẫn test thủ công
│   └── EUREKA_CLIENT.md              # Tài liệu Eureka client
│
├── network/
│   ├── chaincode/docube/             # Source code chaincode (Go)
│   │   ├── main.go                   # Entry point chaincode, đăng ký contracts
│   │   ├── models.go                 # Struct: DocumentAssetNFT, AccessNFT, AuditRecord
│   │   ├── document_contract.go      # Smart contract: CreateDocument, UpdateDocument, GetDocument, GetDocumentHistory
│   │   ├── access_contract.go        # Smart contract: GrantAccess, RevokeAccess, GetAccess
│   │   ├── authorization.go          # Logic kiểm tra caller có phải owner không
│   │   ├── identity_utils.go         # Tiện ích đọc MSP ID / MSPID từ stub context
│   │   ├── timeline_utils.go         # Tiện ích tạo timestamp chuẩn RFC3339
│   │   ├── validation_utils.go       # Validate input (rỗng, format...)
│   │   ├── go.mod                    # Module riêng của chaincode
│   │   └── go.sum
│   │
│   ├── configtx/
│   │   └── configtx.yaml             # Định nghĩa channel, policy, org MSP cho Fabric network
│   │
│   ├── organizations/
│   │   ├── cryptogen/                # Template để cryptogen sinh ra crypto material
│   │   │   ├── crypto-config-adminorg.yaml
│   │   │   ├── crypto-config-userorg.yaml
│   │   │   └── crypto-config-orderer.yaml
│   │   ├── fabric-ca/                # Config Fabric CA (alternative với cryptogen)
│   │   ├── ordererOrganizations/     # Crypto material của Orderer (cert, key, TLS)
│   │   └── peerOrganizations/        # Crypto material của AdminOrg và UserOrg (cert, key, TLS)
│   │                                 # ⚠️ Đây là PRIVATE KEY — không push lên public repo
│   │
│   ├── channel-artifacts/
│   │   └── docubechannel.block       # Genesis block của channel (tạo bởi configtxgen)
│   │
│   └── document_nft_cc.tar.gz        # Chaincode đã đóng gói sẵn để install lên peer
│
└── service/                          # Go service — kết nối Fabric, phục vụ HTTP + Kafka
    ├── .env.example                  # Mẫu biến môi trường (copy thành .env khi dev local)
    ├── go.mod                        # Module: github.com/horob1/docube_blockchain_service
    ├── go.sum
    │
    ├── cmd/
    │   ├── server/main.go            # Entry point service: khởi tạo tất cả, start HTTP server
    │   └── test_kafka/main.go        # Tool dev: publish message test vào Kafka (không dùng production)
    │
    ├── config/
    │   ├── app.yaml                  # Config mặc định: port, Eureka URL, Kafka brokers, Redis addr
    │   └── fabric.yaml               # Config Fabric: channel, chaincode, đường dẫn crypto
    │
    ├── deployments/docker/
    │   └── Dockerfile                # Dockerfile riêng của service (dùng khi build service độc lập)
    │
    └── internal/                     # Package nội bộ — không import được từ ngoài module
        ├── config/
        │   └── config.go             # Load config từ YAML + override bằng env var
        │                             # Thứ tự ưu tiên: env var > app.yaml/fabric.yaml
        │
        ├── fabric/client/
        │   └── client.go             # Wrapper Hyperledger Fabric Gateway SDK
        │                             # Read:  GetDocument, GetAccess, GetDocumentHistory
        │                             # Write: CreateDocument, UpdateDocument, GrantAccess, RevokeAccess
        │
        ├── kafka/
        │   ├── models.go             # Struct Kafka event: CreateDocumentEvent, GrantAccessEvent...
        │   └── consumer.go           # Consumer 4 topics song song, route đến handler tương ứng
        │                             # Topics: document.create | document.update | access.grant | access.revoke
        │
        ├── cache/
        │   ├── redis.go              # Redis client wrapper: Get/Set/Del với TTL
        │   └── access_cache.go       # Cache kết quả access check theo key "access:{docID}:{userID}"
        │                             # TTL mặc định 15 phút, bị xóa ngay khi grant/revoke
        │
        ├── eureka/
        │   └── client.go             # Đăng ký service với Eureka (Spring Cloud Discovery)
        │                             # Heartbeat mỗi 30s, tự re-register nếu bị drop
        │
        ├── service/
        │   └── document_service.go   # Business logic: kiểm tra quyền trước khi trả document
        │                             # Thứ tự check: owner? → cache? → blockchain AccessNFT
        │
        └── transport/http/
            └── handler.go            # HTTP handler: đăng ký routes, parse request, format response
                                      # GET /api/v1/blockchain/health
                                      # GET /api/v1/blockchain/documents/{id}
                                      # GET /api/v1/blockchain/documents/{id}/history
```

---

## Luồng dữ liệu tóm tắt

```
WRITE (qua Kafka):
  Service khác → Kafka topic → consumer.go → fabric/client → Chaincode → Ledger

READ (qua HTTP):
  Client → Gateway (inject X-User-Id) → handler.go → document_service.go
         → cache HIT  → trả về ngay
         → cache MISS → fabric/client → Chaincode → cache result → trả về
```

## Tại sao dùng `internal/`?

Go convention: package trong `internal/` chỉ có thể import bởi code trong cùng module.
Ngăn service khác import trực tiếp implementation detail của blockchain service.
