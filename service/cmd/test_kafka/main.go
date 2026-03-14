package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/plain"
)

// Các model tương ứng với models.go trong service
type CreateDocumentEvent struct {
	DocumentID   string `json:"documentId"`
	DocHash      string `json:"docHash"`
	HashAlgo     string `json:"hashAlgo"`
	SystemUserId string `json:"systemUserId"`
}

type UpdateDocumentEvent struct {
	DocumentID      string `json:"documentId"`
	NewDocHash      string `json:"newDocHash"`
	NewHashAlgo     string `json:"newHashAlgo"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

type GrantAccessEvent struct {
	DocumentID     string `json:"documentId"`
	GranteeUserID  string `json:"granteeUserId"`
	GranteeUserMSP string `json:"granteeUserMsp"`
	SystemUserId   string `json:"systemUserId"`
}

type RevokeAccessEvent struct {
	DocumentID string `json:"documentId"`
	UserID     string `json:"userId"`
}

func main() {
	fmt.Println("==================================================")
	fmt.Println("🚀 KAFKA CONSUMER TEST SCRIPT - Tự Động Publish")
	fmt.Println("==================================================")

	// Kafka config
	broker := "localhost:7092"
	mechanism := plain.Mechanism{
		Username: "horob1",
		Password: "2410",
	}
	sharedTransport := &kafka.Transport{
		SASL: mechanism,
	}

	// Dùng AutoTopicCreation trong lúc Write với Retry mechanism
	fmt.Println("⏳ Đang chuẩn bị publish message (tự động tạo topic nếu chưa có)...")
	time.Sleep(1 * time.Second)

	// Dữ liệu test giả lập
	// Lưu ý: User ID "alice-uuid" và "bob-uuid" không cần phải tạo thực sự trong DB Auth-Service
	// vì Fabric chaincode chỉ lưu chuỗi này như là ID của owner/grantee. Tuy nhiên khi gọi qua Gateway
	// thì CẦN token hợp lệ (nếu bạn muốn verify bằng GET request). Ở đây ta chỉ bắn Kafka để test consumer.
	docID := fmt.Sprintf("test-doc-%d", time.Now().Unix())
	aliceID := "alice-uuid-123"
	bobID := "bob-uuid-456"

	// ---------------------------------------------------------
	// Kịch bản 1: Tạo Document (Topic: docube.document.create)
	// ---------------------------------------------------------
	createEvt := CreateDocumentEvent{
		DocumentID:   docID,
		DocHash:      "hash-lan-đầu",
		HashAlgo:     "SHA256",
		SystemUserId: aliceID,
	}
	fmt.Printf("\n[1] Đang publish lệnh CREATE DOCUMENT: %s...\n", docID)
	publishMsg(broker, sharedTransport, "docube.document.create", createEvt)
	
	fmt.Println("⏳ Đợi 3 giây để blockchain service xử lý...")
	time.Sleep(3 * time.Second)

	// ---------------------------------------------------------
	// Kịch bản 2: Cấp quyền cho Bob (Topic: docube.access.grant)
	// ---------------------------------------------------------
	grantEvt := GrantAccessEvent{
		DocumentID:     docID,
		GranteeUserID:  bobID,
		GranteeUserMSP: "AdminOrgMSP",
		SystemUserId:   aliceID,
	}
	fmt.Printf("\n[2] Đang publish lệnh GRANT ACCESS cho Bob (%s)...\n", bobID)
	publishMsg(broker, sharedTransport, "docube.access.grant", grantEvt)

	fmt.Println("⏳ Đợi 3 giây để blockchain service xử lý...")
	time.Sleep(3 * time.Second)

	// ---------------------------------------------------------
	// Kịch bản 3: Update Document (Private -> Public) (Topic: docube.document.update)
	// ---------------------------------------------------------
	updateEvt := UpdateDocumentEvent{
		DocumentID:      docID,
		NewDocHash:      "hash-mới-sau-khi-public",
		NewHashAlgo:     "SHA256",
		ExpectedVersion: 1, // Lần update đầu tiên version là 1 (biến thành 2)
	}
	fmt.Printf("\n[3] Đang publish lệnh UPDATE DOCUMENT bản public...\n")
	publishMsg(broker, sharedTransport, "docube.document.update", updateEvt)

	fmt.Println("⏳ Đợi 3 giây để blockchain service xử lý...")
	time.Sleep(3 * time.Second)

	// ---------------------------------------------------------
	// Kịch bản 4: Thu hồi quyền của Bob (Topic: docube.access.revoke)
	// ---------------------------------------------------------
	revokeEvt := RevokeAccessEvent{
		DocumentID: docID,
		UserID:     bobID,
	}
	fmt.Printf("\n[4] Đang publish lệnh REVOKE ACCESS của Bob...\n")
	publishMsg(broker, sharedTransport, "docube.access.revoke", revokeEvt)

	fmt.Println("\n✅ ĐÃ HOÀN TẤT BẮN TEST KAFKA!")
	fmt.Println("Hãy kiểm tra log của docube_blockchain_service để xem kết quả consumer.")
}

func publishMsg(broker string, transport *kafka.Transport, topic string, payload interface{}) {
	writer := &kafka.Writer{
		Addr:                   kafka.TCP(broker),
		Topic:                  topic,
		Balancer:               &kafka.LeastBytes{},
		Transport:              transport,
		AllowAutoTopicCreation: true,
	}
	defer writer.Close()

	valBytes, _ := json.Marshal(payload)

	var err error
	maxRetries := 5
	for i := 1; i <= maxRetries; i++ {
		err = writer.WriteMessages(context.Background(), kafka.Message{
			Value: valBytes,
		})
		
		if err == nil {
			fmt.Printf("   -> Gửi thành công! Data: %s\n", string(valBytes))
			return // Thành công thì thoát
		}

		log.Printf("⚠️ Lỗi gửi Message (Lần %d/%d) lên topic %s: %v. Đang thử lại sau 2s...", i, maxRetries, topic, err)
		time.Sleep(2 * time.Second)
	}

	log.Fatalf("❌ Thất bại hoàn toàn sau %d lần thử publish tới topic %s: %v", maxRetries, topic, err)
}
