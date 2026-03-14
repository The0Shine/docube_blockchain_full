// Package main provides the AccessContract for access control management.
package main

import (
	"encoding/json"
	"fmt"

	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

// AccessContract provides functions for managing access NFTs.
type AccessContract struct {
	contractapi.Contract
}

// =============================================================================
// ACCESS CONTROL OPERATIONS
// =============================================================================

// GrantAccess grants access to a document for a specific user.
// Authorization: ADMIN or OWNER can grant access.
// systemUserId is the application-level user ID (not Fabric identity)
// Emits: AccessGranted event (+ AdminAction if admin override)
func (ac *AccessContract) GrantAccess(
	ctx contractapi.TransactionContextInterface,
	documentID string,
	granteeUserID string,
	granteeUserMSP string,
	systemUserId string,
) error {
	// Get caller identity
	caller, err := GetCallerInfo(ctx)
	if err != nil {
		return err
	}

	// First, verify the document exists
	docKey, err := BuildDocumentKey(ctx, documentID)
	if err != nil {
		return fmt.Errorf("failed to build document key: %w", err)
	}

	doc, err := GetDocument(ctx, docKey)
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
	authResult, err := AuthorizeWrite(ctx, doc, "GrantAccess")
	if err != nil {
		return err
	}

	// Build access key
	accessKey, err := BuildAccessKey(ctx, documentID, granteeUserID)
	if err != nil {
		return fmt.Errorf("failed to build access key: %w", err)
	}

	// Check if access already exists
	existingAccess, err := GetAccessNFT(ctx, accessKey)
	if err != nil {
		return err
	}
	if existingAccess != nil && existingAccess.Status == StatusActive {
		return fmt.Errorf("%s: access already granted for document %s to user %s", 
			ErrAlreadyExists, documentID, granteeUserID)
	}

	// Get transaction timestamp
	timestamp, err := GetTxTimestamp(ctx)
	if err != nil {
		return err
	}

	// Create access NFT
	accessNFT := AccessNFT{
		AccessNFTID:  fmt.Sprintf("ACC-%s-%s", documentID, granteeUserID),
		DocumentID:   documentID,
		OwnerID:      granteeUserID,
		OwnerMSP:     granteeUserMSP,
		SystemUserId: systemUserId,
		Status:       StatusActive,
		GrantedAt:    timestamp,
	}

	// Save to ledger
	if err := SaveState(ctx, accessKey, accessNFT); err != nil {
		return fmt.Errorf("failed to save access NFT: %w", err)
	}

	// Emit admin audit event if admin override
	if authResult.IsAdmin {
		if err := EmitAdminAuditEvent(ctx, "GrantAccess", accessNFT.AccessNFTID, documentID, ""); err != nil {
			return err
		}
	}

	// Emit event
	if err := EmitEvent(ctx, EventAccessGranted, EventPayload{
		AssetID:    accessNFT.AccessNFTID,
		DocumentID: documentID,
		ActorID:    caller.ID,
		Timestamp:  timestamp,
	}); err != nil {
		return err
	}

	// Append timeline record
	return AppendTimeline(ctx, documentID, ActionAccessGranted, map[string]string{
		"granteeUserId":  granteeUserID,
		"granteeUserMsp": granteeUserMSP,
		"systemUserId":   systemUserId,
	})
}

// RevokeAccess revokes access to a document for a specific user.
// Authorization: ADMIN or OWNER can revoke access.
// Note: This updates status to REVOKED, not physical delete.
// Emits: AccessRevoked event (+ AdminAction if admin override)
func (ac *AccessContract) RevokeAccess(
	ctx contractapi.TransactionContextInterface,
	documentID string,
	userID string,
) error {
	// Get caller identity
	caller, err := GetCallerInfo(ctx)
	if err != nil {
		return err
	}

	// First, verify the document exists
	docKey, err := BuildDocumentKey(ctx, documentID)
	if err != nil {
		return fmt.Errorf("failed to build document key: %w", err)
	}

	doc, err := GetDocument(ctx, docKey)
	if err != nil {
		return err
	}
	if doc == nil {
		return fmt.Errorf("%s: document %s not found", ErrNotFound, documentID)
	}

	// Authorization check (ADMIN > OWNER > reject)
	authResult, err := AuthorizeWrite(ctx, doc, "RevokeAccess")
	if err != nil {
		return err
	}

	// Build access key
	accessKey, err := BuildAccessKey(ctx, documentID, userID)
	if err != nil {
		return fmt.Errorf("failed to build access key: %w", err)
	}

	// Get existing access
	access, err := GetAccessNFT(ctx, accessKey)
	if err != nil {
		return err
	}
	if access == nil {
		return fmt.Errorf("%s: access not found for document %s and user %s", 
			ErrNotFound, documentID, userID)
	}

	// Validate access status (can't revoke already revoked)
	if err := ValidateAccessStatus(access.Status); err != nil {
		return err
	}

	// Get transaction timestamp
	timestamp, err := GetTxTimestamp(ctx)
	if err != nil {
		return err
	}

	// Revoke access (status update, NOT delete)
	access.Status = StatusRevoked
	access.RevokedBy = caller.ID
	access.RevokedAt = timestamp

	// Save to ledger
	if err := SaveState(ctx, accessKey, access); err != nil {
		return fmt.Errorf("failed to save access NFT: %w", err)
	}

	// Emit admin audit event if admin override
	if authResult.IsAdmin {
		if err := EmitAdminAuditEvent(ctx, "RevokeAccess", access.AccessNFTID, documentID, ""); err != nil {
			return err
		}
	}

	// Emit event
	if err := EmitEvent(ctx, EventAccessRevoked, EventPayload{
		AssetID:    access.AccessNFTID,
		DocumentID: documentID,
		ActorID:    caller.ID,
		Timestamp:  timestamp,
	}); err != nil {
		return err
	}

	// Append timeline record
	return AppendTimeline(ctx, documentID, ActionAccessRevoked, map[string]string{
		"userId": userID,
	})
}

// =============================================================================
// ACCESS QUERY OPERATIONS
// =============================================================================

// GetAccess retrieves a single access record.
func (ac *AccessContract) GetAccess(
	ctx contractapi.TransactionContextInterface,
	documentID string,
	userID string,
) (*AccessNFT, error) {
	// Build access key
	accessKey, err := BuildAccessKey(ctx, documentID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to build access key: %w", err)
	}

	// Get access
	access, err := GetAccessNFT(ctx, accessKey)
	if err != nil {
		return nil, err
	}
	if access == nil {
		return nil, fmt.Errorf("%s: access not found for document %s and user %s", 
			ErrNotFound, documentID, userID)
	}

	return access, nil
}

// GetAllAccessByDocument retrieves all access records for a document.
// Uses CouchDB rich query for efficient filtering.
func (ac *AccessContract) GetAllAccessByDocument(
	ctx contractapi.TransactionContextInterface,
	documentID string,
) ([]*AccessNFT, error) {
	// CouchDB rich query
	queryString := fmt.Sprintf(`{
		"selector": {
			"documentId": "%s"
		}
	}`, documentID)

	resultsIterator, err := ctx.GetStub().GetQueryResult(queryString)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer resultsIterator.Close()

	var accessRecords []*AccessNFT
	for resultsIterator.HasNext() {
		queryResult, err := resultsIterator.Next()
		if err != nil {
			return nil, err
		}

		var access AccessNFT
		if err := json.Unmarshal(queryResult.Value, &access); err != nil {
			return nil, err
		}
		accessRecords = append(accessRecords, &access)
	}

	return accessRecords, nil
}

// GetAllAccessByUser retrieves all access records for a user.
// Uses CouchDB rich query for efficient database-level filtering.
func (ac *AccessContract) GetAllAccessByUser(
	ctx contractapi.TransactionContextInterface,
	userID string,
) ([]*AccessNFT, error) {
	// CouchDB rich query with userID and active status filter
	queryString := fmt.Sprintf(`{
		"selector": {
			"ownerId": "%s",
			"status": "ACTIVE"
		}
	}`, userID)

	resultsIterator, err := ctx.GetStub().GetQueryResult(queryString)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer resultsIterator.Close()

	var accessRecords []*AccessNFT
	for resultsIterator.HasNext() {
		queryResult, err := resultsIterator.Next()
		if err != nil {
			return nil, err
		}

		var access AccessNFT
		if err := json.Unmarshal(queryResult.Value, &access); err != nil {
			return nil, err
		}
		accessRecords = append(accessRecords, &access)
	}

	return accessRecords, nil
}

// GetAccessHistory retrieves the complete history of an access record.
// Uses GetHistoryForKey for audit purposes.
func (ac *AccessContract) GetAccessHistory(
	ctx contractapi.TransactionContextInterface,
	documentID string,
	userID string,
) ([]*AuditRecord, error) {
	// Build access key
	accessKey, err := BuildAccessKey(ctx, documentID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to build access key: %w", err)
	}

	// Get history
	historyIterator, err := ctx.GetStub().GetHistoryForKey(accessKey)
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
			var access AccessNFT
			if err := json.Unmarshal(modification.Value, &access); err != nil {
				return nil, err
			}
			value = access
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

// =============================================================================
// ACCESS PERMISSION CHECK (READ-ONLY)
// =============================================================================

// CheckAccessPermission checks whether a given systemUserId has access
// to the specified document. This is a read-only (evaluate) function.
//
// IMPORTANT: Access NFTs are keyed by application-layer system UUID (not Fabric identity),
// so this function accepts systemUserId as an explicit parameter rather than
// deriving it from the Fabric caller context.
//
// Logic:
//  1. Load document — if not found → DOC_NOT_FOUND
//  2. If document status != ACTIVE → DOC_INACTIVE
//  3. If systemUserId == doc.SystemUserId → OWNER (always allowed)
//  4. Check AccessNFT keyed by systemUserId → GRANTED or NOT_GRANTED
//
// Returns a JSON string: {"allowed":bool,"reason":"...","documentId":"...","callerId":"...","action":"..."}
func (ac *AccessContract) CheckAccessPermission(
	ctx contractapi.TransactionContextInterface,
	documentID string,
	systemUserId string,
	action string,
) (string, error) {
	result := AccessCheckResult{
		DocumentID: documentID,
		CallerID:   systemUserId,
		Action:     action,
	}

	// Load document
	docKey, err := BuildDocumentKey(ctx, documentID)
	if err != nil {
		return "", fmt.Errorf("failed to build document key: %w", err)
	}

	doc, err := GetDocument(ctx, docKey)
	if err != nil {
		return "", fmt.Errorf("failed to get document: %w", err)
	}
	if doc == nil {
		result.Allowed = false
		result.Reason = ReasonDocNotFound
		return marshalResult(result)
	}

	// Check document status
	if doc.Status != StatusActive {
		result.Allowed = false
		result.Reason = ReasonDocInactive
		return marshalResult(result)
	}

	// Owner check: compare application-layer system UUID against doc.SystemUserId
	if systemUserId == doc.SystemUserId {
		result.Allowed = true
		result.Reason = ReasonOwner
		return marshalResult(result)
	}

	// Check access NFT keyed by systemUserId (matches how GrantAccess stores it)
	accessKey, err := BuildAccessKey(ctx, documentID, systemUserId)
	if err != nil {
		return "", fmt.Errorf("failed to build access key: %w", err)
	}

	access, err := GetAccessNFT(ctx, accessKey)
	if err != nil {
		return "", fmt.Errorf("failed to get access NFT: %w", err)
	}

	if access != nil && access.Status == StatusActive {
		result.Allowed = true
		result.Reason = ReasonGranted
		return marshalResult(result)
	}

	result.Allowed = false
	result.Reason = ReasonNotGranted
	return marshalResult(result)
}

// marshalResult helper to JSON-encode AccessCheckResult.
func marshalResult(result AccessCheckResult) (string, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal access check result: %w", err)
	}
	return string(data), nil
}

// =============================================================================
// DOCUMENT TIMELINE QUERY
// =============================================================================

// GetDocumentTimeline retrieves the complete timeline (audit log) for a document.
// Returns all timeline records sorted by composite key order (chronological).
func (ac *AccessContract) GetDocumentTimeline(
	ctx contractapi.TransactionContextInterface,
	documentID string,
) ([]*TimelineRecord, error) {
	return GetDocumentTimelineRecords(ctx, documentID)
}
