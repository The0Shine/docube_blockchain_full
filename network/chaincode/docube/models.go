// Package main provides the Docube chaincode implementation
// for document management and access control on Hyperledger Fabric.
package main

// =============================================================================
// DATA MODELS
// =============================================================================

// DocumentAssetNFT represents a document NFT on the ledger.
// It supports versioning, soft deletion, and ownership tracking.
type DocumentAssetNFT struct {
	AssetID      string `json:"assetId"`              // Unique asset identifier
	DocumentID   string `json:"documentId"`           // Document identifier
	DocHash      string `json:"docHash"`              // Hash of document content
	HashAlgo     string `json:"hashAlgo"`             // Hash algorithm used (e.g., SHA256)
	OwnerID      string `json:"ownerId"`              // Owner's identity (from x509)
	OwnerMSP     string `json:"ownerMsp"`             // Owner's MSP ID
	SystemUserId string `json:"systemUserId"`         // System-level user ID (application layer)
	Version      int64  `json:"version"`              // Version number for optimistic locking
	Status       string `json:"status"`               // ACTIVE | DELETED
	CreatedAt    string `json:"createdAt"`            // ISO8601 timestamp
	UpdatedAt    string `json:"updatedAt"`            // ISO8601 timestamp
}

// AccessNFT represents an access token granting access to a document.
// Supports revocation without physical deletion.
// Granter info is tracked in the Timeline record (ActorID, ActorMSP, TxID).
//
// NOTE: Access NFTs are keyed on the ledger by (documentId, OwnerID) where
// OwnerID is the application-layer system UUID (passed via granteeUserID parameter),
// NOT the Fabric certificate identity. This matches how CheckAccessPermission
// and GetAccess look up access records using the application-layer user ID.
type AccessNFT struct {
	AccessNFTID  string `json:"accessNftId"`                            // Unique access NFT identifier
	DocumentID   string `json:"documentId"`                             // Referenced document ID
	OwnerID      string `json:"ownerId"`                                // Access holder's application-layer system UUID (used as ledger key via BuildAccessKey)
	OwnerMSP     string `json:"ownerMsp"`                               // Access holder's MSP
	SystemUserId string `json:"systemUserId"`                           // Access holder's application-layer user ID (same as OwnerID for system-UUID-based grants)
	Status       string `json:"status"`                                 // ACTIVE | REVOKED
	GrantedAt    string `json:"grantedAt"`                              // When access was granted
	RevokedBy    string `json:"revokedBy,omitempty" metadata:",optional"` // Who revoked (if revoked)
	RevokedAt    string `json:"revokedAt,omitempty" metadata:",optional"` // When revoked (if revoked)
}

// AuditRecord represents a single history entry for audit purposes.
type AuditRecord struct {
	TxID      string      `json:"txId"`      // Transaction ID
	Timestamp string      `json:"timestamp"` // ISO8601 timestamp
	Value     interface{} `json:"value"`     // The state value at this point
	IsDelete  bool        `json:"isDelete"`  // Whether this was a delete operation
}

// EventPayload is the standard structure for chaincode events.
type EventPayload struct {
	AssetID    string `json:"assetId"`    // Asset identifier
	DocumentID string `json:"documentId"` // Document identifier
	ActorID    string `json:"actorId"`    // Who performed the action
	Timestamp  string `json:"timestamp"`  // When the action occurred
}

// AdminAuditPayload is the structure for admin override audit events.
// Used to log all admin actions for compliance and auditing.
type AdminAuditPayload struct {
	AssetID    string `json:"assetId"`    // Asset identifier
	DocumentID string `json:"documentId"` // Document identifier
	Action     string `json:"action"`     // Action performed (UpdateDocument, GrantAccess, etc.)
	ActorID    string `json:"actorId"`    // Who performed the action
	ActorMSP   string `json:"actorMsp"`   // Actor's MSP
	Role       string `json:"role"`       // Role used (ADMIN)
	Reason     string `json:"reason"`     // Optional reason for admin override
	Timestamp  string `json:"timestamp"`  // When the action occurred
	TxID       string `json:"txId"`       // Transaction ID
}

// TimelineRecord represents a single entry in the Document Timeline Audit Log.
// Each write operation on a document (or its access records) appends one record.
type TimelineRecord struct {
	DocumentID string            `json:"documentId"` // Referenced document
	TxID       string            `json:"txId"`       // Transaction ID
	Timestamp  string            `json:"timestamp"`  // ISO8601 timestamp
	Action     string            `json:"action"`     // Timeline action constant
	ActorID    string            `json:"actorId"`    // Caller's Fabric identity
	ActorMSP   string            `json:"actorMsp"`   // Caller's MSP
	Details    map[string]string `json:"details"`    // Extra info (granteeId, oldOwner, newOwner, etc.)
}

// AccessCheckResult is returned by CheckAccessPermission.
type AccessCheckResult struct {
	Allowed    bool   `json:"allowed"`
	Reason     string `json:"reason"`
	DocumentID string `json:"documentId"`
	CallerID   string `json:"callerId"`
	Action     string `json:"action"`
}

// =============================================================================
// STATUS CONSTANTS
// =============================================================================

const (
	// Document statuses
	StatusActive  = "ACTIVE"
	StatusDeleted = "DELETED"

	// Access statuses
	StatusRevoked = "REVOKED"
)

// =============================================================================
// ERROR CONSTANTS
// =============================================================================

const (
	ErrNotFound        = "ERR_NOT_FOUND"
	ErrAlreadyExists   = "ERR_ALREADY_EXISTS"
	ErrUnauthorized    = "ERR_UNAUTHORIZED"
	ErrNotOwner        = "ERR_NOT_OWNER"
	ErrAdminOverride   = "ERR_ADMIN_OVERRIDE"
	ErrInvalidState    = "ERR_INVALID_STATE"
	ErrVersionMismatch = "ERR_VERSION_MISMATCH"
)

// =============================================================================
// EVENT NAMES
// =============================================================================

const (
	EventDocumentCreated     = "DocumentCreated"
	EventDocumentUpdated     = "DocumentUpdated"
	EventDocumentTransferred = "DocumentTransferred"
	EventDocumentDeleted     = "DocumentDeleted"
	EventAccessGranted       = "AccessGranted"
	EventAccessRevoked       = "AccessRevoked"
	EventAdminAction         = "AdminAction" // Special event for admin override audit
)

// =============================================================================
// KEY PREFIXES
// =============================================================================

const (
	DocKeyPrefix        = "DOC"
	AccessKeyPrefix     = "ACC"
	TimelineKeyPrefix   = "DOCLOG"
	AdminAuditKeyPrefix = "ADMINAUDIT"
)

// =============================================================================
// TIMELINE ACTION CONSTANTS
// =============================================================================

const (
	ActionDocumentCreated       = "DOCUMENT_CREATED"
	ActionDocumentUpdated       = "DOCUMENT_UPDATED"
	ActionOwnershipTransferred  = "OWNERSHIP_TRANSFERRED"
	ActionDocumentDeleted       = "DOCUMENT_DELETED"
	ActionAccessGranted         = "ACCESS_GRANTED"
	ActionAccessRevoked         = "ACCESS_REVOKED"
)

// =============================================================================
// ACCESS CHECK REASON CONSTANTS
// =============================================================================

const (
	ReasonOwner      = "OWNER"
	ReasonGranted    = "GRANTED"
	ReasonNotGranted = "NOT_GRANTED"
	ReasonDocNotFound  = "DOC_NOT_FOUND"
	ReasonDocInactive  = "DOC_INACTIVE"
)
