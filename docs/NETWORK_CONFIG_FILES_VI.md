# CÁC FILE CẤU HÌNH MẠNG - Docube Fabric Network

**Phiên bản tài liệu:** 1.0  
**Cập nhật lần cuối:** 2026-02-01

---

## Mục đích
Tài liệu này giải thích tất cả các file cấu hình liên quan đến mạng trong Docube Fabric Network.

## Phạm vi
- configtx.yaml
- Docker Compose files
- Cấu hình CouchDB
- Network scripts

## Tài liệu liên quan
- [NETWORK_ARCHITECTURE_VI.md](NETWORK_ARCHITECTURE_VI.md)

---

## 1. configtx.yaml

**Vị trí:** `configtx/configtx.yaml`  
**Mục đích:** Định nghĩa cấu hình channel, tổ chức và policies

### 1.1 Phần Organizations

```yaml
Organizations:
  - &OrdererOrg
    Name: OrdererOrg
    ID: OrdererMSP
    MSPDir: ../organizations/ordererOrganizations/docube.com/msp
    
  - &AdminOrg
    Name: AdminOrgMSP
    ID: AdminOrgMSP
    MSPDir: ../organizations/peerOrganizations/adminorg.docube.com/msp
    
  - &UserOrg
    Name: UserOrgMSP
    ID: UserOrgMSP
    MSPDir: ../organizations/peerOrganizations/userorg.docube.com/msp
```

**Ý nghĩa bảo mật:**
- MSPDir phải trỏ đến thư mục chứng chỉ hợp lệ
- MSP ID phải là duy nhất trong mạng
- Policies kiểm soát quyền read/write/admin

### 1.2 Các Policy Quan trọng

| Policy | AdminOrg | UserOrg | Ảnh hưởng |
|--------|----------|---------|-----------|
| Readers | Tất cả thành viên | Tất cả thành viên | Ai có thể query |
| Writers | Admin + Client | Chỉ Admin | Ai có thể gửi tx |
| Admins | Chỉ Admin | Chỉ Admin | Quản trị channel |
| Endorsement | Peer | Peer | Ai ký xác nhận |

### 1.3 Application Policies (Quan trọng)

```yaml
Application:
  Policies:
    Endorsement:
      Type: Signature
      Rule: "OR('AdminOrgMSP.peer')"  # Chỉ AdminOrg có thể endorse
    LifecycleEndorsement:
      Type: Signature
      Rule: "OR('AdminOrgMSP.peer')"  # Chỉ AdminOrg deploy chaincode
```

**Lưu ý bảo mật:** Đảm bảo chaincode writes đi qua AdminOrg peer.

---

## 2. Docker Compose Files

### 2.1 compose-docube-net.yaml

**Vị trí:** `compose/compose-docube-net.yaml`  
**Mục đích:** Định nghĩa tất cả Fabric containers

#### Các dịch vụ:

| Dịch vụ | Image | Cổng | Mục đích |
|---------|-------|------|----------|
| orderer.docube.com | fabric-orderer:latest | 7050, 7053 | Sắp xếp transaction |
| peer0.adminorg | fabric-peer:latest | 7051 | Peer AdminOrg |
| peer0.userorg | fabric-peer:latest | 9051 | Peer UserOrg |

#### Biến môi trường chính:

```yaml
# Orderer
ORDERER_GENERAL_TLS_ENABLED=true
ORDERER_GENERAL_LOCALMSPID=OrdererMSP
ORDERER_CHANNELPARTICIPATION_ENABLED=true

# Peer
CORE_PEER_TLS_ENABLED=true
CORE_PEER_LOCALMSPID=AdminOrgMSP
CORE_PEER_MSPCONFIGPATH=/etc/hyperledger/fabric/msp
```

### 2.2 compose-couch.yaml

**Vị trí:** `compose/compose-couch.yaml`  
**Mục đích:** Định nghĩa CouchDB containers cho state database

```yaml
services:
  couchdb0.adminorg:
    image: couchdb:3.3.2
    environment:
      - COUCHDB_USER=admin
      - COUCHDB_PASSWORD=adminpw
    ports:
      - "5984:5984"

  couchdb0.userorg:
    image: couchdb:3.3.2
    ports:
      - "7984:5984"
```

#### Cấu hình CouchDB cho Peer:

