# FUNCTION FLOWS - Docube Chaincode

**Document Version:** 2.0  
**Last Updated:** 2026-02-01

---

## Purpose
This document provides detailed step-by-step flow explanations for each business function in the Docube chaincode, covering both DocumentContract and AccessContract.

## Scope
- All Document operations (7 functions)
- All Access operations (6 functions)
- Authorization flow
- Error handling

## References
- [CODE_ARCHITECTURE_EN.md](CODE_ARCHITECTURE_EN.md)
- [PERMISSION_MATRIX_EN.md](PERMISSION_MATRIX_EN.md)

---

# Part 1: DocumentContract Functions

## 1. CreateDocument

**Location:** `document_contract.go` (Lines 20-85)  
**Authorization:** Any user (USER, OWNER, ADMIN)

### 1.1 Sequence Diagram

```mermaid
sequenceDiagram
    participant Client
    participant Chaincode
    participant StateDB as CouchDB
    
    Client->>Chaincode: CreateDocument(documentID, hash, algo, sysUserId)
    Chaincode->>Chaincode: GetCallerInfo(ctx)
    Chaincode->>Chaincode: BuildDocumentKey(documentID)
    Chaincode->>StateDB: GetState(key)
    
    alt Document exists
        Chaincode-->>Client: Error: ERR_ALREADY_EXISTS
    else Document not exists
        Chaincode->>Chaincode: GetTxTimestamp()
        Chaincode->>Chaincode: Create DocumentAssetNFT
        Chaincode->>StateDB: PutState(key, doc)
        Chaincode->>Chaincode: EmitEvent(DocumentCreated)
        Chaincode-->>Client: Success
    end
```

### 1.2 Step-by-Step Flow

| Step | Action | Code Reference |
|------|--------|----------------|
| 1 | Extract caller identity | `GetCallerInfo(ctx)` |
| 2 | Build ledger key | `BuildDocumentKey(ctx, documentID)` |
| 3 | Check existence | `GetDocument(ctx, key)` |
| 4 | Get timestamp | `GetTxTimestamp(ctx)` |
| 5 | Create NFT struct | Set OwnerID, Version=1, Status=ACTIVE |
| 6 | Save to ledger | `SaveState(ctx, key, doc)` |
| 7 | Emit event | `EmitEvent(ctx, EventDocumentCreated, ...)` |

---

## 2. UpdateDocument

**Location:** `document_contract.go` (Lines 87-166)  
**Authorization:** ADMIN or OWNER only

### 2.1 Sequence Diagram

```mermaid
sequenceDiagram
    participant Client
    participant Chaincode
    participant Auth as Authorization
    participant StateDB as CouchDB
    
    Client->>Chaincode: UpdateDocument(docID, hash, algo, version)
    Chaincode->>Chaincode: BuildDocumentKey(docID)
    Chaincode->>StateDB: GetDocument(key)
    
    alt Not found
        Chaincode-->>Client: Error: ERR_NOT_FOUND
    end
    
    Chaincode->>Auth: AuthorizeWrite(ctx, doc, "UpdateDocument")
    
    alt Not authorized
        Chaincode-->>Client: Error: ERR_UNAUTHORIZED
    end
    
    Chaincode->>Chaincode: ValidateVersion(doc.Version, expected)
    
    alt Version mismatch
        Chaincode-->>Client: Error: ERR_VERSION_MISMATCH
    end
    
    Chaincode->>Chaincode: ValidateDocumentStatus(doc.Status)
    Chaincode->>Chaincode: Update doc fields
    Chaincode->>StateDB: PutState(key, doc)
    Chaincode->>Chaincode: EmitEvent(DocumentUpdated)
    
    opt IsAdmin
        Chaincode->>Chaincode: EmitAdminAuditEvent()
    end
    
    Chaincode-->>Client: Success
```

---

## 3. SoftDeleteDocument

**Location:** `document_contract.go` (Lines 239-308)  
**Authorization:** ADMIN or OWNER only

### 3.1 Flow Diagram

