# Hướng dẫn Test Permission Model - Chaincode v5.0

## 🔑 Permission Model: USER / OWNER / ADMIN

| Role | Quyền |
|------|-------|
| **USER** | Tạo document mới, query tất cả |
| **OWNER** | Update/Delete/Transfer document của mình, Grant/Revoke access |
| **ADMIN** (AdminOrgMSP) | Override mọi quyền Owner, có full access toàn hệ thống |

---

## 1. Thiết lập Môi trường

```bash
cd ~/fabric-samples/docube-network

# AdminOrg (ADMIN - có quyền override)
source setEnv.sh adminorg

# UserOrg (USER/OWNER thông thường)
source setEnv.sh userorg
```

---

## 2. Test Cases theo Authorization Matrix

### 2.1 USER có thể tạo Document (Tất cả user đều có quyền)

```bash
# AdminOrg tạo document
source setEnv.sh adminorg
peer chaincode invoke -o localhost:7050 \
  --ordererTLSHostnameOverride orderer.docube.com \
  --tls --cafile $ORDERER_CA \
  -C docubechannel -n document_nft_cc \
  --peerAddresses localhost:7051 --tlsRootCertFiles $CORE_PEER_TLS_ROOTCERT_FILE \
  -c '{"function":"document:CreateDocument","Args":["test-doc-1","hash1","SHA256","system_user_1"]}'

# UserOrg cũng có thể tạo document
source setEnv.sh userorg
peer chaincode invoke -o localhost:7050 \
  --ordererTLSHostnameOverride orderer.docube.com \
  --tls --cafile $ORDERER_CA \
  -C docubechannel -n document_nft_cc \
  --peerAddresses localhost:7051 --tlsRootCertFiles /home/horob1/fabric-samples/docube-network/organizations/peerOrganizations/adminorg.docube.com/tlsca/tlsca.adminorg.docube.com-cert.pem \
  -c '{"function":"document:CreateDocument","Args":["test-doc-2","hash2","SHA256","system_user_2"]}'
```
**Kết quả mong đợi**: Cả hai đều thành công ✅

---

### 2.2 OWNER có thể sửa document của mình

```bash
# AdminOrg (owner của test-doc-1) update document của mình
source setEnv.sh adminorg
peer chaincode invoke -o localhost:7050 \
  --ordererTLSHostnameOverride orderer.docube.com \
  --tls --cafile $ORDERER_CA \
  -C docubechannel -n document_nft_cc \
  --peerAddresses localhost:7051 --tlsRootCertFiles $CORE_PEER_TLS_ROOTCERT_FILE \
  -c '{"function":"document:UpdateDocument","Args":["test-doc-1","newhash","SHA256","1"]}'
```
**Kết quả mong đợi**: Thành công ✅

---

### 2.3 NON-OWNER bị từ chối (ERR_UNAUTHORIZED)

```bash
# UserOrg cố gắng update document của AdminOrg → BỊ TỪ CHỐI
source setEnv.sh userorg
peer chaincode invoke -o localhost:7050 \
  --ordererTLSHostnameOverride orderer.docube.com \
  --tls --cafile $ORDERER_CA \
  -C docubechannel -n document_nft_cc \
  --peerAddresses localhost:7051 --tlsRootCertFiles /home/horob1/fabric-samples/docube-network/organizations/peerOrganizations/adminorg.docube.com/tlsca/tlsca.adminorg.docube.com-cert.pem \
  -c '{"function":"document:UpdateDocument","Args":["test-doc-1","hackhash","SHA256","2"]}'

# UserOrg cố gắng xóa document của AdminOrg → BỊ TỪ CHỐI
peer chaincode invoke -o localhost:7050 \
  --ordererTLSHostnameOverride orderer.docube.com \
  --tls --cafile $ORDERER_CA \
  -C docubechannel -n document_nft_cc \
  --peerAddresses localhost:7051 --tlsRootCertFiles /home/horob1/fabric-samples/docube-network/organizations/peerOrganizations/adminorg.docube.com/tlsca/tlsca.adminorg.docube.com-cert.pem \
  -c '{"function":"document:SoftDeleteDocument","Args":["test-doc-1"]}'

# UserOrg cố gắng grant access trên document của AdminOrg → BỊ TỪ CHỐI
peer chaincode invoke -o localhost:7050 \
  --ordererTLSHostnameOverride orderer.docube.com \
  --tls --cafile $ORDERER_CA \
  -C docubechannel -n document_nft_cc \
  --peerAddresses localhost:7051 --tlsRootCertFiles /home/horob1/fabric-samples/docube-network/organizations/peerOrganizations/adminorg.docube.com/tlsca/tlsca.adminorg.docube.com-cert.pem \
  -c '{"function":"access:GrantAccess","Args":["test-doc-1","Hacker","UserOrgMSP","sysX"]}'
```
**Kết quả mong đợi**: Tất cả bị từ chối với `ERR_UNAUTHORIZED` ✅

