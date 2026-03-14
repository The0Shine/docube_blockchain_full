package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strconv"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/plain"

	"github.com/horob1/docube_blockchain_service/internal/cache"
	fabricClient "github.com/horob1/docube_blockchain_service/internal/fabric/client"
)

// Consumer listens on Kafka topics and submits chaincode transactions.
type Consumer struct {
	cfg         KafkaConfig
	fabric      *fabricClient.FabricClient
	accessCache *cache.AccessCache
}

// NewConsumer creates a new Kafka Consumer.
func NewConsumer(cfg KafkaConfig, fc *fabricClient.FabricClient, ac *cache.AccessCache) *Consumer {
	return &Consumer{cfg: cfg, fabric: fc, accessCache: ac}
}

// Start launches one goroutine per topic. Call with `go consumer.Start(ctx)`.
// Blocks until context is cancelled.
func (c *Consumer) Start(ctx context.Context) {
	log.Println("[KAFKA] 🚀 Starting Kafka consumers...")

	topics := []string{
		TopicDocumentCreate,
		TopicDocumentUpdate,
		TopicAccessGrant,
		TopicAccessRevoke,
	}

	// Ensure all topics exist before consuming
	for _, topic := range topics {
		c.ensureTopic(topic)
	}

	for _, topic := range topics {
		go c.consumeTopic(ctx, topic)
	}

	// Block until context cancelled
	<-ctx.Done()
	log.Println("[KAFKA] 🛑 Consumer shutting down")
}

// ensureTopic creates the Kafka topic if it does not exist.
func (c *Consumer) ensureTopic(topic string) {
	dialer := c.buildDialer()

	conn, err := dialer.Dial("tcp", c.cfg.Brokers[0])
	if err != nil {
		log.Printf("[KAFKA] ⚠️  Cannot connect to create topic %s: %v", topic, err)
		return
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		log.Printf("[KAFKA] ⚠️  Cannot get controller for topic %s: %v", topic, err)
		return
	}

	controllerConn, err := dialer.Dial("tcp", net.JoinHostPort(controller.Host, strconv.Itoa(controller.Port)))
	if err != nil {
		log.Printf("[KAFKA] ⚠️  Cannot connect to controller for topic %s: %v", topic, err)
		return
	}
	defer controllerConn.Close()

	err = controllerConn.CreateTopics(kafka.TopicConfig{
		Topic:             topic,
		NumPartitions:     1,
		ReplicationFactor: 1,
	})
	if err != nil {
		// Ignore "topic already exists" errors
		log.Printf("[KAFKA] ℹ️  Topic %s: %v", topic, err)
	} else {
		log.Printf("[KAFKA] ✅ Topic created: %s", topic)
	}
}

// consumeTopic reads messages from a single topic in a loop.
func (c *Consumer) consumeTopic(ctx context.Context, topic string) {
	dialer := c.buildDialer()

	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        c.cfg.Brokers,
		Topic:          topic,
		GroupID:        c.cfg.GroupID,
		Dialer:         dialer,
		MinBytes:       1,
		MaxBytes:       10e6, // 10 MB
		CommitInterval: time.Second,
		StartOffset:    kafka.FirstOffset,
	})
	defer r.Close()

	log.Printf("[KAFKA] 📥 Listening on topic: %s (group: %s)", topic, c.cfg.GroupID)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msg, err := r.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return // Context cancelled, clean exit
			}
			log.Printf("[KAFKA] ❌ Error reading from %s: %v — retrying in 3s", topic, err)
			time.Sleep(3 * time.Second)
			continue
		}

		log.Printf("[KAFKA] 📨 Received msg from topic=%s partition=%d offset=%d",
			topic, msg.Partition, msg.Offset)

		// Dispatch message to the right handler
		var handlerErr error
		switch topic {
		case TopicDocumentCreate:
			handlerErr = c.handleCreateDocument(msg.Value)
		case TopicDocumentUpdate:
			handlerErr = c.handleUpdateDocument(msg.Value)
		case TopicAccessGrant:
			handlerErr = c.handleGrantAccess(msg.Value)
		case TopicAccessRevoke:
			handlerErr = c.handleRevokeAccess(msg.Value)
		default:
			log.Printf("[KAFKA] ⚠️  Unknown topic: %s", topic)
		}

		if handlerErr != nil {
			// Log the error but still commit — dead-letter queue can be added later
			log.Printf("[KAFKA] ❌ Handler error on topic=%s: %v", topic, handlerErr)
		} else {
			log.Printf("[KAFKA] ✅ Message processed successfully on topic=%s", topic)
		}

		// Commit offset after processing
		if err := r.CommitMessages(ctx, msg); err != nil {
			log.Printf("[KAFKA] ⚠️  Failed to commit offset: %v", err)
		}
	}
}

