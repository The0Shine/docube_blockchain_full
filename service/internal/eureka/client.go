package eureka

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Client is a Eureka service discovery client
type Client struct {
	serverURL   string
	appName     string
	instanceID  string
	hostname    string
	ipAddress   string
	port        int
	env         string
	httpClient  *http.Client
	heartbeatInterval time.Duration
	retryInterval     time.Duration
	stopCh      chan struct{}
}

// InstanceInfo represents Eureka instance registration payload
type InstanceInfo struct {
	Instance Instance `json:"instance"`
}

// Instance holds instance details for Eureka
type Instance struct {
	InstanceID       string            `json:"instanceId"`
	HostName         string            `json:"hostName"`
	App              string            `json:"app"`
	IPAddr           string            `json:"ipAddr"`
	Status           string            `json:"status"`
	Port             PortConfig        `json:"port"`
	SecurePort       PortConfig        `json:"securePort"`
	DataCenterInfo   DataCenterInfo    `json:"dataCenterInfo"`
	Metadata         map[string]string `json:"metadata"`
	HomePageURL      string            `json:"homePageUrl"`
	StatusPageURL    string            `json:"statusPageUrl"`
	HealthCheckURL   string            `json:"healthCheckUrl"`
	VIPAddress       string            `json:"vipAddress"`
	SecureVIPAddress string            `json:"secureVipAddress"`
}

// PortConfig holds port configuration
type PortConfig struct {
	Value   int    `json:"$"`
	Enabled string `json:"@enabled"`
}

// DataCenterInfo holds datacenter information
type DataCenterInfo struct {
	Class string `json:"@class"`
	Name  string `json:"name"`
}

// NewClient creates a new Eureka client
func NewClient(serverURL, appName string, port int, env string, heartbeatSec, retrySec int) (*Client, error) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "localhost"
	}

	ipAddress := getOutboundIP()

	// EUREKA_INSTANCE_IP overrides the auto-detected IP and hostname.
	// EUREKA_INSTANCE_PORT overrides the port registered to Eureka.
	// Use these when the service runs inside Docker but the gateway runs on the host:
	// set EUREKA_INSTANCE_IP=127.0.0.1 and EUREKA_INSTANCE_PORT=<host-port>, and expose
	// that host port to the container so Eureka registers a host-reachable address.
	if instanceIP := os.Getenv("EUREKA_INSTANCE_IP"); instanceIP != "" {
		ipAddress = instanceIP
		hostname = instanceIP
	}
	if instancePort := os.Getenv("EUREKA_INSTANCE_PORT"); instancePort != "" {
		if p, err := strconv.Atoi(instancePort); err == nil {
			port = p
		}
	}

	instanceID := fmt.Sprintf("%s:%s:%d", hostname, strings.ToLower(appName), port)

	return &Client{
		serverURL:   strings.TrimSuffix(serverURL, "/"),
		appName:     strings.ToUpper(appName),
		instanceID:  instanceID,
		hostname:    hostname,
		ipAddress:   ipAddress,
		port:        port,
		env:         env,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		heartbeatInterval: time.Duration(heartbeatSec) * time.Second,
		retryInterval:     time.Duration(retrySec) * time.Second,
		stopCh:      make(chan struct{}),
	}, nil
}

// Register registers the instance with Eureka server
func (c *Client) Register() error {
	instanceInfo := c.buildInstanceInfo()

	body, err := json.Marshal(instanceInfo)
	if err != nil {
		return fmt.Errorf("failed to marshal instance info: %w", err)
	}

	url := fmt.Sprintf("%s/apps/%s", c.serverURL, c.appName)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to register with Eureka: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("eureka registration failed with status: %d", resp.StatusCode)
	}

	log.Printf("[EUREKA] ✅ Successfully registered: app=%s, instanceId=%s, ip=%s:%d",
		c.appName, c.instanceID, c.ipAddress, c.port)

	return nil
}

// Deregister removes the instance from Eureka server
func (c *Client) Deregister() error {
	url := fmt.Sprintf("%s/apps/%s/%s", c.serverURL, c.appName, c.instanceID)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create deregister request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to deregister from Eureka: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("eureka deregistration failed with status: %d", resp.StatusCode)
	}

	log.Printf("[EUREKA] ✅ Successfully deregistered: app=%s, instanceId=%s", c.appName, c.instanceID)
	return nil
}

// StartHeartbeat starts sending periodic heartbeats to Eureka
func (c *Client) StartHeartbeat(ctx context.Context) {
	ticker := time.NewTicker(c.heartbeatInterval)
	defer ticker.Stop()

	log.Printf("[EUREKA] 💓 Starting heartbeat every %v", c.heartbeatInterval)

	for {
		select {
		case <-ctx.Done():
			log.Println("[EUREKA] Heartbeat stopped due to context cancellation")
			return
		case <-c.stopCh:
			log.Println("[EUREKA] Heartbeat stopped")
			return
		case <-ticker.C:
			if err := c.sendHeartbeat(); err != nil {
				log.Printf("[EUREKA] ⚠️ Heartbeat failed: %v - retrying in %v", err, c.retryInterval)
				// Retry with shorter interval
				time.Sleep(c.retryInterval)
				if err := c.sendHeartbeat(); err != nil {
					log.Printf("[EUREKA] ⚠️ Heartbeat retry failed: %v - will try re-registration", err)
					// Try to re-register
					if regErr := c.Register(); regErr != nil {
						log.Printf("[EUREKA] ❌ Re-registration failed: %v", regErr)
					}
				}
			}
		}
	}
}

// Stop stops the heartbeat
func (c *Client) Stop() {
	close(c.stopCh)
}

func (c *Client) sendHeartbeat() error {
	url := fmt.Sprintf("%s/apps/%s/%s", c.serverURL, c.appName, c.instanceID)
	req, err := http.NewRequest(http.MethodPut, url, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("instance not found - need to re-register")
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("heartbeat failed with status: %d", resp.StatusCode)
	}

	log.Printf("[EUREKA] 💓 Heartbeat sent successfully")
	return nil
}

func (c *Client) buildInstanceInfo() InstanceInfo {
	baseURL := fmt.Sprintf("http://%s:%d", c.ipAddress, c.port)

	return InstanceInfo{
		Instance: Instance{
			InstanceID: c.instanceID,
			HostName:   c.hostname,
			App:        c.appName,
			IPAddr:     c.ipAddress,
			Status:     "UP",
			Port: PortConfig{
				Value:   c.port,
				Enabled: "true",
			},
			SecurePort: PortConfig{
				Value:   443,
				Enabled: "false",
			},
			DataCenterInfo: DataCenterInfo{
				Class: "com.netflix.appinfo.InstanceInfo$DefaultDataCenterInfo",
				Name:  "MyOwn",
			},
			Metadata: map[string]string{
				"env":      c.env,
				"language": "go",
				"role":     "gateway",
			},
			HomePageURL:      baseURL,
			StatusPageURL:    baseURL + "/info",
			HealthCheckURL:   baseURL + "/health",
			VIPAddress:       strings.ToLower(c.appName),
			SecureVIPAddress: strings.ToLower(c.appName),
		},
	}
}

// getOutboundIP gets the preferred outbound IP of this machine
func getOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}
