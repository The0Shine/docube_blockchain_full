package client

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/hyperledger/fabric-gateway/pkg/client"
	"github.com/hyperledger/fabric-gateway/pkg/identity"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// FabricClient wraps the Fabric Gateway SDK for chaincode interactions.
type FabricClient struct {
	gateway  *client.Gateway
	contract *client.Contract
	conn     *grpc.ClientConn
}

// DocumentAsset mirrors the chaincode's DocumentAssetNFT model.
type DocumentAsset struct {
	AssetID      string `json:"assetId"`
	DocumentID   string `json:"documentId"`
	DocHash      string `json:"docHash"`
	HashAlgo     string `json:"hashAlgo"`
	OwnerID      string `json:"ownerId"`
	OwnerMSP     string `json:"ownerMsp"`
	SystemUserId string `json:"systemUserId"`
	Version      int64  `json:"version"`
	Status       string `json:"status"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

// AccessNFT mirrors the chaincode's AccessNFT model.
type AccessNFT struct {
	AccessNFTID  string `json:"accessNftId"`
	DocumentID   string `json:"documentId"`
	OwnerID      string `json:"ownerId"`
	OwnerMSP     string `json:"ownerMsp"`
	SystemUserId string `json:"systemUserId"`
	Status       string `json:"status"`
	GrantedAt    string `json:"grantedAt"`
	RevokedAt    string `json:"revokedAt,omitempty"`
	RevokedBy    string `json:"revokedBy,omitempty"`
}

// AccessCheckResult mirrors the chaincode's AccessCheckResult model.
// CallerID holds the application-layer system UUID of the user being checked.
type AccessCheckResult struct {
	Allowed    bool   `json:"allowed"`
	Reason     string `json:"reason"`
	DocumentID string `json:"documentId"`
	CallerID   string `json:"callerId"`
	Action     string `json:"action"`
}

// AuditRecord mirrors the chaincode's AuditRecord model.
type AuditRecord struct {
	TxID      string      `json:"txId"`
	Timestamp string      `json:"timestamp"`
	Value     interface{} `json:"value"`
	IsDelete  bool        `json:"isDelete"`
}

// Config holds all values needed to connect to the Fabric network.
type Config struct {
	ChannelName   string
	ChaincodeName string
	MspID         string
	PeerEndpoint  string
	GatewayPeer   string
	CryptoPath    string
	CertPath      string
	KeyDir        string
	TLSCertPath   string
}

// New creates a new FabricClient connected to the Fabric peer.
func New(cfg Config) (*FabricClient, error) {
	certPath := filepath.Join(cfg.CryptoPath, cfg.CertPath)
	keyDir := filepath.Join(cfg.CryptoPath, cfg.KeyDir)
	tlsCertPath := filepath.Join(cfg.CryptoPath, cfg.TLSCertPath)

	log.Printf("[FABRIC] Connecting to peer: %s", cfg.PeerEndpoint)
	log.Printf("[FABRIC] MSP: %s, Channel: %s, Chaincode: %s", cfg.MspID, cfg.ChannelName, cfg.ChaincodeName)

	tlsCert, err := os.ReadFile(tlsCertPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read TLS cert: %w", err)
	}
	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(tlsCert) {
		return nil, fmt.Errorf("failed to add TLS cert to pool")
	}
	conn, err := grpc.NewClient(cfg.PeerEndpoint, grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(certPool, cfg.GatewayPeer)))
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection: %w", err)
	}

	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read user cert: %w", err)
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM certificate")
	}
	parsedCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}
	id, err := identity.NewX509Identity(cfg.MspID, parsedCert)
	if err != nil {
		return nil, fmt.Errorf("failed to create X509 identity: %w", err)
	}

	keyFiles, err := os.ReadDir(keyDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read key directory: %w", err)
	}
	if len(keyFiles) == 0 {
		return nil, fmt.Errorf("no key files found in %s", keyDir)
	}
	keyPEM, err := os.ReadFile(filepath.Join(keyDir, keyFiles[0].Name()))
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %w", err)
	}
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, fmt.Errorf("failed to decode PEM private key")
	}
	privateKey, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		privateKey, err = x509.ParseECPrivateKey(keyBlock.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}
	}
	sign, err := identity.NewPrivateKeySign(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create signer: %w", err)
	}

	gw, err := client.Connect(
		id,
		client.WithSign(sign),
		client.WithClientConnection(conn),
		client.WithEvaluateTimeout(5*time.Second),
		client.WithEndorseTimeout(15*time.Second),
		client.WithSubmitTimeout(5*time.Second),
		client.WithCommitStatusTimeout(1*time.Minute),
	)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to connect to gateway: %w", err)
	}

	network := gw.GetNetwork(cfg.ChannelName)
	log.Println("[FABRIC] ✅ Successfully connected to Fabric network")
	return &FabricClient{
		gateway:  gw,
		contract: network.GetContract(cfg.ChaincodeName),
		conn:     conn,
	}, nil
}

// Close releases the gRPC connection and gateway resources.
func (fc *FabricClient) Close() {
	fc.gateway.Close()
	fc.conn.Close()
	log.Println("[FABRIC] Connection closed")
}

// --- Read operations (Evaluate — no ledger write) ---

func (fc *FabricClient) GetDocument(documentID string) (*DocumentAsset, error) {
	result, err := fc.contract.EvaluateTransaction("document:GetDocument", documentID)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate GetDocument: %w", err)
	}
	var doc DocumentAsset
	if err := json.Unmarshal(result, &doc); err != nil {
		return nil, fmt.Errorf("failed to unmarshal document: %w", err)
	}
	return &doc, nil
}

func (fc *FabricClient) GetAccess(documentID, userID string) (*AccessNFT, error) {
	result, err := fc.contract.EvaluateTransaction("access:GetAccess", documentID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate GetAccess: %w", err)
	}
	var access AccessNFT
	if err := json.Unmarshal(result, &access); err != nil {
		return nil, fmt.Errorf("failed to unmarshal access NFT: %w", err)
	}
	return &access, nil
}

// CheckAccessPermission evaluates whether systemUserId has access to the document.
// Uses the chaincode's CheckAccessPermission function which keys AccessNFTs by system UUID.
// action is a string label for audit purposes (e.g., "read", "download").
func (fc *FabricClient) CheckAccessPermission(documentID, systemUserId, action string) (*AccessCheckResult, error) {
	result, err := fc.contract.EvaluateTransaction("access:CheckAccessPermission", documentID, systemUserId, action)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate CheckAccessPermission: %w", err)
	}
	var checkResult AccessCheckResult
	if err := json.Unmarshal(result, &checkResult); err != nil {
		return nil, fmt.Errorf("failed to unmarshal access check result: %w", err)
	}
	return &checkResult, nil
}

func (fc *FabricClient) GetDocumentHistory(documentID string) ([]*AuditRecord, error) {
	result, err := fc.contract.EvaluateTransaction("document:GetDocumentHistory", documentID)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate GetDocumentHistory: %w", err)
	}
	var history []*AuditRecord
	if err := json.Unmarshal(result, &history); err != nil {
		return nil, fmt.Errorf("failed to unmarshal history: %w", err)
	}
	return history, nil
}

// --- Write operations (Submit — commits to ledger) ---

func (fc *FabricClient) CreateDocument(documentID, docHash, hashAlgo, systemUserId string) error {
	log.Printf("[FABRIC] SubmitTx CreateDocument: doc=%s user=%s", documentID, systemUserId)
	_, err := fc.contract.SubmitTransaction("document:CreateDocument", documentID, docHash, hashAlgo, systemUserId)
	if err != nil {
		return fmt.Errorf("CreateDocument chaincode error: %w", err)
	}
	log.Printf("[FABRIC] ✅ CreateDocument success: doc=%s", documentID)
	return nil
}

func (fc *FabricClient) UpdateDocument(documentID, newDocHash, newHashAlgo string, expectedVersion int64) error {
	log.Printf("[FABRIC] SubmitTx UpdateDocument: doc=%s version=%d", documentID, expectedVersion)
	_, err := fc.contract.SubmitTransaction("document:UpdateDocument", documentID, newDocHash, newHashAlgo, fmt.Sprintf("%d", expectedVersion))
	if err != nil {
		return fmt.Errorf("UpdateDocument chaincode error: %w", err)
	}
	log.Printf("[FABRIC] ✅ UpdateDocument success: doc=%s", documentID)
	return nil
}

func (fc *FabricClient) GrantAccess(documentID, granteeUserID, granteeUserMSP, systemUserId string) error {
	log.Printf("[FABRIC] SubmitTx GrantAccess: doc=%s grantee=%s", documentID, granteeUserID)
	_, err := fc.contract.SubmitTransaction("access:GrantAccess", documentID, granteeUserID, granteeUserMSP, systemUserId)
	if err != nil {
		return fmt.Errorf("GrantAccess chaincode error: %w", err)
	}
	log.Printf("[FABRIC] ✅ GrantAccess success: doc=%s grantee=%s", documentID, granteeUserID)
	return nil
}

func (fc *FabricClient) RevokeAccess(documentID, userID string) error {
	log.Printf("[FABRIC] SubmitTx RevokeAccess: doc=%s user=%s", documentID, userID)
	_, err := fc.contract.SubmitTransaction("access:RevokeAccess", documentID, userID)
	if err != nil {
		return fmt.Errorf("RevokeAccess chaincode error: %w", err)
	}
	log.Printf("[FABRIC] ✅ RevokeAccess success: doc=%s user=%s", documentID, userID)
	return nil
}