```mermaid
flowchart TD
    A[SoftDeleteDocument] --> B[Get Document]
    B --> C{Exists?}
    C -->|No| D[ERR_NOT_FOUND]
    C -->|Yes| E[AuthorizeWrite]
    E --> F{Authorized?}
    F -->|No| G[ERR_UNAUTHORIZED]
    F -->|Yes| H{Status ACTIVE?}
    H -->|No| I[ERR_INVALID_STATE]
    H -->|Yes| J[Set Status=DELETED]
    J --> K[SaveState]
    K --> L[Emit DocumentDeleted]
    L --> M{IsAdmin?}
    M -->|Yes| N[Emit AdminAuditEvent]
    M -->|No| O[Return Success]
    N --> O
```

---

## 4. TransferOwnership

**Location:** `document_contract.go` (Lines 168-237)  
**Authorization:** ADMIN or OWNER only

### 4.1 Sequence Diagram

```mermaid
sequenceDiagram
    participant Client
    participant Chaincode
    participant Auth
    participant StateDB
    
    Client->>Chaincode: TransferOwnership(docID, newOwner, newMSP)
    Chaincode->>StateDB: GetDocument(key)
    Chaincode->>Auth: AuthorizeWrite(ctx, doc, "TransferOwnership")
    
    alt Not authorized
        Chaincode-->>Client: Error: ERR_UNAUTHORIZED
    end
    
    Chaincode->>Chaincode: Validate Status
    Chaincode->>Chaincode: Update OwnerID, OwnerMSP, Version++
    Chaincode->>StateDB: PutState(key, doc)
    Chaincode->>Chaincode: EmitEvent(DocumentTransferred)
    
    opt IsAdmin
        Chaincode->>Chaincode: EmitAdminAuditEvent()
    end
    
    Chaincode-->>Client: Success
```

---

## 5-7. Query Operations

### 5. GetDocument

```mermaid
sequenceDiagram
    Client->>Chaincode: GetDocument(docID)
    Chaincode->>StateDB: GetState(key)
    
    alt Not found
        Chaincode-->>Client: Error: ERR_NOT_FOUND
    else Found
        Chaincode-->>Client: DocumentAssetNFT
    end
```

### 6. GetAllDocuments

```mermaid
sequenceDiagram
    Client->>Chaincode: GetAllDocuments()
    Chaincode->>CouchDB: GetQueryResult(selector)
    Note over CouchDB: selector: {"status": "ACTIVE"}
    CouchDB-->>Chaincode: Iterator
    Chaincode->>Chaincode: Iterate results
    Chaincode-->>Client: []DocumentAssetNFT
```

### 7. GetDocumentHistory

```mermaid
sequenceDiagram
    Client->>Chaincode: GetDocumentHistory(docID)
    Chaincode->>StateDB: GetHistoryForKey(key)
    StateDB-->>Chaincode: History Iterator
    Chaincode->>Chaincode: Parse each version
    Chaincode-->>Client: []AuditRecord
```

---

# Part 2: AccessContract Functions

## 8. GrantAccess

**Location:** `access_contract.go` (Lines 20-115)  
**Authorization:** ADMIN or Document OWNER only

### 8.1 Sequence Diagram

