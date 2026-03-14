#!/usr/bin/env bash
# =============================================================================
# Docube Fabric Init Entrypoint
# =============================================================================
# Runs inside the init container to bootstrap the entire Fabric network:
#   1. Generate crypto material (cryptogen)
#   2. Create channel genesis block (configtxgen)
#   3. Join orderer + peers to channel
#   4. Deploy chaincode (package → install → approve → commit)
#
# Volume mounts (từ docker-compose):
#   fabric-crypto            → /fabric/organizations
#   fabric-channel-artifacts → /fabric/channel-artifacts
#
# Thư mục baked trong image (từ Dockerfile COPY network/ /fabric/):
#   /fabric/configtx/configtx.yaml
#   /fabric/organizations/cryptogen/
#   /fabric/chaincode/docube/
#   /fabric/config/   (core.yaml, orderer.yaml)
# =============================================================================

set -e

# --- Defaults ---
CHANNEL_NAME="${CHANNEL_NAME:-docubechannel}"
CC_NAME="${CC_NAME:-document_nft_cc}"
CC_SRC_PATH="${CC_SRC_PATH:-/fabric/chaincode/docube}"
CC_RUNTIME_LANGUAGE="${CC_RUNTIME_LANGUAGE:-golang}"
CC_VERSION="${CC_VERSION:-1.0}"
CC_SEQUENCE="${CC_SEQUENCE:-1}"
ORDERER_ENDPOINT="${ORDERER_ENDPOINT:-orderer.docube.com:7050}"
ORDERER_ADMIN_ENDPOINT="${ORDERER_ADMIN_ENDPOINT:-orderer.docube.com:7053}"
PEER_ADMINORG_ENDPOINT="${PEER_ADMINORG_ENDPOINT:-peer0.adminorg.docube.com:7051}"
PEER_USERORG_ENDPOINT="${PEER_USERORG_ENDPOINT:-peer0.userorg.docube.com:9051}"
MAX_RETRY="${MAX_RETRY:-10}"
CLI_DELAY="${CLI_DELAY:-3}"
SKIP_CHAINCODE="${SKIP_CHAINCODE:-false}"

# --- Paths trong container ---
CRYPTO_BASE=/fabric/organizations
CHANNEL_ARTIFACTS=/fabric/channel-artifacts
CONFIGTX_PATH=/fabric/configtx
CONFIG_PATH=/fabric/config

export CORE_PEER_TLS_ENABLED=true

# --- Crypto paths ---
ORDERER_CA="${CRYPTO_BASE}/ordererOrganizations/docube.com/tlsca/tlsca.docube.com-cert.pem"
ORDERER_ADMIN_TLS_SIGN_CERT="${CRYPTO_BASE}/ordererOrganizations/docube.com/orderers/orderer.docube.com/tls/server.crt"
ORDERER_ADMIN_TLS_PRIVATE_KEY="${CRYPTO_BASE}/ordererOrganizations/docube.com/orderers/orderer.docube.com/tls/server.key"
PEER0_ADMINORG_CA="${CRYPTO_BASE}/peerOrganizations/adminorg.docube.com/tlsca/tlsca.adminorg.docube.com-cert.pem"
PEER0_USERORG_CA="${CRYPTO_BASE}/peerOrganizations/userorg.docube.com/tlsca/tlsca.userorg.docube.com-cert.pem"

# --- Logging helpers ---
info()    { echo -e "\033[1;34m[INIT]\033[0m $1"; }
success() { echo -e "\033[1;32m[INIT] OK\033[0m $1"; }
error()   { echo -e "\033[1;31m[INIT] ERR\033[0m $1"; }
warn()    { echo -e "\033[1;33m[INIT] WARN\033[0m $1"; }

# --- Helper: set peer env vars ---
setGlobals() {
  local ORG=$1
  export FABRIC_CFG_PATH=$CONFIG_PATH
  if [ "$ORG" == "adminorg" ]; then
    export CORE_PEER_LOCALMSPID=AdminOrgMSP
    export CORE_PEER_TLS_ROOTCERT_FILE=$PEER0_ADMINORG_CA
    export CORE_PEER_MSPCONFIGPATH="${CRYPTO_BASE}/peerOrganizations/adminorg.docube.com/users/Admin@adminorg.docube.com/msp"
    export CORE_PEER_ADDRESS=$PEER_ADMINORG_ENDPOINT
  elif [ "$ORG" == "userorg" ]; then
    export CORE_PEER_LOCALMSPID=UserOrgMSP
    export CORE_PEER_TLS_ROOTCERT_FILE=$PEER0_USERORG_CA
    export CORE_PEER_MSPCONFIGPATH="${CRYPTO_BASE}/peerOrganizations/userorg.docube.com/users/Admin@userorg.docube.com/msp"
    export CORE_PEER_ADDRESS=$PEER_USERORG_ENDPOINT
  fi
}

