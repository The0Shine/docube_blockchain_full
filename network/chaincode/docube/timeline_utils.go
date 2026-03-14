// Package main provides timeline audit log utilities for the Docube chaincode.
// Each document write operation appends a TimelineRecord to the ledger using
// composite key: DOCLOG~documentId~txId
package main

import (
	"encoding/json"
	"fmt"

	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

// =============================================================================
// TIMELINE KEY BUILDING
// =============================================================================

// BuildTimelineKey creates a composite key for timeline records.
// Format: DOCLOG~{documentId}~{txId}
// Using txId ensures uniqueness and natural ordering within a transaction.
func BuildTimelineKey(ctx contractapi.TransactionContextInterface, documentID, txID string) (string, error) {
	return ctx.GetStub().CreateCompositeKey(TimelineKeyPrefix, []string{documentID, txID})
}

// =============================================================================
// TIMELINE WRITE HELPER
// =============================================================================

// AppendTimeline writes a TimelineRecord to the ledger.
// Should be called in every write transaction that modifies document state.
func AppendTimeline(
	ctx contractapi.TransactionContextInterface,
	documentID string,
	action string,
	details map[string]string,
) error {
	caller, err := GetCallerInfo(ctx)
	if err != nil {
		return fmt.Errorf("timeline: get caller: %w", err)
	}

	timestamp, err := GetTxTimestamp(ctx)
	if err != nil {
		return fmt.Errorf("timeline: get timestamp: %w", err)
	}

	txID := GetTxID(ctx)

	record := TimelineRecord{
		DocumentID: documentID,
		TxID:       txID,
		Timestamp:  timestamp,
		Action:     action,
		ActorID:    caller.ID,
		ActorMSP:   caller.MSPID,
		Details:    details,
	}

	key, err := BuildTimelineKey(ctx, documentID, txID)
	if err != nil {
		return fmt.Errorf("timeline: build key: %w", err)
	}

	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("timeline: marshal record: %w", err)
	}

	if err := ctx.GetStub().PutState(key, data); err != nil {
		return fmt.Errorf("timeline: put state: %w", err)
	}

	return nil
}

// =============================================================================
// TIMELINE QUERY
// =============================================================================

// GetDocumentTimelineRecords retrieves all timeline records for a document
// by doing a partial composite key query on DOCLOG~documentId~.
func GetDocumentTimelineRecords(ctx contractapi.TransactionContextInterface, documentID string) ([]*TimelineRecord, error) {
	resultsIterator, err := ctx.GetStub().GetStateByPartialCompositeKey(TimelineKeyPrefix, []string{documentID})
	if err != nil {
		return nil, fmt.Errorf("timeline query: %w", err)
	}
	defer resultsIterator.Close()

	var records []*TimelineRecord
	for resultsIterator.HasNext() {
		queryResult, err := resultsIterator.Next()
		if err != nil {
			return nil, fmt.Errorf("timeline iterate: %w", err)
		}

		var record TimelineRecord
		if err := json.Unmarshal(queryResult.Value, &record); err != nil {
			return nil, fmt.Errorf("timeline unmarshal: %w", err)
		}
		records = append(records, &record)
	}

	return records, nil
}