```mermaid
sequenceDiagram
    participant Client
    participant Chaincode
    participant Auth
    participant StateDB
    
    Client->>Chaincode: GrantAccess(docID, granteeID, granteeMSP, sysUserId)
    
    Note over Chaincode: Step 1: Get Caller Info
    Chaincode->>Chaincode: GetCallerInfo(ctx)
    
    Note over Chaincode: Step 2: Verify Document Exists
    Chaincode->>Chaincode: BuildDocumentKey(docID)
    Chaincode->>StateDB: GetDocument(docKey)
    
    alt Document not found
        Chaincode-->>Client: Error: ERR_NOT_FOUND
    end
    
    Note over Chaincode: Step 3: Validate Document Status
    Chaincode->>Chaincode: ValidateDocumentStatus(doc.Status)
    
    alt Document deleted
        Chaincode-->>Client: Error: ERR_INVALID_STATE
    end
    
    Note over Chaincode: Step 4: Authorization Check
    Chaincode->>Auth: AuthorizeWrite(ctx, doc, "GrantAccess")
    
    alt Not authorized (not doc owner or admin)
        Chaincode-->>Client: Error: ERR_UNAUTHORIZED
    end
    
    Note over Chaincode: Step 5: Check Existing Access
    Chaincode->>Chaincode: BuildAccessKey(docID, granteeID)
    Chaincode->>StateDB: GetAccessNFT(accessKey)
    
    alt Access already exists (ACTIVE)
        Chaincode-->>Client: Error: ERR_ALREADY_EXISTS
    end
    
    Note over Chaincode: Step 6: Create AccessNFT
    Chaincode->>Chaincode: Create AccessNFT struct
    Note right of Chaincode: AccessNFTID: ACC-{docID}-{granteeID}<br/>Status: ACTIVE<br/>GrantedBy: caller.ID<br/>GrantedAt: timestamp
    
    Chaincode->>StateDB: PutState(accessKey, accessNFT)
    
    Note over Chaincode: Step 7: Admin Audit (if admin)
    opt authResult.IsAdmin
        Chaincode->>Chaincode: EmitAdminAuditEvent("GrantAccess")
    end
    
    Note over Chaincode: Step 8: Emit Event
    Chaincode->>Chaincode: EmitEvent(AccessGranted)
    Chaincode-->>Client: Success
```

### 8.2 Step-by-Step Flow

| Step | Action | Code Line | Notes |
|------|--------|-----------|-------|
| 1 | Get caller identity | 32-35 | Uses GetCallerInfo |
| 2 | Build document key | 38-41 | DOC~{documentId} |
| 3 | Get document | 43-49 | Must exist |
| 4 | Validate doc status | 52-54 | Must be ACTIVE |
| 5 | **Authorization** | 57-60 | **Checks DOC owner, not access owner** |
| 6 | Build access key | 63-66 | ACC~{docId}~{userId} |
| 7 | Check duplicate | 69-76 | Cannot grant twice |
| 8 | Get timestamp | 79-82 | Deterministic |
| 9 | Create AccessNFT | 85-94 | Set all fields |
| 10 | Save to ledger | 97-99 | PutState |
| 11 | Admin audit | 102-106 | If admin |
| 12 | Emit event | 109-114 | AccessGranted |

### 8.3 AccessNFT Created

```go
accessNFT := AccessNFT{
    AccessNFTID:  "ACC-{docID}-{granteeID}",
    DocumentID:   documentID,
    OwnerID:      granteeUserID,      // Who receives access
    OwnerMSP:     granteeUserMSP,
    SystemUserId: systemUserId,
    Status:       "ACTIVE",
    GrantedBy:    caller.ID,          // Who grants (doc owner or admin)
    GrantedAt:    timestamp,
}
```

---

## 9. RevokeAccess

**Location:** `access_contract.go` (Lines 117-203)  
**Authorization:** ADMIN or Document OWNER only

### 9.1 Sequence Diagram

