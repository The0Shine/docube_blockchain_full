package service

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/horob1/docube_blockchain_service/internal/cache"
	fabricClient "github.com/horob1/docube_blockchain_service/internal/fabric/client"
)

// DocumentService handles business logic for document operations.
type DocumentService struct {
	fabric      *fabricClient.FabricClient
	accessCache *cache.AccessCache
}

func NewDocumentService(fc *fabricClient.FabricClient, ac *cache.AccessCache) *DocumentService {
	return &DocumentService{fabric: fc, accessCache: ac}
}

// DocumentResponse is the API response DTO for a document query.
type DocumentResponse struct {
	AssetID      string `json:"assetId"`
	DocumentID   string `json:"documentId"`
	DocHash      string `json:"docHash"`
	HashAlgo     string `json:"hashAlgo"`
	SystemUserId string `json:"systemUserId"`
	Version      int64  `json:"version"`
	Status       string `json:"status"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

// AccessDeniedError indicates the user does not have permission.
type AccessDeniedError struct{ Reason string }

func (e *AccessDeniedError) Error() string { return fmt.Sprintf("access denied: %s", e.Reason) }

// NotFoundError indicates the document was not found on the ledger.
type NotFoundError struct{ DocumentID string }

func (e *NotFoundError) Error() string { return fmt.Sprintf("document not found: %s", e.DocumentID) }

// GetDocumentWithAccessCheck retrieves a document after verifying that
// userId is the owner or holds an active AccessNFT.
func (s *DocumentService) GetDocumentWithAccessCheck(documentID, userID string) (*DocumentResponse, error) {
	doc, err := s.fetchDocument(documentID)
	if err != nil {
		return nil, err
	}
	if err := s.checkAccess(documentID, userID, doc.SystemUserId); err != nil {
		return nil, err
	}
	return mapToResponse(doc), nil
}

// GetDocumentHistoryWithAccessCheck retrieves the audit history of a document
// after verifying access.
func (s *DocumentService) GetDocumentHistoryWithAccessCheck(documentID, userID string) ([]*fabricClient.AuditRecord, error) {
	doc, err := s.fetchDocument(documentID)
	if err != nil {
		return nil, err
	}
	if err := s.checkAccess(documentID, userID, doc.SystemUserId); err != nil {
		return nil, err
	}
	return s.fabric.GetDocumentHistory(documentID)
}

// fetchDocument retrieves an active document from the ledger.
// Returns NotFoundError for both missing and soft-deleted documents.
func (s *DocumentService) fetchDocument(documentID string) (*fabricClient.DocumentAsset, error) {
	doc, err := s.fabric.GetDocument(documentID)
	if err != nil {
		if strings.Contains(err.Error(), "NOT_FOUND") || strings.Contains(err.Error(), "not found") {
			return nil, &NotFoundError{DocumentID: documentID}
		}
		return nil, fmt.Errorf("failed to get document: %w", err)
	}
	if doc.Status == "DELETED" {
		return nil, &NotFoundError{DocumentID: documentID}
	}
	return doc, nil
}

// checkAccess returns nil if userID is the owner or holds an active AccessNFT.
// Results are cached in Redis to avoid repeated chaincode queries.
func (s *DocumentService) checkAccess(documentID, userID, ownerID string) error {
	if userID == ownerID {
		log.Printf("[SERVICE] Access granted (OWNER) user=%s doc=%s", userID, documentID)
		return nil
	}

	ctx := context.Background()

	if cached := s.accessCache.GetAccessCheck(ctx, documentID, userID); cached != nil {
		if cached.Allowed {
			log.Printf("[SERVICE] Access granted (CACHE HIT) user=%s doc=%s", userID, documentID)
			return nil
		}
		log.Printf("[SERVICE] Access denied (CACHE HIT) user=%s doc=%s reason=%s", userID, documentID, cached.Reason)
		return &AccessDeniedError{Reason: cached.Reason}
	}

	// Cache miss — query blockchain
	accessNFT, err := s.fabric.GetAccess(documentID, userID)
	if err != nil {
		log.Printf("[SERVICE] GetAccess error for user=%s doc=%s: %v", userID, documentID, err)
	}

	allowed := err == nil && accessNFT != nil && accessNFT.Status == "ACTIVE"
	reason := "NOT_GRANTED"
	if allowed {
		reason = "GRANTED"
	}

	s.accessCache.SetAccessCheck(ctx, documentID, userID, allowed, reason)

	if !allowed {
		log.Printf("[SERVICE] Access denied user=%s doc=%s reason=%s", userID, documentID, reason)
		return &AccessDeniedError{Reason: reason}
	}

	log.Printf("[SERVICE] Access granted (GRANTEE) user=%s doc=%s", userID, documentID)
	return nil
}

func mapToResponse(doc *fabricClient.DocumentAsset) *DocumentResponse {
	return &DocumentResponse{
		AssetID:      doc.AssetID,
		DocumentID:   doc.DocumentID,
		DocHash:      doc.DocHash,
		HashAlgo:     doc.HashAlgo,
		SystemUserId: doc.SystemUserId,
		Version:      doc.Version,
		Status:       doc.Status,
		CreatedAt:    doc.CreatedAt,
		UpdatedAt:    doc.UpdatedAt,
	}
}