# --- Helper: wait for TCP port ---
waitFor() {
  local NAME=$1
  local HOST=$2
  local PORT=$3
  local COUNTER=0
  info "Waiting for ${NAME} at ${HOST}:${PORT}..."
  while ! nc -z "$HOST" "$PORT" 2>/dev/null; do
    COUNTER=$((COUNTER + 1))
    if [ $COUNTER -ge 60 ]; then
      error "${NAME} not ready after 60 attempts. Exiting."
      exit 1
    fi
    echo "[INIT] ${NAME} not ready yet (attempt ${COUNTER}/60)..."
    sleep $CLI_DELAY
  done
  success "${NAME} is ready"
}

# --- Helper: retry a command ---
retryCmd() {
  local DESC=$1
  shift
  local rc=1
  local COUNTER=0
  while [ $rc -ne 0 ] && [ $COUNTER -lt $MAX_RETRY ]; do
    sleep $CLI_DELAY
    "$@" && rc=0 || rc=$?
    COUNTER=$((COUNTER + 1))
  done
  if [ $rc -ne 0 ]; then
    error "${DESC} failed after ${MAX_RETRY} attempts"
    exit 1
  fi
}

# =============================================================================
# STEP 1: Generate crypto material
# FIX: path tuyệt đối /fabric/organizations/cryptogen/ thay vì ./organizations/
# =============================================================================
generateCrypto() {
  local CERT="${CRYPTO_BASE}/peerOrganizations/adminorg.docube.com/peers/peer0.adminorg.docube.com/msp/signcerts/peer0.adminorg.docube.com-cert.pem"
  if [ -f "$CERT" ]; then
    warn "Crypto material already exists. Skipping generation."
    return 0
  fi

  info "Generating crypto material..."

  # Cryptogen configs are baked at /fabric/cryptogen-configs/ (safe from volume overlay)
  # Fallback to volume path for backward compatibility
  CRYPTOGEN_CFG="/fabric/cryptogen-configs"
  if [ ! -d "$CRYPTOGEN_CFG" ]; then
    CRYPTOGEN_CFG="${CRYPTO_BASE}/cryptogen"
  fi

  cryptogen generate \
    --config="${CRYPTOGEN_CFG}/crypto-config-adminorg.yaml" \
    --output="${CRYPTO_BASE}"

  cryptogen generate \
    --config="${CRYPTOGEN_CFG}/crypto-config-userorg.yaml" \
    --output="${CRYPTO_BASE}"

  cryptogen generate \
    --config="${CRYPTOGEN_CFG}/crypto-config-orderer.yaml" \
    --output="${CRYPTO_BASE}"

  success "Crypto material generated"

  # Remove admincerts from peer local MSPs — not needed with NodeOUs enabled.
  # Newer cryptogen versions place certs here without proper OU extensions,
  # which causes peer MSP validation to fail. NodeOUs identifies admins by
  # OU=admin in the cert subject, not by admincerts/ presence.
  info "Clearing peer local MSP admincerts (NodeOUs mode)..."
  find "${CRYPTO_BASE}/peerOrganizations" -path "*/peers/*/msp/admincerts/*" -delete 2>/dev/null || true
  success "admincerts cleared"

  # Signal that crypto is fully ready (admincerts cleared) — peers wait for this marker
  touch "${CRYPTO_BASE}/.crypto-ready"
}

# =============================================================================
# STEP 2: Create channel genesis block
# FIX: FABRIC_CFG_PATH=/fabric/configtx, output vào /fabric/channel-artifacts/
# =============================================================================
createChannelGenesisBlock() {
  if [ -f "${CHANNEL_ARTIFACTS}/${CHANNEL_NAME}.block" ]; then
    warn "Channel genesis block already exists. Skipping."
    return 0
  fi

  info "Creating channel genesis block..."
  mkdir -p "${CHANNEL_ARTIFACTS}"

  export FABRIC_CFG_PATH=$CONFIGTX_PATH

  configtxgen \
    -profile DocubeChannel \
    -outputBlock "${CHANNEL_ARTIFACTS}/${CHANNEL_NAME}.block" \
    -channelID "$CHANNEL_NAME"

  success "Channel genesis block created"
}