```mermaid
sequenceDiagram
    participant Client
    participant Chaincode
    participant Auth
    participant StateDB
    
    Client->>Chaincode: RevokeAccess(docID, userID)
    
    Note over Chaincode: Step 1: Get Caller Info
    Chaincode->>Chaincode: GetCallerInfo(ctx)
    
    Note over Chaincode: Step 2: Verify Document Exists
    Chaincode->>StateDB: GetDocument(docKey)
    
    alt Document not found
        Chaincode-->>Client: Error: ERR_NOT_FOUND
    end
    
    Note over Chaincode: Step 3: Authorization Check
    Chaincode->>Auth: AuthorizeWrite(ctx, doc, "RevokeAccess")
    
    alt Not authorized (not doc owner or admin)
        Chaincode-->>Client: Error: ERR_UNAUTHORIZED
    end
    
    Note over Chaincode: Step 4: Get Existing Access
    Chaincode->>Chaincode: BuildAccessKey(docID, userID)
    Chaincode->>StateDB: GetAccessNFT(accessKey)
    
    alt Access not found
        Chaincode-->>Client: Error: ERR_NOT_FOUND
    end
    
    Note over Chaincode: Step 5: Validate Access Status
    Chaincode->>Chaincode: ValidateAccessStatus(access.Status)
    
    alt Already revoked
        Chaincode-->>Client: Error: ERR_INVALID_STATE
    end
    
    Note over Chaincode: Step 6: Update AccessNFT
    Chaincode->>Chaincode: Set Status=REVOKED, RevokedBy, RevokedAt
    Chaincode->>StateDB: PutState(accessKey, access)
    
    Note over Chaincode: Step 7: Admin Audit (if admin)
    opt authResult.IsAdmin
        Chaincode->>Chaincode: EmitAdminAuditEvent("RevokeAccess")
    end
    
    Note over Chaincode: Step 8: Emit Event
    Chaincode->>Chaincode: EmitEvent(AccessRevoked)
    Chaincode-->>Client: Success
```

### 9.2 Step-by-Step Flow

| Step | Action | Code Line | Notes |
|------|--------|-----------|-------|
| 1 | Get caller identity | 127-130 | Uses GetCallerInfo |
| 2 | Build document key | 133-136 | DOC~{documentId} |
| 3 | Get document | 138-144 | Must exist |
| 4 | **Authorization** | 147-150 | **Checks DOC owner, not access owner** |
| 5 | Build access key | 153-156 | ACC~{docId}~{userId} |
| 6 | Get access | 159-166 | Must exist |
| 7 | Validate status | 169-171 | Must be ACTIVE |
| 8 | Get timestamp | 174-177 | Deterministic |
| 9 | Update AccessNFT | 180-182 | Set REVOKED, RevokedBy, RevokedAt |
| 10 | Save to ledger | 185-187 | PutState |
| 11 | Admin audit | 190-194 | If admin |
| 12 | Emit event | 197-202 | AccessRevoked |

### 9.3 AccessNFT Updated

```go
// Before revoke
access.Status = "ACTIVE"
access.RevokedBy = ""
access.RevokedAt = ""

// After revoke
access.Status = "REVOKED"
access.RevokedBy = caller.ID      // Who revokes
access.RevokedAt = timestamp      // When revoked
```

---

## 10. GetAccess

**Location:** `access_contract.go` (Lines 209-232)  
**Authorization:** Any user

### 10.1 Sequence Diagram

```mermaid
sequenceDiagram
    participant Client
    participant Chaincode
    participant StateDB
    
    Client->>Chaincode: GetAccess(docID, userID)
    Chaincode->>Chaincode: BuildAccessKey(docID, userID)
    Chaincode->>StateDB: GetState(accessKey)
    
    alt Not found
        Chaincode-->>Client: Error: ERR_NOT_FOUND
    else Found
        Chaincode-->>Client: AccessNFT
    end
```

---

## 11. GetAllAccessByDocument

**Location:** `access_contract.go` (Lines 234-268)  
**Authorization:** Any user

### 11.1 Sequence Diagram

```mermaid
sequenceDiagram
    participant Client
    participant Chaincode
    participant CouchDB
    
    Client->>Chaincode: GetAllAccessByDocument(docID)
    
    Note over Chaincode: Build CouchDB Query
    Chaincode->>CouchDB: GetQueryResult
    Note over CouchDB: {"selector":{"documentId":"docID"}}
    
    CouchDB-->>Chaincode: Results Iterator
    
    loop For each result
        Chaincode->>Chaincode: Unmarshal to AccessNFT
        Chaincode->>Chaincode: Append to list
    end
    
    Chaincode-->>Client: []AccessNFT (all statuses)
```

### 11.2 Query Details

```go
queryString := fmt.Sprintf(`{
    "selector": {
        "documentId": "%s"
    }
}`, documentID)
```

**Note:** Returns ALL access records (both ACTIVE and REVOKED).

---

## 12. GetAllAccessByUser

