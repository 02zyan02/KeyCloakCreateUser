package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Nerzal/gocloak/v13"
)
 

type Metrics struct {
	mu            sync.Mutex
	totalRequests int
	totalLatency  time.Duration
	peakLatency   time.Duration
	errorCounts   map[int]int
	totalErrors   int
}

var metrics = Metrics{
	errorCounts: make(map[int]int),
}

var (
	totalGroupsCreated int
	totalUsersCreated  int
	mu                 sync.Mutex // Mutex to prevent race conditions
)

var (
	adminUser     = "admin"
	adminPassword = "admin"
	realm         = "master"
)

func main() {
	client := gocloak.NewClient("http://192.168.0.66:8080")
	ctx := context.Background()

	// Authenticate with Keycloak
	token, err := client.LoginAdmin(ctx, adminUser, adminPassword, realm)
	if err != nil {
		log.Fatalf("Login failed: %v", err)
	}

	expirationTime := time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)

	for {
		// Check if the token has expired or is about to expire
		if time.Now().After(expirationTime.Add(-5 * time.Minute)) {
			log.Println("Refreshing token...")
			newToken, err := client.RefreshToken(ctx, token.RefreshToken, "admin-cli", "", realm)
			if err != nil {
				log.Println("Token expired, logging in again...")
				newToken, err := client.LoginAdmin(ctx, adminUser, adminPassword, realm)
				if err != nil {
					log.Fatalf("Failed to reauthenticate: %v", err)
				}
				token = newToken
			} else {
				token = newToken
			}
			expirationTime = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
		}

		startTime := time.Now()
		err := createGroupAndUsers(ctx, client, token, realm, expirationTime)
		latency := time.Since(startTime)

		updateLatencyMetrics(latency)

		if err != nil {
			log.Printf("Error: %v", err)
		}
		printMetrics()
	}
}

func createGroupAndUsers(ctx context.Context, client *gocloak.GoCloak, token *gocloak.JWT, realm string, expirationTime time.Time) error {
	groupName := fmt.Sprintf("Group-%d", time.Now().Unix())
	startTime := time.Now()
	groupID, err := client.CreateGroup(ctx, token.AccessToken, realm, gocloak.Group{Name: &groupName})
	latency := time.Since(startTime)

	// Update latency metrics
	updateLatencyMetrics(latency)

	if err != nil {
		return fmt.Errorf("failed to create group: %v", err)
	}

	log.Printf("Created group: %s (ID: %s)", groupName, groupID)
	incrementGroupCounter()

	for subGrpIdx := 1; subGrpIdx <= 10; subGrpIdx++ {
		subGrpName := fmt.Sprintf("%s-subgroup-%d", groupName, subGrpIdx)
		subGrp := gocloak.Group{Name: &subGrpName}

		startTime := time.Now()
		subGrpID, err := client.CreateChildGroup(ctx, token.AccessToken, realm, groupID, subGrp)
		latency := time.Since(startTime)

		updateLatencyMetrics(latency)

		if err != nil {
			log.Printf("Failed to create subgroup %s: %v", subGrpName, err)
			updateErrorMetrics(500)
			continue
		}

		log.Printf("Created subgroup: %s (ID: %s)", subGrpName, subGrpID)

		time.Sleep(500 * time.Millisecond)

		//create user in subgroup
		for userIdx := 1; userIdx <= 10; userIdx++ {
			userName := fmt.Sprintf("User-%d-%d", time.Now().Unix(), userIdx)
			subGrpName := fmt.Sprintf("/%s/%s-subgroup-%d", groupName, groupName, subGrpIdx)

			user := gocloak.User{
				Username: &userName,
				Enabled:  gocloak.BoolP(true),
				Groups:   &[]string{subGrpName},
			}

			userID, err := client.CreateUser(ctx, token.AccessToken, realm, user)

			// Update latency metrics
			updateLatencyMetrics(latency)

			if err != nil {
				log.Printf("Failed to create user %s: %v", userName, err)
				updateErrorMetrics(500)
				continue
			}
			log.Printf("Created user: %s (ID: %s)", userName, userID)
			incrementUserCounter()
		}
		time.Sleep(5 * time.Minute)

		if time.Now().After(expirationTime.Add(-5 * time.Minute)) {
			log.Println("Refreshing token...")
			newToken, err := client.RefreshToken(ctx, token.RefreshToken, "admin-cli", "", realm)
			if err != nil {
				log.Println("Token expired, logging in again...")
				newToken, err := client.LoginAdmin(ctx, adminUser, adminPassword, realm)
				if err != nil {
					log.Fatalf("Failed to reauthenticate: %v", err)
				}
				token = newToken
			} else {
				token = newToken
			}
			expirationTime = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
		}

	}

	return nil
}

func incrementGroupCounter() {
	mu.Lock()
	defer mu.Unlock()
	totalGroupsCreated++
}

func incrementUserCounter() {
	mu.Lock()
	defer mu.Unlock()
	totalUsersCreated++
}

// Update metrics for request latency
func updateLatencyMetrics(latency time.Duration) {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	metrics.totalRequests++
	metrics.totalLatency += latency

	if latency > metrics.peakLatency {
		metrics.peakLatency = latency
	}
}

// Update error metrics
func updateErrorMetrics(statusCode int) {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	metrics.errorCounts[statusCode]++
	metrics.totalErrors++
}

// Print metrics
func printMetrics() {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	mu.Lock()
	defer mu.Unlock()

	//Calculate average latency
	avgLatency := time.Duration(0)
	if metrics.totalRequests > 0 {
		avgLatency = metrics.totalLatency / time.Duration(metrics.totalRequests)
	}
	log.Printf("Total groups created: %d", totalGroupsCreated)
	log.Printf("Total users created: %d", totalUsersCreated)
	log.Printf("Average Latency: %v", avgLatency)
	log.Printf("Peak Latency: %v", metrics.peakLatency)
	log.Printf("Total Errors: %d", metrics.totalErrors)

	// Print error counts by status code
	for code, count := range metrics.errorCounts {
		log.Printf("HTTP %d Errors: %d", code, count)
	}
}
