// Package main provides authorization utilities for the Docube chaincode.
// Implements USER/OWNER/ADMIN role-based access control.
package main

import (
	"encoding/json"
	"fmt"

	"github.com/hyperledger/fabric-chaincode-go/v2/pkg/cid"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

// =============================================================================
// ROLE CONSTANTS
// =============================================================================

const (
	// RoleUser is the default role for any valid identity
	RoleUser = "USER"
	// RoleOwner is derived when caller owns the document
	RoleOwner = "OWNER"
	// RoleAdmin has full permissions on all documents
	RoleAdmin = "ADMIN"

	// AdminMSP is the MSP ID that has admin privileges
	AdminMSP = "AdminOrgMSP"

	// AdminCertAttribute is the certificate attribute for admin role
	AdminCertAttribute = "role"
	AdminCertValue     = "admin"
)

// =============================================================================
// AUTHORIZATION RESULT
// =============================================================================

// AuthResult contains the result of an authorization check
type AuthResult struct {
	Allowed  bool   // Whether the action is allowed
	IsAdmin  bool   // Whether caller is admin (for audit logging)
	Role     string // The role used for authorization (USER/OWNER/ADMIN)
	CallerID string // Caller identity for logging
}

// =============================================================================
// ADMIN CHECK
// =============================================================================

// IsAdmin checks if the caller has ADMIN role.
// Admin is identified by:
// 1. MSP ID == AdminOrgMSP
// 2. OR certificate attribute role=admin
func IsAdmin(ctx contractapi.TransactionContextInterface) (bool, error) {
	stub := ctx.GetStub()

	// Check MSP ID first (most common case)
	mspID, err := cid.GetMSPID(stub)
	if err != nil {
		return false, fmt.Errorf("failed to get MSP ID: %w", err)
	}

	if mspID == AdminMSP {
		return true, nil
	}

	// Check certificate attribute as fallback
	roleAttr, found, err := cid.GetAttributeValue(stub, AdminCertAttribute)
	if err != nil {
		// Attribute not found is not an error, just means not admin
		return false, nil
	}

	if found && roleAttr == AdminCertValue {
		return true, nil
	}

	return false, nil
}

// =============================================================================
// OWNER CHECK
// =============================================================================

// IsDocumentOwner checks if the caller owns the document.
// Returns true if caller's ID matches document's OwnerID.
// Note: We only check ID, not MSP, to allow ownership transfer across orgs.
func IsDocumentOwner(caller *CallerInfo, doc *DocumentAssetNFT) bool {
	if doc == nil || caller == nil {
		return false
	}
	return caller.ID == doc.OwnerID
}

// =============================================================================
// AUTHORIZATION PRIORITY
// =============================================================================

// AuthorizeWrite performs authorization check with the following priority:
// 1. If caller is ADMIN → allow (isAdmin=true)
// 2. If action is CreateDocument → allow (anyone can create)
// 3. If caller is OWNER of document → allow (isAdmin=false)
// 4. Else → reject with ERR_UNAUTHORIZED
//
// Parameters:
// - ctx: transaction context
// - doc: document to check ownership against (can be nil for CreateDocument)
// - action: action being performed (for audit logging)
//
// Returns AuthResult with authorization details
func AuthorizeWrite(
	ctx contractapi.TransactionContextInterface,
	doc *DocumentAssetNFT,
	action string,
) (*AuthResult, error) {
	// Get caller info
	caller, err := GetCallerInfo(ctx)
	if err != nil {
		return nil, err
	}

	result := &AuthResult{
		CallerID: caller.ID,
	}

	// Priority 1: Check if caller is ADMIN
	isAdmin, err := IsAdmin(ctx)
	if err != nil {
		return nil, err
	}

	if isAdmin {
		result.Allowed = true
		result.IsAdmin = true
		result.Role = RoleAdmin
		return result, nil
	}

	// Priority 2: CreateDocument is allowed for all users
	// (doc will be nil for create operations)
	if doc == nil {
		result.Allowed = true
		result.IsAdmin = false
		result.Role = RoleUser
		return result, nil
	}

	// Priority 3: Check if caller is OWNER
	if IsDocumentOwner(caller, doc) {
		result.Allowed = true
		result.IsAdmin = false
		result.Role = RoleOwner
		return result, nil
	}

	// Priority 4: Not authorized
	result.Allowed = false
	result.Role = RoleUser
	return result, fmt.Errorf("%s: caller %s is not authorized to %s document %s",
		ErrUnauthorized, caller.ID, action, doc.DocumentID)
}

// =============================================================================
// AUDIT HELPERS
// =============================================================================

// EmitAdminAuditEvent persists an admin audit record to the ledger using PutState.
//
// NOTE: Fabric only allows one SetEvent per transaction — calling SetEvent twice
// would overwrite the domain event (DocumentUpdated, AccessGranted, etc.) with the
// admin audit event. To avoid this collision, admin audit data is written directly
// to the ledger under key ADMINAUDIT~{txId}. This ensures:
//   - Domain events (DocumentUpdated, AccessGranted, etc.) are emitted correctly.
//   - Admin audit records are immutably stored on-chain and queryable via GetState.
func EmitAdminAuditEvent(
	ctx contractapi.TransactionContextInterface,
	action string,
	assetID string,
	documentID string,
	reason string,
) error {
	caller, err := GetCallerInfo(ctx)
	if err != nil {
		return err
	}

	timestamp, err := GetTxTimestamp(ctx)
	if err != nil {
		return err
	}

	txID := GetTxID(ctx)

	payload := AdminAuditPayload{
		AssetID:    assetID,
		DocumentID: documentID,
		Action:     action,
		ActorID:    caller.ID,
		ActorMSP:   caller.MSPID,
		Role:       RoleAdmin,
		Reason:     reason,
		Timestamp:  timestamp,
		TxID:       txID,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal admin audit payload: %w", err)
	}

	// Write to ledger instead of SetEvent to avoid overwriting the domain event.
	// Key: ADMINAUDIT~{txId} — unique per transaction, queryable via GetStateByPartialCompositeKey.
	key, err := ctx.GetStub().CreateCompositeKey(AdminAuditKeyPrefix, []string{txID})
	if err != nil {
		return fmt.Errorf("failed to create admin audit key: %w", err)
	}
	return ctx.GetStub().PutState(key, data)
}