---

### 2.4 ADMIN có thể Override bất kỳ Document nào

```bash
# AdminOrg (ADMIN) update document của UserOrg → THÀNH CÔNG
source setEnv.sh adminorg
peer chaincode invoke -o localhost:7050 \
  --ordererTLSHostnameOverride orderer.docube.com \
  --tls --cafile $ORDERER_CA \
  -C docubechannel -n document_nft_cc \
  --peerAddresses localhost:7051 --tlsRootCertFiles $CORE_PEER_TLS_ROOTCERT_FILE \
  -c '{"function":"document:UpdateDocument","Args":["test-doc-2","admin-override","SHA256","1"]}'

# AdminOrg grant access trên document của UserOrg → THÀNH CÔNG
peer chaincode invoke -o localhost:7050 \
  --ordererTLSHostnameOverride orderer.docube.com \
  --tls --cafile $ORDERER_CA \
  -C docubechannel -n document_nft_cc \
  --peerAddresses localhost:7051 --tlsRootCertFiles $CORE_PEER_TLS_ROOTCERT_FILE \
  -c '{"function":"access:GrantAccess","Args":["test-doc-2","AdminGrantedUser","AdminOrgMSP","sys3"]}'
```
**Kết quả mong đợi**: Thành công với AdminAction event ✅

---

### 2.5 QUERY không giới hạn Role

```bash
# UserOrg query document của AdminOrg → THÀNH CÔNG
source setEnv.sh userorg
peer chaincode query -C docubechannel -n document_nft_cc \
  -c '{"function":"document:GetDocument","Args":["test-doc-1"]}'

# Query tất cả documents
peer chaincode query -C docubechannel -n document_nft_cc \
  -c '{"function":"document:GetAllDocuments","Args":[]}'
```
**Kết quả mong đợi**: Thành công, không giới hạn theo role ✅

---

## 3. Chạy Script Test Tự động

```bash
cd ~/fabric-samples/docube-network
./tests/permission_test.sh
```

Script sẽ test 16 scenarios và báo cáo kết quả chi tiết.

---

## 4. Authorization Matrix Summary

| Function | USER | OWNER | ADMIN |
|----------|:----:|:-----:|:-----:|
| CreateDocument | ✅ | ✅ | ✅ |
| UpdateDocument | ❌ | ✅ | ✅ |
| SoftDeleteDocument | ❌ | ✅ | ✅ |
| TransferOwnership | ❌ | ✅ | ✅ |
| GrantAccess | ❌ | ✅ | ✅ |
| RevokeAccess | ❌ | ✅ | ✅ |
| Query* | ✅ | ✅ | ✅ |

---

## 5. Xem CouchDB World State

```bash
# AdminOrg CouchDB
firefox http://localhost:5984/_utils
# Username: admin | Password: adminpw

# UserOrg CouchDB
firefox http://localhost:7984/_utils

# Database: docubechannel_document_nft_cc
```

---

## 6. Các Lỗi Thường Gặp

| Lỗi | Nguyên nhân | Cách sửa |
|-----|-------------|----------|
| `ERR_UNAUTHORIZED` | Không phải OWNER hoặc ADMIN | Chuyển sang đúng org/đúng owner |
| `ERR_NOT_FOUND` | Document/Access không tồn tại | Kiểm tra ID |
| `ERR_ALREADY_EXISTS` | Document/Access đã có | Dùng ID khác |
| `ERR_VERSION_MISMATCH` | Version không khớp | Query lại version mới nhất |
| `ERR_INVALID_STATE` | Document đã xóa/Access đã revoke | Không thể thao tác |