```yaml
peer0.adminorg:
  environment:
    - CORE_LEDGER_STATE_STATEDATABASE=CouchDB
    - CORE_LEDGER_STATE_COUCHDBCONFIG_COUCHDBADDRESS=couchdb0.adminorg:5984
    - CORE_LEDGER_STATE_COUCHDBCONFIG_USERNAME=admin
    - CORE_LEDGER_STATE_COUCHDBCONFIG_PASSWORD=adminpw
```

---

## 3. Network Scripts

### 3.1 network.sh

**Vị trí:** `network.sh`  
**Mục đích:** Script chính quản lý mạng

#### Các lệnh:

| Lệnh | Mô tả |
|------|-------|
| `./network.sh up` | Khởi động containers |
| `./network.sh down` | Dừng và xóa mạng |
| `./network.sh createChannel` | Tạo docubechannel |

#### Biến chính:

```bash
DATABASE="couchdb"           # State database
CHANNEL_NAME="docubechannel" # Tên channel
COMPOSE_FILE_COUCH="compose-couch.yaml"
```

### 3.2 setEnv.sh

**Vị trí:** `setEnv.sh`  
**Mục đích:** Thiết lập môi trường cho peer CLI

```bash
# Cách dùng
source setEnv.sh adminorg  # Thiết lập môi trường AdminOrg
source setEnv.sh userorg   # Thiết lập môi trường UserOrg

# Các biến được thiết lập:
CORE_PEER_ADDRESS=localhost:7051
CORE_PEER_LOCALMSPID=AdminOrgMSP
CORE_PEER_TLS_ROOTCERT_FILE=...
CORE_PEER_MSPCONFIGPATH=...
```

### 3.3 deployCC.sh

**Vị trí:** `scripts/deployCC.sh`  
**Mục đích:** Deploy chaincode lên channel

```bash
# Cách dùng
./scripts/deployCC.sh <channel> <cc_name> <cc_path> <lang> <version> <sequence>

# Ví dụ
./scripts/deployCC.sh docubechannel document_nft_cc ./chaincode/docube golang 5.0 2
```

#### Các bước Lifecycle:
1. Đóng gói chaincode
2. Cài đặt trên AdminOrg peer
3. Cài đặt trên UserOrg peer
4. Phê duyệt cho AdminOrg
5. Phê duyệt cho UserOrg
6. Commit lên channel

---

## 4. Cấu trúc Chứng chỉ

### 4.1 Thư mục Tổ chức

```
organizations/
├── ordererOrganizations/
│   └── docube.com/
│       ├── ca/                    # Chứng chỉ Root CA
│       ├── msp/                   # MSP tổ chức
│       │   ├── cacerts/           # Chứng chỉ CA
│       │   ├── tlscacerts/        # Chứng chỉ TLS CA
│       │   └── config.yaml        # Cấu hình MSP
│       └── orderers/
│           └── orderer.docube.com/
│               ├── msp/           # MSP Orderer
│               └── tls/           # Chứng chỉ TLS
└── peerOrganizations/
    ├── adminorg.docube.com/
    │   ├── ca/
    │   ├── msp/
    │   ├── peers/peer0.adminorg.docube.com/
    │   └── users/Admin@adminorg.docube.com/
    └── userorg.docube.com/
        └── ...
```

### 4.2 Best Practices Bảo mật

| Mục | Khuyến nghị |
|-----|-------------|
| Private keys | Không bao giờ commit lên git |
| TLS | Luôn bật |
| Credentials | Sử dụng secrets management |
| CA | Cân nhắc Fabric CA cho production |

---

## 5. Tóm tắt Cấu hình

| File | Vị trí | Mục đích |
|------|--------|----------|
| configtx.yaml | configtx/ | Định nghĩa Channel/Org |
| compose-docube-net.yaml | compose/ | Định nghĩa Container |
| compose-couch.yaml | compose/ | Containers CouchDB |
| network.sh | ./ | Quản lý mạng |
| setEnv.sh | ./ | Môi trường CLI |
| deployCC.sh | scripts/ | Deploy chaincode |

---

## Lịch sử Tài liệu

| Phiên bản | Ngày | Tác giả | Thay đổi |
|-----------|------|---------|----------|
| 1.0 | 2026-02-01 | Đội Docube | Tài liệu ban đầu |
