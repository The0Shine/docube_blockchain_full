package kafka

// KafkaConfig holds all configuration for the Kafka consumer.
type KafkaConfig struct {
	Brokers       []string
	GroupID       string
	SASLUsername  string
	SASLPassword  string
	SASLEnabled   bool
}

// Topic names consumed by the blockchain service.
const (
	TopicDocumentCreate = "docube.document.create"
	TopicDocumentUpdate = "docube.document.update"
	TopicAccessGrant    = "docube.access.grant"
	TopicAccessRevoke   = "docube.access.revoke"
)

// =============================================================================
// KAFKA MESSAGE PAYLOADS
// =============================================================================

// CreateDocumentEvent is the Kafka message payload for docube.document.create
// Publish this JSON to topic: docube.document.create
// Example:
//
//	{
//	  "documentId": "doc-001",
//	  "docHash": "abc123...",
//	  "hashAlgo": "SHA256",
//	  "systemUserId": "user-uuid-from-auth"
//	}
type CreateDocumentEvent struct {
	DocumentID   string `json:"documentId"`
	DocHash      string `json:"docHash"`
	HashAlgo     string `json:"hashAlgo"`
	SystemUserId string `json:"systemUserId"`
}

// UpdateDocumentEvent is the Kafka message payload for docube.document.update
// Publish this JSON to topic: docube.document.update
// Example:
//
//	{
//	  "documentId": "doc-001",
//	  "newDocHash": "newHash...",
//	  "newHashAlgo": "SHA256",
//	  "expectedVersion": 1
//	}
type UpdateDocumentEvent struct {
	DocumentID      string `json:"documentId"`
	NewDocHash      string `json:"newDocHash"`
	NewHashAlgo     string `json:"newHashAlgo"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

// GrantAccessEvent is the Kafka message payload for docube.access.grant
// Publish this JSON to topic: docube.access.grant
// Example:
//
//	{
//	  "documentId": "doc-001",
//	  "granteeUserId": "user-bob-uuid",
//	  "granteeUserMsp": "AdminOrgMSP",
//	  "systemUserId": "user-alice-uuid"
//	}
type GrantAccessEvent struct {
	DocumentID     string `json:"documentId"`
	GranteeUserID  string `json:"granteeUserId"`
	GranteeUserMSP string `json:"granteeUserMsp"`
	SystemUserId   string `json:"systemUserId"`
}

// RevokeAccessEvent is the Kafka message payload for docube.access.revoke
// Publish this JSON to topic: docube.access.revoke
// Example:
//
//	{
//	  "documentId": "doc-001",
//	  "userId": "user-bob-uuid"
//	}
type RevokeAccessEvent struct {
	DocumentID string `json:"documentId"`
	UserID     string `json:"userId"`
}