# =============================================================================
# STEP 3: Create channel + join peers
# FIX: dùng ORDERER_ADMIN_ENDPOINT (orderer.docube.com:7053) thay vì localhost:7053
# =============================================================================
joinPeerToChannel() {
  local ORG=$1
  local BLOCK="${CHANNEL_ARTIFACTS}/${CHANNEL_NAME}.block"

  # Idempotent: dùng peer channel getinfo để kiểm tra trước
  if peer channel getinfo -c "$CHANNEL_NAME" >/dev/null 2>&1; then
    warn "${ORG} peer already in channel '${CHANNEL_NAME}'. Skipping."
    return 0
  fi

  local rc=1
  local COUNTER=0
  while [ $rc -ne 0 ] && [ $COUNTER -lt $MAX_RETRY ]; do
    sleep $CLI_DELAY
    OUTPUT=$(peer channel join -b "$BLOCK" 2>&1)
    rc=$?
    if [ $rc -eq 0 ]; then
      return 0
    fi
    # Nếu lỗi "already exists" thì coi như thành công
    if echo "$OUTPUT" | grep -q "already exists"; then
      warn "${ORG} peer already in channel '${CHANNEL_NAME}'. Skipping."
      return 0
    fi
    info "[${ORG}] Join attempt ${COUNTER}/${MAX_RETRY}: ${OUTPUT}"
    COUNTER=$((COUNTER + 1))
  done
  error "${ORG} peer channel join failed after ${MAX_RETRY} attempts"
  exit 1
}

createAndJoinChannel() {
  # --- Join orderer to channel (idempotent) ---
  info "Joining orderer to channel '${CHANNEL_NAME}'..."
  if OSNADMIN_OUT=$(osnadmin channel join \
    --channelID "$CHANNEL_NAME" \
    --config-block "${CHANNEL_ARTIFACTS}/${CHANNEL_NAME}.block" \
    -o "$ORDERER_ADMIN_ENDPOINT" \
    --ca-file "$ORDERER_CA" \
    --client-cert "$ORDERER_ADMIN_TLS_SIGN_CERT" \
    --client-key "$ORDERER_ADMIN_TLS_PRIVATE_KEY" 2>&1); then
    success "Orderer joined channel '${CHANNEL_NAME}'"
  elif echo "$OSNADMIN_OUT" | grep -qE "already exists|Status: 405"; then
    warn "Orderer already in channel '${CHANNEL_NAME}'. Skipping."
  else
    error "osnadmin failed: ${OSNADMIN_OUT}"
    exit 1
  fi

  # --- Join AdminOrg peer (idempotent) ---
  info "Joining AdminOrg peer to channel..."
  setGlobals adminorg
  joinPeerToChannel "AdminOrg"
  success "AdminOrg peer: in channel"

  # --- Join UserOrg peer (idempotent) ---
  info "Joining UserOrg peer to channel..."
  setGlobals userorg
  joinPeerToChannel "UserOrg"
  success "UserOrg peer: in channel"
}

