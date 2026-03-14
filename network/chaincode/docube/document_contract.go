// Package main provides the DocumentContract for document management.
package main

import (
	"encoding/json"
	"fmt"

	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

// DocumentContract provides functions for managing document NFTs.
type DocumentContract struct {
	contractapi.Contract
}

// =============================================================================
// DOCUMENT CRUD OPERATIONS
// =============================================================================

// CreateDocument creates a new document NFT on the ledger.
// Only the creator becomes the owner.
// systemUserId is the application-level user ID (not Fabric identity)
// Emits: DocumentCreated event
func (dc *DocumentContract) CreateDocument(
	ctx contractapi.TransactionContextInterface,
	documentID string,
	docHash string,
	hashAlgo string,
	systemUserId string,
) error {
	// Get caller identity
	caller, err := GetCallerInfo(ctx)
	if err != nil {
		return err
	}

	// Build document key
	key, err := BuildDocumentKey(ctx, documentID)
	if err != nil {
		return fmt.Errorf("failed to build document key: %w", err)
	}

	// Check if document already exists
	exists, err := AssetExists(ctx, key)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("%s: document %s already exists", ErrAlreadyExists, documentID)
	}

	// Get transaction timestamp
	timestamp, err := GetTxTimestamp(ctx)
	if err != nil {
		return err
	}

	// Create new document
	doc := DocumentAssetNFT{
		AssetID:      fmt.Sprintf("DOC-%s", documentID),
		DocumentID:   documentID,
		DocHash:      docHash,
		HashAlgo:     hashAlgo,
		OwnerID:      caller.ID,
		OwnerMSP:     caller.MSPID,
		SystemUserId: systemUserId,
		Version:      1,
		Status:       StatusActive,
		CreatedAt:    timestamp,
		UpdatedAt:    timestamp,
	}

	// Save to ledger
	if err := SaveState(ctx, key, doc); err != nil {
		return fmt.Errorf("failed to save document: %w", err)
	}

	// Emit event
	if err := EmitEvent(ctx, EventDocumentCreated, EventPayload{
		AssetID:    doc.AssetID,
		DocumentID: documentID,
		ActorID:    caller.ID,
		Timestamp:  timestamp,
	}); err != nil {
		return err
	}

	// Append timeline record
	return AppendTimeline(ctx, documentID, ActionDocumentCreated, map[string]string{
		"assetId":  doc.AssetID,
		"docHash":  docHash,
		"hashAlgo": hashAlgo,
	})
}

// UpdateDocument updates an existing document's hash.
// Requires version check for optimistic locking.
// Authorization: ADMIN or OWNER can update.
// Emits: DocumentUpdated event (+ AdminAction if admin override)
func (dc *DocumentContract) UpdateDocument(
	ctx contractapi.TransactionContextInterface,
	documentID string,
	newDocHash string,
	newHashAlgo string,
	expectedVersion int64,
) error {
	// Get caller identity
	caller, err := GetCallerInfo(ctx)
	if err != nil {
		return err
	}

	// Build document key
	key, err := BuildDocumentKey(ctx, documentID)
	if err != nil {
		return fmt.Errorf("failed to build document key: %w", err)
	}

	// Get existing document
	doc, err := GetDocument(ctx, key)
	if err != nil {
		return err
	}
	if doc == nil {
		return fmt.Errorf("%s: document %s not found", ErrNotFound, documentID)
	}

	// Validate document status
	if err := ValidateDocumentStatus(doc.Status); err != nil {
		return err
	}

	// Authorization check (ADMIN > OWNER > reject)
	authResult, err := AuthorizeWrite(ctx, doc, "UpdateDocument")
	if err != nil {
		return err
	}

	// Validate version (optimistic locking)
	if err := ValidateVersion(doc.Version, expectedVersion); err != nil {
		return err
	}

	// Get transaction timestamp
	timestamp, err := GetTxTimestamp(ctx)
	if err != nil {
		return err
	}

	// Update document
	doc.DocHash = newDocHash
	doc.HashAlgo = newHashAlgo
	doc.Version++
	doc.UpdatedAt = timestamp

	// Save to ledger
	if err := SaveState(ctx, key, doc); err != nil {
		return fmt.Errorf("failed to save document: %w", err)
	}

	// Emit admin audit event if admin override
	if authResult.IsAdmin {
		if err := EmitAdminAuditEvent(ctx, "UpdateDocument", doc.AssetID, documentID, ""); err != nil {
			return err
		}
	}

	// Emit event
	if err := EmitEvent(ctx, EventDocumentUpdated, EventPayload{
		AssetID:    doc.AssetID,
		DocumentID: documentID,
		ActorID:    caller.ID,
		Timestamp:  timestamp,
	}); err != nil {
		return err
	}

	// Append timeline record
	return AppendTimeline(ctx, documentID, ActionDocumentUpdated, map[string]string{
		"newDocHash":  newDocHash,
		"newHashAlgo": newHashAlgo,
		"version":     fmt.Sprintf("%d", doc.Version),
	})
}

