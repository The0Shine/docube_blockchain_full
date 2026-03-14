# REBUILD AND TEST GUIDE - Docube Fabric Network

**Document Version:** 1.0  
**Last Updated:** 2026-02-01

---

## Purpose
This document provides step-by-step instructions for rebuilding the network from scratch and running validation tests.

## Scope
- Docker cleanup
- Network rebuild
- Channel recreation
- Chaincode deployment
- Test validation

## Audience
- DevOps Engineers
- QA Engineers
- Developers

## References
- [NETWORK_ARCHITECTURE_EN.md](NETWORK_ARCHITECTURE_EN.md)
- [PERMISSION_MATRIX_EN.md](PERMISSION_MATRIX_EN.md)

---

## 1. Prerequisites

### 1.1 System Requirements

| Requirement | Minimum |
|-------------|---------|
| Docker | 20.10+ |
| Docker Compose | 2.0+ |
| Go | 1.21+ |
| Node.js (optional) | 18+ |
| RAM | 4GB |
| Disk | 10GB free |

### 1.2 Fabric Binaries

```bash
# Verify Fabric binaries are installed
ls ~/fabric-samples/bin/
# Expected: peer, orderer, configtxgen, cryptogen, etc.
```

---

## 2. Complete Network Cleanup

### 2.1 Stop All Containers

```bash
cd ~/fabric-samples/docube-network

# Stop network and remove containers
./network.sh down
```

### 2.2 Docker Cleanup (Deep Clean)

```bash
# Remove all fabric-related containers
docker rm -f $(docker ps -aq --filter "label=service=hyperledger-fabric") 2>/dev/null

# Remove all chaincode containers
docker rm -f $(docker ps -aq --filter "name=dev-peer") 2>/dev/null

# Remove chaincode images
docker rmi -f $(docker images -q "dev-peer*") 2>/dev/null

# Remove volumes
docker volume prune -f

# Clean CouchDB volumes specifically
docker volume rm $(docker volume ls -q | grep docube) 2>/dev/null
```

### 2.3 Remove Generated Artifacts

```bash
cd ~/fabric-samples/docube-network

# Remove crypto materials (will be regenerated)
rm -rf organizations/peerOrganizations
rm -rf organizations/ordererOrganizations

# Remove channel artifacts
rm -rf channel-artifacts/*

# Remove chaincode package
rm -f *.tar.gz
```

---

## 3. Network Rebuild

### 3.1 Generate Crypto Materials

```bash
cd ~/fabric-samples/docube-network

# Using cryptogen (development mode)
cryptogen generate --config=./organizations/cryptogen/crypto-config-orderer.yaml --output="organizations"
cryptogen generate --config=./organizations/cryptogen/crypto-config-adminorg.yaml --output="organizations"
cryptogen generate --config=./organizations/cryptogen/crypto-config-userorg.yaml --output="organizations"
```

### 3.2 Start Network Containers

```bash
# Start with CouchDB
./network.sh up

# Verify containers are running
docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
```

Expected output:
```
NAMES                        STATUS     PORTS
orderer.docube.com          Up          7050, 7053
peer0.adminorg.docube.com   Up          7051
peer0.userorg.docube.com    Up          9051
couchdb0.adminorg           Up          5984
couchdb0.userorg            Up          7984
```

### 3.3 Create Channel

```bash
# Create channel
./network.sh createChannel

# Verify channel creation
source setEnv.sh adminorg
peer channel list
# Expected: docubechannel
```

---

## 4. Chaincode Deployment

### 4.1 Deploy Chaincode

```bash
cd ~/fabric-samples/docube-network

# Deploy chaincode v5.0
./scripts/deployCC.sh docubechannel document_nft_cc ./chaincode/docube golang 5.0 1
```

### 4.2 Verify Deployment

```bash
# Check committed chaincode
source setEnv.sh adminorg
peer lifecycle chaincode querycommitted -C docubechannel

# Expected output:
# Name: document_nft_cc, Version: 5.0, Sequence: 1
```

---

## 5. Validation Tests

### 5.1 Run Automated Permission Tests

```bash
cd ~/fabric-samples/docube-network
chmod +x ./tests/permission_test.sh
./tests/permission_test.sh
```

### 5.2 Expected Test Results