# =============================================================================
# STEP 4: Deploy chaincode
# FIX: dùng container hostname thay vì localhost cho tất cả peer/orderer endpoints
# =============================================================================
deployChaincode() {
  if [ "$SKIP_CHAINCODE" == "true" ]; then
    warn "SKIP_CHAINCODE=true. Skipping chaincode deployment."
    return 0
  fi

  export FABRIC_CFG_PATH=$CONFIG_PATH

  # Idempotency: skip nếu chaincode đã committed (bắt mọi format JSON: có/không có space)
  setGlobals adminorg
  COMMITTED=$(peer lifecycle chaincode querycommitted \
    --channelID "$CHANNEL_NAME" --name "$CC_NAME" 2>/dev/null || true)
  if echo "$COMMITTED" | grep -q "Sequence: ${CC_SEQUENCE},"; then
    warn "Chaincode '${CC_NAME}' v${CC_VERSION} sequence ${CC_SEQUENCE} already committed. Skipping deployment."
    success "Chaincode '${CC_NAME}' already deployed!"
    return 0
  fi

  # --- Package ---
  info "Packaging chaincode '${CC_NAME}'..."
  if [ ! -f "${CC_NAME}.tar.gz" ]; then
    peer lifecycle chaincode package "${CC_NAME}.tar.gz" \
      --path "$CC_SRC_PATH" \
      --lang "$CC_RUNTIME_LANGUAGE" \
      --label "${CC_NAME}_${CC_VERSION}"
    success "Chaincode packaged"
  else
    warn "Package already exists. Skipping."
  fi

  # --- Install on AdminOrg ---
  info "Installing chaincode on AdminOrg..."
  setGlobals adminorg
  peer lifecycle chaincode install "${CC_NAME}.tar.gz"
  success "Chaincode installed on AdminOrg"

  # --- Install on UserOrg ---
  info "Installing chaincode on UserOrg..."
  setGlobals userorg
  peer lifecycle chaincode install "${CC_NAME}.tar.gz"
  success "Chaincode installed on UserOrg"

  # --- Get package ID ---
  setGlobals adminorg
  PACKAGE_ID=$(peer lifecycle chaincode queryinstalled --output json | \
    jq -r 'try (.installed_chaincodes[].package_id)' | \
    grep "^${CC_NAME}_${CC_VERSION}")
  info "Package ID: ${PACKAGE_ID}"

  if [ -z "$PACKAGE_ID" ]; then
    error "Could not find package ID for ${CC_NAME}_${CC_VERSION}"
    exit 1
  fi

  # --- Approve for AdminOrg ---
  # FIX: -o orderer.docube.com:7050 thay vì localhost:7050
  info "Approving chaincode for AdminOrg..."
  setGlobals adminorg
  peer lifecycle chaincode approveformyorg \
    -o "$ORDERER_ENDPOINT" \
    --ordererTLSHostnameOverride orderer.docube.com \
    --tls --cafile "$ORDERER_CA" \
    --channelID "$CHANNEL_NAME" \
    --name "$CC_NAME" \
    --version "$CC_VERSION" \
    --package-id "$PACKAGE_ID" \
    --sequence "$CC_SEQUENCE" \
    --signature-policy "OR('AdminOrgMSP.peer','UserOrgMSP.peer')"
  success "Chaincode approved by AdminOrg"

  # --- Approve for UserOrg ---
  info "Approving chaincode for UserOrg..."
  setGlobals userorg
  peer lifecycle chaincode approveformyorg \
    -o "$ORDERER_ENDPOINT" \
    --ordererTLSHostnameOverride orderer.docube.com \
    --tls --cafile "$ORDERER_CA" \
    --channelID "$CHANNEL_NAME" \
    --name "$CC_NAME" \
    --version "$CC_VERSION" \
    --package-id "$PACKAGE_ID" \
    --sequence "$CC_SEQUENCE" \
    --signature-policy "OR('AdminOrgMSP.peer','UserOrgMSP.peer')"
  success "Chaincode approved by UserOrg"

  # --- Check commit readiness ---
  info "Checking commit readiness..."
  setGlobals adminorg
  peer lifecycle chaincode checkcommitreadiness \
    --channelID "$CHANNEL_NAME" \
    --name "$CC_NAME" \
    --version "$CC_VERSION" \
    --sequence "$CC_SEQUENCE" \
    --output json

  # --- Commit ---
  # FIX: --peerAddresses dùng container hostname thay vì localhost
  info "Committing chaincode definition..."
  setGlobals adminorg
  peer lifecycle chaincode commit \
    -o "$ORDERER_ENDPOINT" \
    --ordererTLSHostnameOverride orderer.docube.com \
    --tls --cafile "$ORDERER_CA" \
    --channelID "$CHANNEL_NAME" \
    --name "$CC_NAME" \
    --version "$CC_VERSION" \
    --sequence "$CC_SEQUENCE" \
    --peerAddresses "$PEER_ADMINORG_ENDPOINT" \
    --tlsRootCertFiles "$PEER0_ADMINORG_CA" \
    --signature-policy "OR('AdminOrgMSP.peer','UserOrgMSP.peer')"
  success "Chaincode committed"

  # --- Verify ---
  info "Verifying committed chaincode..."
  setGlobals adminorg
  peer lifecycle chaincode querycommitted \
    --channelID "$CHANNEL_NAME" \
    --name "$CC_NAME"
  success "Chaincode '${CC_NAME}' deployed successfully!"
}

# =============================================================================
# MAIN
# =============================================================================
main() {
  info "=========================================="
  info "  Docube Fabric Network Init"
  info "=========================================="
  info "Channel:       ${CHANNEL_NAME}"
  info "Chaincode:     ${CC_NAME} v${CC_VERSION}"
  info "CC Source:     ${CC_SRC_PATH}"
  info "Orderer:       ${ORDERER_ENDPOINT}"
  info "Peer AdminOrg: ${PEER_ADMINORG_ENDPOINT}"
  info "Peer UserOrg:  ${PEER_USERORG_ENDPOINT}"
  info "=========================================="

  # Step 1: Crypto
  generateCrypto

  # Chờ orderer và peers TCP sẵn sàng
  waitFor "Orderer"       "orderer.docube.com"        "7050"
  waitFor "AdminOrg Peer" "peer0.adminorg.docube.com" "7051"
  waitFor "UserOrg Peer"  "peer0.userorg.docube.com"  "9051"

  # Step 2: Genesis block
  createChannelGenesisBlock

  # Step 3: Channel
  createAndJoinChannel

  # Step 4: Chaincode
  deployChaincode

  info ""
  success "=========================================="
  success "  Fabric network is READY!"
  success "  Channel:   ${CHANNEL_NAME}"
  success "  Chaincode: ${CC_NAME} v${CC_VERSION}"
  success "=========================================="

  touch /fabric/.init-complete
  exit 0
}

main "$@"