**Location:** `access_contract.go` (Lines 270-305)  
**Authorization:** Any user

### 12.1 Sequence Diagram

```mermaid
sequenceDiagram
    participant Client
    participant Chaincode
    participant CouchDB
    
    Client->>Chaincode: GetAllAccessByUser(userID)
    
    Note over Chaincode: Build CouchDB Query
    Chaincode->>CouchDB: GetQueryResult
    Note over CouchDB: {"selector":{"ownerId":"userID","status":"ACTIVE"}}
    
    CouchDB-->>Chaincode: Results Iterator
    
    loop For each result
        Chaincode->>Chaincode: Unmarshal to AccessNFT
        Chaincode->>Chaincode: Append to list
    end
    
    Chaincode-->>Client: []AccessNFT (ACTIVE only)
```

### 12.2 Query Details

```go
queryString := fmt.Sprintf(`{
    "selector": {
        "ownerId": "%s",
        "status": "ACTIVE"
    }
}`, userID)
```

**Note:** Returns only ACTIVE access records (excludes REVOKED).

---

## 13. GetAccessHistory

**Location:** `access_contract.go` (Lines 307-353)  
**Authorization:** Any user

### 13.1 Sequence Diagram

```mermaid
sequenceDiagram
    participant Client
    participant Chaincode
    participant StateDB
    
    Client->>Chaincode: GetAccessHistory(docID, userID)
    Chaincode->>Chaincode: BuildAccessKey(docID, userID)
    Chaincode->>StateDB: GetHistoryForKey(accessKey)
    StateDB-->>Chaincode: History Iterator
    
    loop For each modification
        Chaincode->>Chaincode: Parse modification
        Note right of Chaincode: TxID, Timestamp, Value, IsDelete
        Chaincode->>Chaincode: Append to history
    end
    
    Chaincode-->>Client: []AuditRecord
```

---

# Part 3: Authorization Flow

## 14. AuthorizeWrite Function

```mermaid
flowchart TD
    A[AuthorizeWrite] --> B[GetCallerInfo]
    B --> C[IsAdmin?]
    C -->|Yes| D[Return: Allowed=true, Role=ADMIN]
    C -->|No| E{doc == nil?}
    E -->|Yes| F[Return: Allowed=true, Role=USER]
    E -->|No| G[IsDocumentOwner?]
    G -->|Yes| H[Return: Allowed=true, Role=OWNER]
    G -->|No| I[Return: Error ERR_UNAUTHORIZED]
```

## 15. Key Point: Access Authorization

> **Important:** For GrantAccess and RevokeAccess, the authorization checks the **Document Owner**, not the Access holder. This means:
> - Only the document owner can grant access to their document
> - Only the document owner can revoke access from their document
> - Admin can override both operations

---

# Part 4: Error Summary

| Function | Possible Errors |
|----------|-----------------|
| CreateDocument | `ERR_ALREADY_EXISTS` |
| UpdateDocument | `ERR_NOT_FOUND`, `ERR_UNAUTHORIZED`, `ERR_VERSION_MISMATCH`, `ERR_INVALID_STATE` |
| SoftDeleteDocument | `ERR_NOT_FOUND`, `ERR_UNAUTHORIZED`, `ERR_INVALID_STATE` |
| TransferOwnership | `ERR_NOT_FOUND`, `ERR_UNAUTHORIZED`, `ERR_INVALID_STATE` |
| **GrantAccess** | `ERR_NOT_FOUND` (doc), `ERR_UNAUTHORIZED`, `ERR_INVALID_STATE` (doc), `ERR_ALREADY_EXISTS` |
| **RevokeAccess** | `ERR_NOT_FOUND` (doc or access), `ERR_UNAUTHORIZED`, `ERR_INVALID_STATE` |
| GetAccess | `ERR_NOT_FOUND` |
| GetAllAccessByDocument | None |
| GetAllAccessByUser | None |
| GetAccessHistory | None |

---

## Document History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-02-01 | Docube Team | Initial document |
| 2.0 | 2026-02-01 | Docube Team | Added full AccessContract flows |