| Section | Tests | Expected |
|---------|-------|----------|
| USER can create | 2 | All PASS |
| OWNER can modify own | 4 | All PASS |
| NON-OWNER rejected | 4 | All PASS (ERR_UNAUTHORIZED) |
| ADMIN override | 3 | All PASS |
| Query operations | 3 | All PASS |

### 5.3 Manual Test Commands

#### Test 1: Create Document (Any User)

```bash
# AdminOrg creates document
source setEnv.sh adminorg
peer chaincode invoke -o localhost:7050 \
  --ordererTLSHostnameOverride orderer.docube.com \
  --tls --cafile $ORDERER_CA \
  -C docubechannel -n document_nft_cc \
  --peerAddresses localhost:7051 \
  --tlsRootCertFiles $CORE_PEER_TLS_ROOTCERT_FILE \
  -c '{"function":"document:CreateDocument","Args":["test-1","hash123","SHA256","sys1"]}'
```

#### Test 2: Non-Owner Update (Should Fail)

```bash
# UserOrg tries to update AdminOrg's document
source setEnv.sh userorg
peer chaincode invoke -o localhost:7050 \
  --ordererTLSHostnameOverride orderer.docube.com \
  --tls --cafile $ORDERER_CA \
  -C docubechannel -n document_nft_cc \
  --peerAddresses localhost:7051 \
  --tlsRootCertFiles /home/horob1/fabric-samples/docube-network/organizations/peerOrganizations/adminorg.docube.com/tlsca/tlsca.adminorg.docube.com-cert.pem \
  -c '{"function":"document:UpdateDocument","Args":["test-1","hackhash","SHA256","1"]}'

# Expected: ERR_UNAUTHORIZED
```

#### Test 3: Admin Override (Should Pass)

```bash
# AdminOrg updates UserOrg's document (admin override)
source setEnv.sh adminorg
peer chaincode invoke -o localhost:7050 \
  --ordererTLSHostnameOverride orderer.docube.com \
  --tls --cafile $ORDERER_CA \
  -C docubechannel -n document_nft_cc \
  --peerAddresses localhost:7051 \
  --tlsRootCertFiles $CORE_PEER_TLS_ROOTCERT_FILE \
  -c '{"function":"document:UpdateDocument","Args":["user-doc-1","admin-override","SHA256","1"]}'

# Expected: Success with AdminAction event
```

---

## 6. Verify CouchDB State

### 6.1 Access CouchDB UI

```bash
# AdminOrg CouchDB
firefox http://localhost:5984/_utils
# Login: admin / adminpw

# UserOrg CouchDB
firefox http://localhost:7984/_utils
# Login: admin / adminpw
```

### 6.2 Check World State

1. Navigate to database: `docubechannel_document_nft_cc`
2. Verify documents are stored as JSON
3. Check index exists for status field

---

## 7. Troubleshooting

### 7.1 Common Issues

| Issue | Solution |
|-------|----------|
| Container won't start | Check ports: `netstat -tlnp \| grep 7050` |
| Chaincode timeout | Increase timeout: `CORE_CHAINCODE_EXECUTETIMEOUT=300s` |
| CouchDB connection failed | Verify CouchDB container is running |
| TLS errors | Check certificate paths in env variables |

### 7.2 Logs

```bash
# View peer logs
docker logs peer0.adminorg.docube.com -f

# View orderer logs
docker logs orderer.docube.com -f

# View chaincode logs
docker logs $(docker ps -q --filter "name=document_nft") -f
```

---

## 8. Test Report Template

```markdown
# Test Report - Docube Network

**Date:** YYYY-MM-DD
**Tester:** [Name]
**Network Version:** [Version]
**Chaincode Version:** v5.0

## Test Results

| Test Case | Expected | Actual | Pass/Fail |
|-----------|----------|--------|-----------|
| Network startup | Containers running | | |
| Channel creation | docubechannel exists | | |
| Chaincode deployment | v5.0 committed | | |
| Permission: USER create | Success | | |
| Permission: OWNER modify | Success | | |
| Permission: NON-OWNER reject | ERR_UNAUTHORIZED | | |
| Permission: ADMIN override | Success | | |

## Issues Found
- [None / Issue description]

## Conclusion
- [Pass / Fail with notes]
```

---

## Document History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-02-01 | Docube Team | Initial document |