// =============================================================================
// KAFKA CONSUMER HANDLER FUNCTIONS
// These are the actual "consumer" functions that process Kafka messages
// and submit transactions to Hyperledger Fabric.
// =============================================================================

// handleCreateDocument consumes a message from docube.document.create
// and calls fabric.CreateDocument({documentId, docHash, hashAlgo, systemUserId}).
//
// Expected payload:
//
//	{ "documentId": "...", "docHash": "...", "hashAlgo": "SHA256", "systemUserId": "..." }
func (c *Consumer) handleCreateDocument(data []byte) error {
	var event CreateDocumentEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return fmt.Errorf("unmarshal CreateDocumentEvent: %w", err)
	}

	log.Printf("[KAFKA][CONSUMER] handleCreateDocument: doc=%s user=%s",
		event.DocumentID, event.SystemUserId)

	return c.fabric.CreateDocument(
		event.DocumentID,
		event.DocHash,
		event.HashAlgo,
		event.SystemUserId,
	)
}

// handleUpdateDocument consumes a message from docube.document.update
// and calls fabric.UpdateDocument({documentId, newDocHash, newHashAlgo, expectedVersion}).
//
// This is the "private → public" operation: update the doc hash + metadata.
//
// Expected payload:
//
//	{ "documentId": "...", "newDocHash": "...", "newHashAlgo": "SHA256", "expectedVersion": 1 }
func (c *Consumer) handleUpdateDocument(data []byte) error {
	var event UpdateDocumentEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return fmt.Errorf("unmarshal UpdateDocumentEvent: %w", err)
	}

	log.Printf("[KAFKA][CONSUMER] handleUpdateDocument: doc=%s version=%d",
		event.DocumentID, event.ExpectedVersion)

	return c.fabric.UpdateDocument(
		event.DocumentID,
		event.NewDocHash,
		event.NewHashAlgo,
		event.ExpectedVersion,
	)
}

// handleGrantAccess consumes a message from docube.access.grant
// and calls fabric.GrantAccess({documentId, granteeUserId, granteeUserMsp, systemUserId}).
//
// Expected payload:
//
//	{ "documentId": "...", "granteeUserId": "...", "granteeUserMsp": "AdminOrgMSP", "systemUserId": "..." }
func (c *Consumer) handleGrantAccess(data []byte) error {
	var event GrantAccessEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return fmt.Errorf("unmarshal GrantAccessEvent: %w", err)
	}

	log.Printf("[KAFKA][CONSUMER] handleGrantAccess: doc=%s grantee=%s",
		event.DocumentID, event.GranteeUserID)

	err := c.fabric.GrantAccess(
		event.DocumentID,
		event.GranteeUserID,
		event.GranteeUserMSP,
		event.SystemUserId,
	)
	if err != nil {
		return err
	}

	// Invalidate access cache after successful grant
	c.accessCache.InvalidateAccess(context.Background(), event.DocumentID, event.GranteeUserID)
	return nil
}

// handleRevokeAccess consumes a message from docube.access.revoke
// and calls fabric.RevokeAccess({documentId, userId}).
//
// Expected payload:
//
//	{ "documentId": "...", "userId": "..." }
func (c *Consumer) handleRevokeAccess(data []byte) error {
	var event RevokeAccessEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return fmt.Errorf("unmarshal RevokeAccessEvent: %w", err)
	}

	log.Printf("[KAFKA][CONSUMER] handleRevokeAccess: doc=%s user=%s",
		event.DocumentID, event.UserID)

	err := c.fabric.RevokeAccess(event.DocumentID, event.UserID)
	if err != nil {
		return err
	}

	// Invalidate access cache after successful revoke
	c.accessCache.InvalidateAccess(context.Background(), event.DocumentID, event.UserID)
	return nil
}

// =============================================================================
// HELPERS
// =============================================================================

// buildDialer creates a Kafka dialer with optional SASL authentication.
func (c *Consumer) buildDialer() *kafka.Dialer {
	dialer := &kafka.Dialer{
		Timeout:   10 * time.Second,
		DualStack: true,
	}

	if c.cfg.SASLEnabled {
		mechanism := plain.Mechanism{
			Username: c.cfg.SASLUsername,
			Password: c.cfg.SASLPassword,
		}
		dialer.SASLMechanism = mechanism
		log.Printf("[KAFKA] SASL/PLAIN enabled for user: %s", c.cfg.SASLUsername)
	}

	return dialer
}
