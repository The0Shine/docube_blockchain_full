package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/horob1/docube_blockchain_service/internal/cache"
	"github.com/horob1/docube_blockchain_service/internal/config"
	"github.com/horob1/docube_blockchain_service/internal/eureka"
	fabricClient "github.com/horob1/docube_blockchain_service/internal/fabric/client"
	"github.com/horob1/docube_blockchain_service/internal/kafka"
	"github.com/horob1/docube_blockchain_service/internal/service"
	httpTransport "github.com/horob1/docube_blockchain_service/internal/transport/http"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Println("========================================")
	log.Println("🚀 Docube Blockchain Service Starting...")
	log.Println("========================================")

	// -------------------------------------------
	// 1. Load Configuration
	// -------------------------------------------
	cfg, err := config.Load("config/app.yaml")
	if err != nil {
		log.Fatalf("❌ Failed to load configuration: %v", err)
	}

	fabricCfg, err := config.LoadFabric("config/fabric.yaml")
	if err != nil {
		log.Fatalf("❌ Failed to load Fabric configuration: %v", err)
	}

	log.Printf("📋 Configuration loaded:")
	log.Printf("   App Name: %s", cfg.App.Name)
	log.Printf("   App Port: %d", cfg.App.Port)
	log.Printf("   Environment: %s", cfg.App.Env)
	log.Printf("   Eureka Server: %s", cfg.Eureka.ServerURL)
	log.Printf("   Fabric Channel: %s", fabricCfg.Fabric.ChannelName)
	log.Printf("   Fabric Chaincode: %s", fabricCfg.Fabric.ChaincodeName)
	log.Printf("   Kafka Brokers: %v", cfg.Kafka.Brokers)
	log.Printf("   Redis: %s (db=%d, ttl=%ds)", cfg.Redis.Addr, cfg.Redis.DB, cfg.Redis.TTL)

	// -------------------------------------------
	// 2. Initialize Fabric Client
	// -------------------------------------------
	log.Println("🔗 Connecting to Fabric network...")
	fc, err := fabricClient.New(fabricClient.Config{
		ChannelName:   fabricCfg.Fabric.ChannelName,
		ChaincodeName: fabricCfg.Fabric.ChaincodeName,
		MspID:         fabricCfg.Fabric.MspID,
		PeerEndpoint:  fabricCfg.Fabric.PeerEndpoint,
		GatewayPeer:   fabricCfg.Fabric.GatewayPeer,
		CryptoPath:    fabricCfg.Fabric.CryptoPath,
		CertPath:      fabricCfg.Fabric.CertPath,
		KeyDir:        fabricCfg.Fabric.KeyDir,
		TLSCertPath:   fabricCfg.Fabric.TLSCertPath,
	})
	if err != nil {
		log.Fatalf("❌ Failed to connect to Fabric network: %v", err)
	}
	defer fc.Close()

	// -------------------------------------------
	// 3. Initialize Redis Cache
	// -------------------------------------------
	log.Println("🔴 Connecting to Redis...")
	redisClient, err := cache.NewRedisClient(cache.RedisConfig{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
		TTL:      cfg.Redis.TTL,
	})
	if err != nil {
		log.Fatalf("❌ Failed to connect to Redis: %v", err)
	}
	defer redisClient.Close()

	accessCache := cache.NewAccessCache(redisClient)

	// -------------------------------------------
	// 4. Initialize Service Layer
	// -------------------------------------------
	documentService := service.NewDocumentService(fc, accessCache)

	// -------------------------------------------
	// 5. Initialize HTTP Handler & Server
	// -------------------------------------------
	handler := httpTransport.NewHandler(documentService)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.App.Port),
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// -------------------------------------------
	// 6. Create Eureka Client
	// -------------------------------------------
	eurekaClient, err := eureka.NewClient(
		cfg.Eureka.ServerURL,
		cfg.App.Name,
		cfg.App.Port,
		cfg.App.Env,
		cfg.Eureka.HeartbeatInterval,
		cfg.Eureka.RetryInterval,
	)
	if err != nil {
		log.Fatalf("❌ Failed to create Eureka client: %v", err)
	}

	// -------------------------------------------
	// 7. Register with Eureka
	// -------------------------------------------
	log.Println("📡 Registering with Eureka server...")

	var registerErr error
	for i := 0; i < 3; i++ {
		registerErr = eurekaClient.Register()
		if registerErr == nil {
			break
		}
		log.Printf("⚠️ Registration attempt %d failed: %v - retrying in 5s", i+1, registerErr)
		time.Sleep(5 * time.Second)
	}
	if registerErr != nil {
		log.Printf("⚠️ Could not register with Eureka (service will run standalone): %v", registerErr)
	}

	// -------------------------------------------
	// 8. Create context for graceful shutdown
	// -------------------------------------------
	ctx, cancel := context.WithCancel(context.Background())

	// -------------------------------------------
	// 9. Start Eureka Heartbeat in Background
	// -------------------------------------------
	go eurekaClient.StartHeartbeat(ctx)

	// -------------------------------------------
	// 10. Start Kafka Consumer in Background
	//
	// Consumes from 4 topics:
	//   docube.document.create  → handleCreateDocument → fabric.CreateDocument
	//   docube.document.update  → handleUpdateDocument → fabric.UpdateDocument (private→public)
	//   docube.access.grant     → handleGrantAccess    → fabric.GrantAccess + cache invalidation
	//   docube.access.revoke    → handleRevokeAccess   → fabric.RevokeAccess + cache invalidation
	// -------------------------------------------
	kafkaCfg := kafka.KafkaConfig{
		Brokers:      cfg.Kafka.Brokers,
		GroupID:      cfg.Kafka.GroupID,
		SASLEnabled:  cfg.Kafka.SASLEnabled,
		SASLUsername: cfg.Kafka.SASLUsername,
		SASLPassword: cfg.Kafka.SASLPassword,
	}
	kafkaConsumer := kafka.NewConsumer(kafkaCfg, fc, accessCache)
	go kafkaConsumer.Start(ctx)
	log.Println("📨 Kafka consumer started")
	log.Println("   Topics: docube.document.create | docube.document.update | docube.access.grant | docube.access.revoke")

	// -------------------------------------------
	// 11. Start HTTP Server in Background
	// -------------------------------------------
	go func() {
		log.Printf("🌐 HTTP server listening on port %d", cfg.App.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ HTTP server failed: %v", err)
		}
	}()

	// -------------------------------------------
	// 12. Setup Signal Handler for Graceful Shutdown
	// -------------------------------------------
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	log.Println("========================================")
	log.Println("✅ Service is running and ready!")
	log.Printf("   HTTP API: http://localhost:%d/api/v1/blockchain", cfg.App.Port)
	log.Println("   Kafka Consumer: listening on 4 topics")
	log.Println("   Redis Cache: access control caching enabled")
	log.Println("   Press Ctrl+C to shutdown gracefully")
	log.Println("========================================")

	// -------------------------------------------
	// 13. Block Main Thread - Wait for Shutdown Signal
	// -------------------------------------------
	sig := <-sigCh
	log.Printf("\n🛑 Received signal: %v - initiating graceful shutdown...", sig)

	// -------------------------------------------
	// 14. Graceful Shutdown
	// -------------------------------------------
	cancel()
	eurekaClient.Stop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("⚠️ HTTP server shutdown error: %v", err)
	}

	log.Println("📡 Deregistering from Eureka...")
	if err := eurekaClient.Deregister(); err != nil {
		log.Printf("⚠️ Failed to deregister from Eureka: %v", err)
	}

	log.Println("========================================")
	log.Println("👋 Service shutdown complete. Goodbye!")
	log.Println("========================================")
}
