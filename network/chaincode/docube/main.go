// Package main provides the Docube chaincode entry point.
// This chaincode implements document management and access control
// using a multi-contract architecture for Hyperledger Fabric v2.x+.
//
// Contracts:
//   - DocumentContract: Manages document NFT lifecycle
//   - AccessContract: Manages access control NFTs
//
// Features:
//   - Deterministic execution (no time.Now, no random)
//   - Version-controlled updates with optimistic locking
//   - Soft delete pattern (no physical deletion)
//   - Complete audit history via GetHistoryForKey
//   - CouchDB rich queries support
//   - Event emission for all write operations
package main

import (
	"log"

	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

func main() {
	// Create the DocumentContract with a namespace
	documentContract := new(DocumentContract)
	documentContract.Name = "document"
	documentContract.Info.Version = "1.0.0"
	documentContract.Info.Description = "Document NFT Management Contract"
	documentContract.Info.Title = "DocumentContract"

	// Create the AccessContract with a namespace
	accessContract := new(AccessContract)
	accessContract.Name = "access"
	accessContract.Info.Version = "1.0.0"
	accessContract.Info.Description = "Access Control NFT Management Contract"
	accessContract.Info.Title = "AccessContract"

	// Create chaincode with both contracts
	chaincode, err := contractapi.NewChaincode(documentContract, accessContract)
	if err != nil {
		log.Panicf("Error creating docube chaincode: %v", err)
	}

	// Set chaincode metadata
	chaincode.Info.Title = "Docube Chaincode"
	chaincode.Info.Version = "1.0.0"

	// Start the chaincode
	if err := chaincode.Start(); err != nil {
		log.Panicf("Error starting docube chaincode: %v", err)
	}
}
