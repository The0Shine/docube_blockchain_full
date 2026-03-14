// Package main provides identity utilities for the Docube chaincode.
package main

import (
	"fmt"

	"github.com/hyperledger/fabric-chaincode-go/v2/pkg/cid"
	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

// CallerInfo contains the identity information of the transaction caller.
type CallerInfo struct {
	ID    string // User ID extracted from x509 certificate
	MSPID string // MSP ID of the caller's organization
}

// GetCallerInfo extracts and returns the caller's identity information.
// It uses the Client Identity Library (cid) to get data from the x509 certificate.
func GetCallerInfo(ctx contractapi.TransactionContextInterface) (*CallerInfo, error) {
	// Get the client identity from context
	clientID, err := cid.GetID(ctx.GetStub())
	if err != nil {
		return nil, fmt.Errorf("failed to get client ID: %w", err)
	}

	// Get the MSP ID
	mspID, err := cid.GetMSPID(ctx.GetStub())
	if err != nil {
		return nil, fmt.Errorf("failed to get MSP ID: %w", err)
	}

	return &CallerInfo{
		ID:    clientID,
		MSPID: mspID,
	}, nil
}

// GetClientID returns only the client ID (user identity) from the x509 certificate.
func GetClientID(ctx contractapi.TransactionContextInterface) (string, error) {
	clientID, err := cid.GetID(ctx.GetStub())
	if err != nil {
		return "", fmt.Errorf("failed to get client ID: %w", err)
	}
	return clientID, nil
}

// GetMSPID returns only the MSP ID of the caller's organization.
func GetMSPID(ctx contractapi.TransactionContextInterface) (string, error) {
	mspID, err := cid.GetMSPID(ctx.GetStub())
	if err != nil {
		return "", fmt.Errorf("failed to get MSP ID: %w", err)
	}
	return mspID, nil
}

// IsOwner checks if the caller is the owner of the given asset.
// Returns true if caller's ID and MSP match the owner's ID and MSP.
func IsOwner(caller *CallerInfo, ownerID, ownerMSP string) bool {
	return caller.ID == ownerID && caller.MSPID == ownerMSP
}
