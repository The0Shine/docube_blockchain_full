package http

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/horob1/docube_blockchain_service/internal/service"
)

// Handler handles HTTP requests for the blockchain service.
type Handler struct {
	documentService *service.DocumentService
}

// NewHandler creates a new Handler with the given DocumentService.
func NewHandler(ds *service.DocumentService) *Handler {
	return &Handler{documentService: ds}
}

// APIResponse is the standard JSON response envelope.
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *APIError   `json:"error,omitempty"`
}

// APIError holds error details for the response.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// RegisterRoutes registers all HTTP routes on the given ServeMux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/blockchain/documents/{id}", h.GetDocument)
	mux.HandleFunc("GET /api/v1/blockchain/documents/{id}/history", h.GetDocumentHistory)
	mux.HandleFunc("GET /api/v1/blockchain/health", h.HealthCheck)
}

// HealthCheck returns service health status.
func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    map[string]string{"status": "UP"},
	})
}

// GetDocument handles GET /api/v1/blockchain/documents/{id}
// It extracts X-User-Id from the header and delegates to the service layer.
func (h *Handler) GetDocument(w http.ResponseWriter, r *http.Request) {
	// --- 1. Extract document ID from URL path ---
	documentID := r.PathValue("id")
	if documentID == "" {
		writeJSON(w, http.StatusBadRequest, APIResponse{
			Success: false,
			Error: &APIError{
				Code:    "INVALID_REQUEST",
				Message: "Document ID is required",
			},
		})
		return
	}

	// --- 2. Extract X-User-Id from header (injected by Gateway) ---
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, APIResponse{
			Success: false,
			Error: &APIError{
				Code:    "UNAUTHORIZED",
				Message: "Missing X-User-Id header",
			},
		})
		return
	}

	// Trim whitespace
	userID = strings.TrimSpace(userID)
	documentID = strings.TrimSpace(documentID)

	log.Printf("[HANDLER] GET /documents/%s by user=%s", documentID, userID)

	// --- 3. Delegate to service layer ---
	doc, err := h.documentService.GetDocumentWithAccessCheck(documentID, userID)
	if err != nil {
		var accessErr *service.AccessDeniedError
		var notFoundErr *service.NotFoundError

		switch {
		case errors.As(err, &accessErr):
			writeJSON(w, http.StatusForbidden, APIResponse{
				Success: false,
				Error: &APIError{
					Code:    "ACCESS_DENIED",
					Message: accessErr.Error(),
				},
			})
		case errors.As(err, &notFoundErr):
			writeJSON(w, http.StatusNotFound, APIResponse{
				Success: false,
				Error: &APIError{
					Code:    "NOT_FOUND",
					Message: notFoundErr.Error(),
				},
			})
		default:
			log.Printf("[HANDLER] ❌ Internal error: %v", err)
			writeJSON(w, http.StatusInternalServerError, APIResponse{
				Success: false,
				Error: &APIError{
					Code:    "INTERNAL_ERROR",
					Message: "An unexpected error occurred",
				},
			})
		}
		return
	}

	// --- 4. Return success response ---
	writeJSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    doc,
	})
}

// GetDocumentHistory handles GET /api/v1/blockchain/documents/{id}/history
func (h *Handler) GetDocumentHistory(w http.ResponseWriter, r *http.Request) {
	// --- 1. Extract path and header ---
	documentID := r.PathValue("id")
	userID := r.Header.Get("X-User-Id")

	if documentID == "" || userID == "" {
		writeJSON(w, http.StatusBadRequest, APIResponse{
			Success: false,
			Error: &APIError{
				Code:    "INVALID_REQUEST",
				Message: "Document ID and X-User-Id are required",
			},
		})
		return
	}

	userID = strings.TrimSpace(userID)
	documentID = strings.TrimSpace(documentID)

	log.Printf("[HANDLER] GET /documents/%s/history by user=%s", documentID, userID)

	// --- 2. Delegate to service ---
	history, err := h.documentService.GetDocumentHistoryWithAccessCheck(documentID, userID)
	if err != nil {
		var accessErr *service.AccessDeniedError
		var notFoundErr *service.NotFoundError

		switch {
		case errors.As(err, &accessErr):
			writeJSON(w, http.StatusForbidden, APIResponse{
				Success: false,
				Error: &APIError{
					Code:    "ACCESS_DENIED",
					Message: accessErr.Error(),
				},
			})
		case errors.As(err, &notFoundErr):
			writeJSON(w, http.StatusNotFound, APIResponse{
				Success: false,
				Error: &APIError{
					Code:    "NOT_FOUND",
					Message: notFoundErr.Error(),
				},
			})
		default:
			log.Printf("[HANDLER] ❌ Internal error: %v", err)
			writeJSON(w, http.StatusInternalServerError, APIResponse{
				Success: false,
				Error: &APIError{
					Code:    "INTERNAL_ERROR",
					Message: "An unexpected error occurred",
				},
			})
		}
		return
	}

	// --- 3. Return response ---
	writeJSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    history,
	})
}

// writeJSON serializes the response as JSON and writes it to the ResponseWriter.
func writeJSON(w http.ResponseWriter, statusCode int, response APIResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}
