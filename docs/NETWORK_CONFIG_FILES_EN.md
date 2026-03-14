# NETWORK CONFIGURATION FILES - Docube Fabric Network

**Document Version:** 1.0  
**Last Updated:** 2026-02-01

---

## Purpose
This document explains all network-related configuration files in the Docube Fabric Network.

## Scope
- configtx.yaml
- Docker Compose files
- CouchDB configuration
- Network scripts

## References
- [NETWORK_ARCHITECTURE_EN.md](NETWORK_ARCHITECTURE_EN.md)

---

## 1. configtx.yaml

**Location:** `configtx/configtx.yaml`  
**Purpose:** Defines channel configuration, organizations, and policies

### 1.1 Organizations Section

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

**Security Implications:**
- MSPDir must point to valid certificate directories
- MSP ID must be unique across the network
- Policies control read/write/admin access

### 1.2 Key Policies

| Policy | AdminOrg | UserOrg | Impact |
|--------|----------|---------|--------|
| Readers | All members | All members | Who can query |
| Writers | Admin + Client | Admin only | Who can submit tx |
| Admins | Admin only | Admin only | Channel admin |
| Endorsement | Peer | Peer | Who endorses |

### 1.3 Application Policies (Critical)

```yaml
Application:
  Policies:
    Endorsement:
      Type: Signature
      Rule: "OR('AdminOrgMSP.peer')"  # Only AdminOrg can endorse
    LifecycleEndorsement:
      Type: Signature
      Rule: "OR('AdminOrgMSP.peer')"  # Only AdminOrg deploys chaincode
```

**Security Note:** This ensures chaincode writes go through AdminOrg peer.

---

## 2. Docker Compose Files

### 2.1 compose-docube-net.yaml

**Location:** `compose/compose-docube-net.yaml`  
**Purpose:** Defines all Fabric containers

#### Services Defined:

| Service | Image | Ports | Purpose |
|---------|-------|-------|---------|
| orderer.docube.com | fabric-orderer:latest | 7050, 7053 | Transaction ordering |
| peer0.adminorg | fabric-peer:latest | 7051 | AdminOrg peer |
| peer0.userorg | fabric-peer:latest | 9051 | UserOrg peer |

#### Key Environment Variables:

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

**Location:** `compose/compose-couch.yaml`  
**Purpose:** Defines CouchDB containers for state database

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

#### Peer CouchDB Configuration:

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

**Location:** `network.sh`  
**Purpose:** Main script for network management

#### Commands:

| Command | Description |
|---------|-------------|
| `./network.sh up` | Start network containers |
| `./network.sh down` | Stop and clean network |
| `./network.sh createChannel` | Create docubechannel |

#### Key Variables:

```bash
DATABASE="couchdb"           # State database
CHANNEL_NAME="docubechannel" # Channel name
COMPOSE_FILE_COUCH="compose-couch.yaml"
```

### 3.2 setEnv.sh

**Location:** `setEnv.sh`  
**Purpose:** Set environment for peer CLI

```bash
# Usage
source setEnv.sh adminorg  # Set AdminOrg environment
source setEnv.sh userorg   # Set UserOrg environment

# Variables set:
CORE_PEER_ADDRESS=localhost:7051
CORE_PEER_LOCALMSPID=AdminOrgMSP
CORE_PEER_TLS_ROOTCERT_FILE=...
CORE_PEER_MSPCONFIGPATH=...
```

### 3.3 deployCC.sh

**Location:** `scripts/deployCC.sh`  
**Purpose:** Deploy chaincode to channel

```bash
# Usage
./scripts/deployCC.sh <channel> <cc_name> <cc_path> <lang> <version> <sequence>

# Example
./scripts/deployCC.sh docubechannel document_nft_cc ./chaincode/docube golang 5.0 2
```

#### Lifecycle Steps:
1. Package chaincode
2. Install on AdminOrg peer
3. Install on UserOrg peer
4. Approve for AdminOrg
5. Approve for UserOrg
6. Commit to channel

---

## 4. Certificate Structure

### 4.1 Organization Directories

```
organizations/
├── ordererOrganizations/
│   └── docube.com/
│       ├── ca/                    # Root CA certificate
│       ├── msp/                   # Organization MSP
│       │   ├── cacerts/           # CA certificates
│       │   ├── tlscacerts/        # TLS CA certificates
│       │   └── config.yaml        # MSP config
│       └── orderers/
│           └── orderer.docube.com/
│               ├── msp/           # Orderer MSP
│               └── tls/           # TLS certificates
└── peerOrganizations/
    ├── adminorg.docube.com/
    │   ├── ca/
    │   ├── msp/
    │   ├── peers/peer0.adminorg.docube.com/
    │   └── users/Admin@adminorg.docube.com/
    └── userorg.docube.com/
        └── ...
```

### 4.2 Security Best Practices

| Item | Recommendation |
|------|----------------|
| Private keys | Never commit to git |
| TLS | Always enabled |
| Credentials | Use secrets management |
| CA | Consider Fabric CA for production |

---

## 5. Configuration Summary

| File | Location | Purpose |
|------|----------|---------|
| configtx.yaml | configtx/ | Channel/Org definitions |
| compose-docube-net.yaml | compose/ | Container definitions |
| compose-couch.yaml | compose/ | CouchDB containers |
| network.sh | ./ | Network management |
| setEnv.sh | ./ | CLI environment |
| deployCC.sh | scripts/ | Chaincode deployment |

---

## Document History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-02-01 | Docube Team | Initial document |