// TransferOwnership transfers document ownership to a new owner.
// Authorization: ADMIN or OWNER can transfer.
// Emits: DocumentTransferred event (+ AdminAction if admin override)
func (dc *DocumentContract) TransferOwnership(
	ctx contractapi.TransactionContextInterface,
	documentID string,
	newOwnerID string,
	newOwnerMSP string,
) error {
	// Build document key
	key, err := BuildDocumentKey(ctx, documentID)
	if err != nil {
		return fmt.Errorf("failed to build document key: %w", err)
	}

	// Get existing document
	doc, err := GetDocument(ctx, key)
	if err != nil {
		return err
	}
	if doc == nil {
		return fmt.Errorf("%s: document %s not found", ErrNotFound, documentID)
	}

	// Validate document status
	if err := ValidateDocumentStatus(doc.Status); err != nil {
		return err
	}

	// Authorization check (ADMIN > OWNER > reject)
	authResult, err := AuthorizeWrite(ctx, doc, "TransferOwnership")
	if err != nil {
		return err
	}

	// Get transaction timestamp
	timestamp, err := GetTxTimestamp(ctx)
	if err != nil {
		return err
	}

	// Store old owner for event
	oldOwnerID := doc.OwnerID

	// Transfer ownership
	doc.OwnerID = newOwnerID
	doc.OwnerMSP = newOwnerMSP
	doc.Version++
	doc.UpdatedAt = timestamp

	// Save to ledger
	if err := SaveState(ctx, key, doc); err != nil {
		return fmt.Errorf("failed to save document: %w", err)
	}

	// Emit admin audit event if admin override
	if authResult.IsAdmin {
		if err := EmitAdminAuditEvent(ctx, "TransferOwnership", doc.AssetID, documentID, ""); err != nil {
			return err
		}
	}

	// Emit event (using ActorID for old owner, can extend EventPayload if needed)
	if err := EmitEvent(ctx, EventDocumentTransferred, EventPayload{
		AssetID:    doc.AssetID,
		DocumentID: documentID,
		ActorID:    oldOwnerID,
		Timestamp:  timestamp,
	}); err != nil {
		return err
	}

	// Append timeline record
	return AppendTimeline(ctx, documentID, ActionOwnershipTransferred, map[string]string{
		"oldOwnerId": oldOwnerID,
		"newOwnerId": newOwnerID,
		"newOwnerMsp": newOwnerMSP,
	})
}

