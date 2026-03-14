package config

import (
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds all application configuration
type Config struct {
	App    AppConfig    `yaml:"app"`
	Eureka EurekaConfig `yaml:"eureka"`
	Kafka  KafkaConfig  `yaml:"kafka"`
	Redis  RedisConfig  `yaml:"redis"`
}

// RedisConfig holds Redis connection settings
type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
	TTL      int    `yaml:"ttl"` // Cache TTL in seconds
}

// KafkaConfig holds Kafka consumer settings
type KafkaConfig struct {
	Brokers      []string `yaml:"brokers"`
	GroupID      string   `yaml:"group_id"`
	SASLEnabled  bool     `yaml:"sasl_enabled"`
	SASLUsername string   `yaml:"sasl_username"`
	SASLPassword string   `yaml:"sasl_password"`
}

// FabricConfig holds Fabric network connection settings
type FabricConfig struct {
	Fabric struct {
		ChannelName   string `yaml:"channel_name"`
		ChaincodeName string `yaml:"chaincode_name"`
		MspID         string `yaml:"msp_id"`
		PeerEndpoint  string `yaml:"peer_endpoint"`
		GatewayPeer   string `yaml:"gateway_peer"`
		CryptoPath    string `yaml:"crypto_path"`
		CertPath      string `yaml:"cert_path"`
		KeyDir        string `yaml:"key_dir"`
		TLSCertPath   string `yaml:"tls_cert_path"`
	} `yaml:"fabric"`
}

// AppConfig holds application-specific settings
type AppConfig struct {
	Name string `yaml:"name"`
	Port int    `yaml:"port"`
	Env  string `yaml:"env"`
}

// EurekaConfig holds Eureka client settings
type EurekaConfig struct {
	ServerURL         string `yaml:"server_url"`
	HeartbeatInterval int    `yaml:"heartbeat_interval"`
	RetryInterval     int    `yaml:"retry_interval"`
}

// Load loads configuration from YAML file and environment variables
// Environment variables take precedence over YAML values
func Load(configPath string) (*Config, error) {
	cfg := &Config{
		App: AppConfig{
			Name: "fabric-gateway-service",
			Port: 8081,
			Env:  "dev",
		},
		Eureka: EurekaConfig{
			ServerURL:         "http://localhost:9000/eureka",
			HeartbeatInterval: 30,
			RetryInterval:     5,
		},
		Kafka: KafkaConfig{
			Brokers:      []string{"localhost:7092"},
			GroupID:      "blockchain-service-group",
			SASLEnabled:  true,
			SASLUsername: "horob1",
			SASLPassword: "2410",
		},
		Redis: RedisConfig{
			Addr:     "localhost:6379",
			Password: "2410",
			DB:       1,
			TTL:      900, // 15 minutes
		},
	}

	// Load from YAML file if exists
	if configPath != "" {
		if err := loadFromYAML(configPath, cfg); err != nil {
			println("Warning: Could not load config file:", err.Error())
		}
	}

	// Override with environment variables
	loadFromEnv(cfg)

	return cfg, nil
}

// LoadFabric loads Fabric-specific configuration
// Environment variables take precedence over YAML values
func LoadFabric(configPath string) (*FabricConfig, error) {
	cfg := &FabricConfig{}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	// Override with environment variables
	if v := os.Getenv("FABRIC_CRYPTO_PATH"); v != "" {
		cfg.Fabric.CryptoPath = v
	}
	if v := os.Getenv("FABRIC_PEER_ENDPOINT"); v != "" {
		cfg.Fabric.PeerEndpoint = v
	}
	if v := os.Getenv("FABRIC_GATEWAY_PEER"); v != "" {
		cfg.Fabric.GatewayPeer = v
	}
	if v := os.Getenv("FABRIC_CHANNEL_NAME"); v != "" {
		cfg.Fabric.ChannelName = v
	}
	if v := os.Getenv("FABRIC_CHAINCODE_NAME"); v != "" {
		cfg.Fabric.ChaincodeName = v
	}
	if v := os.Getenv("FABRIC_MSP_ID"); v != "" {
		cfg.Fabric.MspID = v
	}
	if v := os.Getenv("FABRIC_CERT_PATH"); v != "" {
		cfg.Fabric.CertPath = v
	}
	if v := os.Getenv("FABRIC_KEY_DIR"); v != "" {
		cfg.Fabric.KeyDir = v
	}
	if v := os.Getenv("FABRIC_TLS_CERT_PATH"); v != "" {
		cfg.Fabric.TLSCertPath = v
	}

	return cfg, nil
}

func loadFromYAML(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, cfg)
}

func loadFromEnv(cfg *Config) { //nolint:cyclop
	// Kafka overrides
	if v := os.Getenv("KAFKA_BROKERS"); v != "" {
		cfg.Kafka.Brokers = splitAndTrim(v)
	}
	if v := os.Getenv("KAFKA_GROUP_ID"); v != "" {
		cfg.Kafka.GroupID = v
	}
	if v := os.Getenv("KAFKA_SASL_USERNAME"); v != "" {
		cfg.Kafka.SASLUsername = v
	}
	if v := os.Getenv("KAFKA_SASL_PASSWORD"); v != "" {
		cfg.Kafka.SASLPassword = v
	}
	if v := os.Getenv("KAFKA_SASL_ENABLED"); v != "" {
		if enabled, err := strconv.ParseBool(v); err == nil {
			cfg.Kafka.SASLEnabled = enabled
		}
	}

	if v := os.Getenv("APP_NAME"); v != "" {
		cfg.App.Name = v
	}
	if v := os.Getenv("APP_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.App.Port = port
		}
	}
	if v := os.Getenv("ENV"); v != "" {
		cfg.App.Env = v
	}
	if v := os.Getenv("EUREKA_SERVER_URL"); v != "" {
		cfg.Eureka.ServerURL = v
	}
	if v := os.Getenv("EUREKA_HEARTBEAT_INTERVAL"); v != "" {
		if interval, err := strconv.Atoi(v); err == nil {
			cfg.Eureka.HeartbeatInterval = interval
		}
	}
	if v := os.Getenv("REDIS_ADDR"); v != "" {
		cfg.Redis.Addr = v
	}
	if v := os.Getenv("REDIS_PASSWORD"); v != "" {
		cfg.Redis.Password = v
	}
	if v := os.Getenv("REDIS_DB"); v != "" {
		if db, err := strconv.Atoi(v); err == nil {
			cfg.Redis.DB = db
		}
	}
	if v := os.Getenv("REDIS_TTL"); v != "" {
		if ttl, err := strconv.Atoi(v); err == nil {
			cfg.Redis.TTL = ttl
		}
	}
}

// splitAndTrim splits a comma-separated string and trims whitespace from each part.
func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
