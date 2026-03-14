# HƯỚNG DẪN REBUILD VÀ TEST - Docube Fabric Network

**Phiên bản tài liệu:** 1.0  
**Cập nhật lần cuối:** 2026-02-01

---

## Mục đích
Tài liệu này cung cấp hướng dẫn từng bước để rebuild mạng từ đầu và chạy các test validation.

## Phạm vi
- Dọn dẹp Docker
- Rebuild mạng
- Tạo lại channel
- Deploy chaincode
- Validation test

## Đối tượng
- Kỹ sư DevOps
- QA Engineers
- Lập trình viên

## Tài liệu liên quan
- [NETWORK_ARCHITECTURE_VI.md](NETWORK_ARCHITECTURE_VI.md)
- [PERMISSION_MATRIX_VI.md](PERMISSION_MATRIX_VI.md)

---

## 1. Yêu cầu Hệ thống

### 1.1 Yêu cầu Tối thiểu

| Yêu cầu | Tối thiểu |
|---------|-----------|
| Docker | 20.10+ |
| Docker Compose | 2.0+ |
| Go | 1.21+ |
| Node.js (tùy chọn) | 18+ |
| RAM | 4GB |
| Ổ đĩa | 10GB trống |

### 1.2 Fabric Binaries

```bash
# Xác minh Fabric binaries đã cài đặt
ls ~/fabric-samples/bin/
# Mong đợi: peer, orderer, configtxgen, cryptogen, v.v.
```

---

## 2. Dọn dẹp Mạng Hoàn toàn

### 2.1 Dừng Tất cả Containers

```bash
cd ~/fabric-samples/docube-network

# Dừng mạng và xóa containers
./network.sh down
```

### 2.2 Dọn dẹp Docker (Sâu)

```bash
# Xóa tất cả containers liên quan fabric
docker rm -f $(docker ps -aq --filter "label=service=hyperledger-fabric") 2>/dev/null

# Xóa tất cả chaincode containers
docker rm -f $(docker ps -aq --filter "name=dev-peer") 2>/dev/null

# Xóa chaincode images
docker rmi -f $(docker images -q "dev-peer*") 2>/dev/null

# Xóa volumes
docker volume prune -f

# Xóa CouchDB volumes cụ thể
docker volume rm $(docker volume ls -q | grep docube) 2>/dev/null
```

### 2.3 Xóa Artifacts Đã Tạo

```bash
cd ~/fabric-samples/docube-network

# Xóa crypto materials (sẽ được tạo lại)
rm -rf organizations/peerOrganizations
rm -rf organizations/ordererOrganizations

# Xóa channel artifacts
rm -rf channel-artifacts/*

# Xóa chaincode package
rm -f *.tar.gz
```

---

## 3. Rebuild Mạng

### 3.1 Tạo Crypto Materials

```bash
cd ~/fabric-samples/docube-network

# Sử dụng cryptogen (chế độ phát triển)
cryptogen generate --config=./organizations/cryptogen/crypto-config-orderer.yaml --output="organizations"
cryptogen generate --config=./organizations/cryptogen/crypto-config-adminorg.yaml --output="organizations"
cryptogen generate --config=./organizations/cryptogen/crypto-config-userorg.yaml --output="organizations"
```

### 3.2 Khởi động Network Containers

```bash
# Khởi động với CouchDB
./network.sh up

# Xác minh containers đang chạy
docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
```

Kết quả mong đợi:
```
NAMES                        STATUS     PORTS
orderer.docube.com          Up          7050, 7053
peer0.adminorg.docube.com   Up          7051
peer0.userorg.docube.com    Up          9051
couchdb0.adminorg           Up          5984
couchdb0.userorg            Up          7984
```

### 3.3 Tạo Channel

```bash
# Tạo channel
./network.sh createChannel

# Xác minh channel đã tạo
source setEnv.sh adminorg
peer channel list
# Mong đợi: docubechannel
```

---

## 4. Deploy Chaincode

### 4.1 Deploy Chaincode

```bash
cd ~/fabric-samples/docube-network

# Deploy chaincode v5.0
./scripts/deployCC.sh docubechannel document_nft_cc ./chaincode/docube golang 5.0 1
```

### 4.2 Xác minh Deployment

```bash
# Kiểm tra chaincode đã commit
source setEnv.sh adminorg
peer lifecycle chaincode querycommitted -C docubechannel

# Kết quả mong đợi:
# Name: document_nft_cc, Version: 5.0, Sequence: 1
```

---

## 5. Validation Tests

### 5.1 Chạy Test Permission Tự động

```bash
cd ~/fabric-samples/docube-network
chmod +x ./tests/permission_test.sh
./tests/permission_test.sh
```

### 5.2 Kết quả Test Mong đợi