// SoftDeleteDocument marks a document as deleted (soft delete).
// Authorization: ADMIN or OWNER can delete.
// Emits: DocumentDeleted event (+ AdminAction if admin override)
func (dc *DocumentContract) SoftDeleteDocument(
	ctx contractapi.TransactionContextInterface,
	documentID string,
) error {
	// Get caller identity
	caller, err := GetCallerInfo(ctx)
	if err != nil {
		return err
	}

	// Build document key
	key, err := BuildDocumentKey(ctx, documentID)
	if err != nil {
		return fmt.Errorf("failed to build document key: %w", err)
	}

	// Get existing document
	doc, err := GetDocument(ctx, key)
	if err != nil {
		return err
	}
	if doc == nil {
		return fmt.Errorf("%s: document %s not found", ErrNotFound, documentID)
	}

	// Validate document status (can't delete already deleted)
	if err := ValidateDocumentStatus(doc.Status); err != nil {
		return err
	}

	// Authorization check (ADMIN > OWNER > reject)
	authResult, err := AuthorizeWrite(ctx, doc, "SoftDeleteDocument")
	if err != nil {
		return err
	}

	// Get transaction timestamp
	timestamp, err := GetTxTimestamp(ctx)
	if err != nil {
		return err
	}

	// Soft delete
	doc.Status = StatusDeleted
	doc.Version++
	doc.UpdatedAt = timestamp

	// Save to ledger
	if err := SaveState(ctx, key, doc); err != nil {
		return fmt.Errorf("failed to save document: %w", err)
	}

	// Emit admin audit event if admin override
	if authResult.IsAdmin {
		if err := EmitAdminAuditEvent(ctx, "SoftDeleteDocument", doc.AssetID, documentID, ""); err != nil {
			return err
		}
	}

	// Emit event
	if err := EmitEvent(ctx, EventDocumentDeleted, EventPayload{
		AssetID:    doc.AssetID,
		DocumentID: documentID,
		ActorID:    caller.ID,
		Timestamp:  timestamp,
	}); err != nil {
		return err
	}

	// Append timeline record
	return AppendTimeline(ctx, documentID, ActionDocumentDeleted, nil)
}

// =============================================================================
// DOCUMENT QUERY OPERATIONS
// =============================================================================

// GetDocument retrieves a single document by ID.
func (dc *DocumentContract) GetDocument(
	ctx contractapi.TransactionContextInterface,
	documentID string,
) (*DocumentAssetNFT, error) {
	// Build document key
	key, err := BuildDocumentKey(ctx, documentID)
	if err != nil {
		return nil, fmt.Errorf("failed to build document key: %w", err)
	}

	// Get document
	doc, err := GetDocument(ctx, key)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, fmt.Errorf("%s: document %s not found", ErrNotFound, documentID)
	}

	return doc, nil
}

// GetAllDocuments retrieves all active documents.
// Uses CouchDB rich query for efficient filtering.
func (dc *DocumentContract) GetAllDocuments(
	ctx contractapi.TransactionContextInterface,
) ([]*DocumentAssetNFT, error) {
	// CouchDB rich query to find all active documents
	queryString := `{
		"selector": {
			"status": "ACTIVE"
		}
	}`

	resultsIterator, err := ctx.GetStub().GetQueryResult(queryString)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer resultsIterator.Close()

	var documents []*DocumentAssetNFT
	for resultsIterator.HasNext() {
		queryResult, err := resultsIterator.Next()
		if err != nil {
			return nil, err
		}

		var doc DocumentAssetNFT
		if err := json.Unmarshal(queryResult.Value, &doc); err != nil {
			return nil, err
		}
		documents = append(documents, &doc)
	}

	return documents, nil
}

// GetDocumentHistory retrieves the complete history of a document.
// Uses GetHistoryForKey for audit purposes.
func (dc *DocumentContract) GetDocumentHistory(
	ctx contractapi.TransactionContextInterface,
	documentID string,
) ([]*AuditRecord, error) {
	// Build document key
	key, err := BuildDocumentKey(ctx, documentID)
	if err != nil {
		return nil, fmt.Errorf("failed to build document key: %w", err)
	}

	// Get history
	historyIterator, err := ctx.GetStub().GetHistoryForKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed to get history: %w", err)
	}
	defer historyIterator.Close()

	var history []*AuditRecord
	for historyIterator.HasNext() {
		modification, err := historyIterator.Next()
		if err != nil {
			return nil, err
		}

		var value interface{}
		if !modification.IsDelete && len(modification.Value) > 0 {
			var doc DocumentAssetNFT
			if err := json.Unmarshal(modification.Value, &doc); err != nil {
				return nil, err
			}
			value = doc
		}

		record := &AuditRecord{
			TxID:      modification.TxId,
			Timestamp: modification.Timestamp.AsTime().UTC().Format("2006-01-02T15:04:05Z"),
			Value:     value,
			IsDelete:  modification.IsDelete,
		}
		history = append(history, record)
	}

	return history, nil
}
