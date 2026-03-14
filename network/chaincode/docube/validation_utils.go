// Package main provides validation utilities for the Docube chaincode.
package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

// =============================================================================
// KEY BUILDING UTILITIES
// =============================================================================

// BuildDocumentKey creates a composite key for document storage.
// Format: DOC~{documentId}
func BuildDocumentKey(ctx contractapi.TransactionContextInterface, documentID string) (string, error) {
	return ctx.GetStub().CreateCompositeKey(DocKeyPrefix, []string{documentID})
}

// BuildAccessKey creates a composite key for access storage.
// Format: ACC~{documentId}~{userId}
func BuildAccessKey(ctx contractapi.TransactionContextInterface, documentID, userID string) (string, error) {
	return ctx.GetStub().CreateCompositeKey(AccessKeyPrefix, []string{documentID, userID})
}

// =============================================================================
// TIMESTAMP UTILITIES
// =============================================================================

// GetTxTimestamp returns the transaction timestamp from the stub.
// This ensures deterministic execution - never use time.Now()!
func GetTxTimestamp(ctx contractapi.TransactionContextInterface) (string, error) {
	txTimestamp, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return "", fmt.Errorf("failed to get transaction timestamp: %w", err)
	}
	
	// Convert to ISO8601 format
	timestamp := time.Unix(txTimestamp.Seconds, int64(txTimestamp.Nanos)).UTC().Format(time.RFC3339)
	return timestamp, nil
}

// GetTxID returns the transaction ID from the stub.
func GetTxID(ctx contractapi.TransactionContextInterface) string {
	return ctx.GetStub().GetTxID()
}

// =============================================================================
// VALIDATION UTILITIES
// =============================================================================

// ValidateDocumentStatus checks if the document status is valid for operations.
// Returns error if status is DELETED.
func ValidateDocumentStatus(status string) error {
	if status == StatusDeleted {
		return fmt.Errorf("%s: document is deleted", ErrInvalidState)
	}
	return nil
}

// ValidateAccessStatus checks if the access status is valid for operations.
// Returns error if status is REVOKED.
func ValidateAccessStatus(status string) error {
	if status == StatusRevoked {
		return fmt.Errorf("%s: access is revoked", ErrInvalidState)
	}
	return nil
}

// ValidateVersion checks if the provided version matches the current version.
// Used for optimistic locking during updates.
func ValidateVersion(currentVersion, providedVersion int64) error {
	if currentVersion != providedVersion {
		return fmt.Errorf("%s: expected version %d, got %d", ErrVersionMismatch, currentVersion, providedVersion)
	}
	return nil
}

// ValidateOwnership checks if the caller is the owner of the asset.
// Returns an unauthorized error if not.
func ValidateOwnership(caller *CallerInfo, ownerID, ownerMSP string) error {
	if !IsOwner(caller, ownerID, ownerMSP) {
		return fmt.Errorf("%s: caller is not the owner of this asset", ErrUnauthorized)
	}
	return nil
}

// =============================================================================
// STATE UTILITIES
// =============================================================================

// AssetExists checks if an asset exists at the given key.
func AssetExists(ctx contractapi.TransactionContextInterface, key string) (bool, error) {
	data, err := ctx.GetStub().GetState(key)
	if err != nil {
		return false, fmt.Errorf("failed to read from world state: %w", err)
	}
	return data != nil, nil
}

// GetDocument retrieves a document from the ledger by its key.
func GetDocument(ctx contractapi.TransactionContextInterface, key string) (*DocumentAssetNFT, error) {
	data, err := ctx.GetStub().GetState(key)
	if err != nil {
		return nil, fmt.Errorf("failed to read from world state: %w", err)
	}
	if data == nil {
		return nil, nil
	}

	var doc DocumentAssetNFT
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("failed to unmarshal document: %w", err)
	}
	return &doc, nil
}

// GetAccessNFT retrieves an access NFT from the ledger by its key.
func GetAccessNFT(ctx contractapi.TransactionContextInterface, key string) (*AccessNFT, error) {
	data, err := ctx.GetStub().GetState(key)
	if err != nil {
		return nil, fmt.Errorf("failed to read from world state: %w", err)
	}
	if data == nil {
		return nil, nil
	}

	var access AccessNFT
	if err := json.Unmarshal(data, &access); err != nil {
		return nil, fmt.Errorf("failed to unmarshal access NFT: %w", err)
	}
	return &access, nil
}

// SaveState saves an asset to the ledger.
func SaveState(ctx contractapi.TransactionContextInterface, key string, asset interface{}) error {
	data, err := json.Marshal(asset)
	if err != nil {
		return fmt.Errorf("failed to marshal asset: %w", err)
	}
	return ctx.GetStub().PutState(key, data)
}

// =============================================================================
// EVENT UTILITIES
// =============================================================================

// EmitEvent emits a chaincode event with the given name and payload.
func EmitEvent(ctx contractapi.TransactionContextInterface, eventName string, payload EventPayload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal event payload: %w", err)
	}
	return ctx.GetStub().SetEvent(eventName, data)
}