| Section | Số Tests | Mong đợi |
|---------|----------|----------|
| USER có thể tạo | 2 | Tất cả PASS |
| OWNER có thể sửa của mình | 4 | Tất cả PASS |
| NON-OWNER bị từ chối | 4 | Tất cả PASS (ERR_UNAUTHORIZED) |
| ADMIN override | 3 | Tất cả PASS |
| Thao tác Query | 3 | Tất cả PASS |

### 5.3 Lệnh Test Thủ công

#### Test 1: Tạo Document (Bất kỳ User)

```bash
# AdminOrg tạo document
source setEnv.sh adminorg
peer chaincode invoke -o localhost:7050 \
  --ordererTLSHostnameOverride orderer.docube.com \
  --tls --cafile $ORDERER_CA \
  -C docubechannel -n document_nft_cc \
  --peerAddresses localhost:7051 \
  --tlsRootCertFiles $CORE_PEER_TLS_ROOTCERT_FILE \
  -c '{"function":"document:CreateDocument","Args":["test-1","hash123","SHA256","sys1"]}'
```

#### Test 2: Non-Owner Update (Phải Thất bại)

```bash
# UserOrg cố update document của AdminOrg
source setEnv.sh userorg
peer chaincode invoke -o localhost:7050 \
  --ordererTLSHostnameOverride orderer.docube.com \
  --tls --cafile $ORDERER_CA \
  -C docubechannel -n document_nft_cc \
  --peerAddresses localhost:7051 \
  --tlsRootCertFiles /home/horob1/fabric-samples/docube-network/organizations/peerOrganizations/adminorg.docube.com/tlsca/tlsca.adminorg.docube.com-cert.pem \
  -c '{"function":"document:UpdateDocument","Args":["test-1","hackhash","SHA256","1"]}'

# Mong đợi: ERR_UNAUTHORIZED
```

#### Test 3: Admin Override (Phải Thành công)

```bash
# AdminOrg update document của UserOrg (admin override)
source setEnv.sh adminorg
peer chaincode invoke -o localhost:7050 \
  --ordererTLSHostnameOverride orderer.docube.com \
  --tls --cafile $ORDERER_CA \
  -C docubechannel -n document_nft_cc \
  --peerAddresses localhost:7051 \
  --tlsRootCertFiles $CORE_PEER_TLS_ROOTCERT_FILE \
  -c '{"function":"document:UpdateDocument","Args":["user-doc-1","admin-override","SHA256","1"]}'

# Mong đợi: Thành công với AdminAction event
```

---

## 6. Xác minh CouchDB State

### 6.1 Truy cập CouchDB UI

```bash
# AdminOrg CouchDB
firefox http://localhost:5984/_utils
# Đăng nhập: admin / adminpw

# UserOrg CouchDB
firefox http://localhost:7984/_utils
# Đăng nhập: admin / adminpw
```

### 6.2 Kiểm tra World State

1. Điều hướng đến database: `docubechannel_document_nft_cc`
2. Xác minh documents được lưu dạng JSON
3. Kiểm tra index tồn tại cho trường status

---

## 7. Xử lý Sự cố

### 7.1 Vấn đề Thường gặp

| Vấn đề | Giải pháp |
|--------|-----------|
| Container không khởi động | Kiểm tra ports: `netstat -tlnp \| grep 7050` |
| Chaincode timeout | Tăng timeout: `CORE_CHAINCODE_EXECUTETIMEOUT=300s` |
| CouchDB kết nối thất bại | Xác minh CouchDB container đang chạy |
| Lỗi TLS | Kiểm tra đường dẫn chứng chỉ trong biến môi trường |

### 7.2 Logs

```bash
# Xem peer logs
docker logs peer0.adminorg.docube.com -f

# Xem orderer logs
docker logs orderer.docube.com -f

# Xem chaincode logs
docker logs $(docker ps -q --filter "name=document_nft") -f
```

---

## 8. Mẫu Báo cáo Test

```markdown
# Báo cáo Test - Docube Network

**Ngày:** YYYY-MM-DD
**Người test:** [Tên]
**Phiên bản Network:** [Phiên bản]
**Phiên bản Chaincode:** v5.0

## Kết quả Test

| Test Case | Mong đợi | Thực tế | Pass/Fail |
|-----------|----------|---------|-----------|
| Khởi động mạng | Containers chạy | | |
| Tạo channel | docubechannel tồn tại | | |
| Deploy chaincode | v5.0 committed | | |
| Quyền: USER tạo | Thành công | | |
| Quyền: OWNER sửa | Thành công | | |
| Quyền: NON-OWNER bị từ chối | ERR_UNAUTHORIZED | | |
| Quyền: ADMIN override | Thành công | | |

## Vấn đề Phát hiện
- [Không có / Mô tả vấn đề]

## Kết luận
- [Pass / Fail với ghi chú]
```

---

## Lịch sử Tài liệu

| Phiên bản | Ngày | Tác giả | Thay đổi |
|-----------|------|---------|----------|
| 1.0 | 2026-02-01 | Đội Docube | Tài liệu ban đầu |
