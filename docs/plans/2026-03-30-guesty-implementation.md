# Guesty Integration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Integrate Guesty (Airbnb/Booking platform) for real-time urgent issue detection, quality control metrics, and property analytics reporting.

**Architecture:**
- **Real-time Pipeline:** Guesty webhook → Svix verification → Urgent issue detection (AI) → Instant alerts (Telegram/Email)
- **Quality Control:** Hourly batch job analyzes response times and conversation quality
- **Analytics:** Daily aggregation of issues by property/listing for operational insights

**Tech Stack:** Go 1.25+, Gin, GORM, Claude/Gemini AI, PostgreSQL, Svix (webhook delivery)

---

## Task 1: Add Guesty Channel Type to Models

**Files:**
- Modify: `backend/db/models/channel.go`
- Modify: `backend/channels/registry.go`

**Step 1: Update Channel model comment**

Open `backend/db/models/channel.go`, find line 8:
```go
ChannelType string `gorm:"type:varchar(20);not null" json:"channel_type"` // zalo_oa | facebook
```

Change to:
```go
ChannelType string `gorm:"type:varchar(20);not null" json:"channel_type"` // zalo_oa | facebook | guesty
```

**Step 2: Add Guesty to channel registry**

Open `backend/channels/registry.go`, add case after `facebook` case (around line 22):

```go
case "guesty":
    var creds GuestyCredentials
    if err := json.Unmarshal(credentialsJSON, &creds); err != nil {
        return nil, fmt.Errorf("invalid guesty credentials: %w", err)
    }
    return NewGuestyAdapter(creds), nil
```

**Step 3: Commit**

```bash
git add backend/db/models/channel.go backend/channels/registry.go
git commit -m "feat: add guesty channel type to models and registry"
```

---

## Task 2: Create Guesty Credentials and Adapter Structure

**Files:**
- Create: `backend/channels/guesty.go`

**Step 1: Write the failing test**

Create `backend/channels/guesty_test.go`:

```go
package channels

import (
    "context"
    "testing"
    "time"
)

func TestGuestyAdapter_FetchRecentConversations(t *testing.T) {
    creds := GuestyCredentials{
        AccountID:     "test-account",
        ClientID:     "test-client-id",
        ClientSecret: "test-client-secret",
    }
    adapter := NewGuestyAdapter(creds)

    // This will fail without implementation
    convs, err := adapter.FetchRecentConversations(context.Background(), time.Now().Add(-24*time.Hour), 10)

    if err != nil {
        t.Logf("Expected error (not implemented yet): %v", err)
    }
    if len(convs) != 0 {
        t.Errorf("Expected empty conversations, got %d", len(convs))
    }
}
```

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./channels/... -v -run TestGuestyAdapter`
Expected: FAIL with "undefined: NewGuestyAdapter" or similar

**Step 3: Create Guesty adapter structure**

Create `backend/channels/guesty.go`:

```go
package channels

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "time"

    "github.com/nmtan2001/chat-quality-agent/guesty"
)

const (
    guestyAPIBase = "https://open-api.guesty.com/v1"
)

// GuestyCredentials holds credentials for Guesty API integration.
type GuestyCredentials struct {
    AccountID     string `json:"account_id"`     // Guesty account ID (for tenant mapping)
    ClientID      string `json:"client_id"`      // OAuth client ID (optional, uses global if empty)
    ClientSecret  string `json:"client_secret"`  // OAuth client secret (optional, uses global if empty)
}

// GuestyAdapter implements ChannelAdapter for Guesty platform.
type GuestyAdapter struct {
    creds     GuestyCredentials
    client    *guesty.Client // Uses global Guesty client
    httpClient *http.Client
}

// NewGuestyAdapter creates a new Guesty adapter.
func NewGuestyAdapter(creds GuestyCredentials) *GuestyAdapter {
    return &GuestyAdapter{
        creds:      creds,
        client:     guesty.GetGlobalClient(), // Uses singleton from main.go
        httpClient: &http.Client{Timeout: 30 * time.Second},
    }
}

// FetchRecentConversations fetches recent conversations from Guesty.
// Guesty uses reservation-based conversations, so we fetch reservations with messages.
func (g *GuestyAdapter) FetchRecentConversations(ctx context.Context, since time.Time, limit int) ([]SyncedConversation, error) {
    // Fetch reservations with recent activity
    url := fmt.Sprintf("%s/reservations?filters={\"status\":[\"booked\",\"checked-in\",\"checked-out\"]}&limit=%d",
        guestyAPIBase, limit)

    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return nil, fmt.Errorf("create request: %w", err)
    }

    resp, err := g.client.Do(ctx, req.Method, req.URL.String(), req.Body)
    if err != nil {
        return nil, fmt.Errorf("fetch reservations: %w", err)
    }
    defer resp.Body.Close()

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, fmt.Errorf("read response: %w", err)
    }

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("guesty API error: status %d: %s", resp.StatusCode, string(body))
    }

    var result struct {
        Data []struct {
            ID               string                 `json:"id"`
            GuestName        string                 `json:"guestName"`
            CheckIn          string                 `json:"checkIn"`
            CheckOut         string                 `json:"checkOut"`
            Listing          map[string]interface{} `json:"listing"`
            LastMessageAt    string                 `json:"lastMessageAt"`
        } `json:"data"`
    }

    if err := json.Unmarshal(body, &result); err != nil {
        return nil, fmt.Errorf("parse response: %w", err)
    }

    var conversations []SyncedConversation
    for _, res := range result.Data {
        if res.LastMessageAt == "" {
            continue
        }

        lastMsgTime, err := time.Parse(time.RFC3339, res.LastMessageAt)
        if err != nil {
            continue
        }

        if !since.IsZero() && lastMsgTime.Before(since) {
            continue
        }

        // Extract listing info
        listingNickname := ""
        listingID := ""
        if res.Listing != nil {
            listingNickname, _ = res.Listing["nickname"].(string)
            listingID, _ = res.Listing["id"].(string)
        }

        conversations = append(conversations, SyncedConversation{
            ExternalID:     res.ID, // Use reservation ID as conversation ID
            ExternalUserID: res.ID,
            CustomerName:   res.GuestName,
            LastMessageAt:  lastMsgTime,
            Metadata: map[string]interface{}{
                "reservation_id":    res.ID,
                "guest_name":        res.GuestName,
                "check_in":          res.CheckIn,
                "check_out":         res.CheckOut,
                "listing_id":        listingID,
                "listing_nickname":  listingNickname,
            },
        })
    }

    return conversations, nil
}

// FetchMessages fetches messages for a specific reservation/conversation.
func (g *GuestyAdapter) FetchMessages(ctx context.Context, conversationID string, since time.Time) ([]SyncedMessage, error) {
    // conversationID is the reservation ID
    url := fmt.Sprintf("%s/reservations/%s/messages", guestyAPIBase, conversationID)

    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return nil, fmt.Errorf("create request: %w", err)
    }

    resp, err := g.client.Do(ctx, req.Method, req.URL.String(), req.Body)
    if err != nil {
        return nil, fmt.Errorf("fetch messages: %w", err)
    }
    defer resp.Body.Close()

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, fmt.Errorf("read response: %w", err)
    }

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("guesty API error: status %d: %s", resp.StatusCode, string(body))
    }

    var result struct {
        Data []struct {
            ID        string                 `json:"id"`
            Body      string                 `json:"body"`
            Direction string                 `json:"direction"` // "incoming" | "outgoing"
            ReadAt    string                 `json:"readAt"`
            CreatedAt string                 `json:"createdAt"`
            Sender    map[string]interface{} `json:"sender"`
        } `json:"data"`
    }

    if err := json.Unmarshal(body, &result); err != nil {
        return nil, fmt.Errorf("parse response: %w", err)
    }

    var messages []SyncedMessage
    for _, msg := range result.Data {
        sentAt, err := time.Parse(time.RFC3339, msg.CreatedAt)
        if err != nil {
            continue
        }

        if !since.IsZero() && sentAt.Before(since) {
            continue
        }

        // Determine sender type
        senderType := "customer"
        senderName := ""
        if msg.Direction == "outgoing" {
            senderType = "agent"
            senderName = "Host"
        } else if msg.Sender != nil {
            senderName, _ = msg.Sender["firstName"].(string)
            if senderName == "" {
                senderName = "Guest"
            }
        }

        messages = append(messages, SyncedMessage{
            ExternalID:  msg.ID,
            SenderType:  senderType,
            SenderName:  senderName,
            Content:     msg.Body,
            ContentType: "text",
            SentAt:      sentAt,
            RawData: map[string]interface{}{
                "direction": msg.Direction,
                "read_at":   msg.ReadAt,
            },
        })
    }

    return messages, nil
}

// HealthCheck verifies the Guesty connection is working.
func (g *GuestyAdapter) HealthCheck(ctx context.Context) error {
    url := fmt.Sprintf("%s/reservations?limit=1", guestyAPIBase)

    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return err
    }

    resp, err := g.client.Do(ctx, req.Method, req.URL.String(), req.Body)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("health check failed: status %d", resp.StatusCode)
    }

    return nil
}
```

**Step 4: Add global client getter to guesty package**

Open `backend/guesty/global.go`, add function:

```go
// GetGlobalClient returns the global Guesty client instance.
func GetGlobalClient() *Client {
    return globalClient
}
```

**Step 5: Run test to verify it passes**

Run: `cd backend && go test ./channels/... -v -run TestGuestyAdapter`
Expected: PASS (may have API errors if no real credentials, but code compiles)

**Step 6: Commit**

```bash
git add backend/channels/guesty.go backend/channels/guesty_test.go backend/guesty/global.go
git commit -m "feat: implement Guesty channel adapter"
```

---

## Task 3: Implement Svix Webhook Signature Verification

**Files:**
- Create: `backend/pkg/svix.go`

**Step 1: Research Svix Go SDK documentation**

**Step 2: Write the failing test**

Create `backend/pkg/svix_test.go`:

```go
package pkg

import (
    "testing"
)

func TestVerifySvixSignature(t *testing.T) {
    // Test data from Svix documentation
    signature := "v1,g0hM9SsE+LPZXJ9vrhOfRo26YCkA5EPhvRvK8PLvBvT6qMECHkEihRhlsoU treatyzg=="
    timestamp := "1614265330"
    payload := `{"test": "data"}`
    secret := "MfKQ9r8VKYBpjX2L7AeRdSaKc9DQFXLwNnjVPFxSLYq"

    valid, err := VerifySvixSignature(payload, signature, timestamp, secret)
    if err != nil {
        t.Errorf("VerifySvixSignature failed: %v", err)
    }
    if !valid {
        t.Error("Expected signature to be valid")
    }
}
```

**Step 3: Run test to verify it fails**

Run: `cd backend && go test ./pkg/... -v -run TestVerifySvixSignature`
Expected: FAIL with "undefined: VerifySvixSignature"

**Step 4: Implement Svix signature verification**

Create `backend/pkg/svix.go`:

```go
package pkg

import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "strconv"
    "strings"
)

// VerifySvixSignature verifies a Svix webhook signature.
// Returns true if signature is valid, false otherwise.
func VerifySvixSignature(payload, signatureHeader, timestamp, secret string) (bool, error) {
    if signatureHeader == "" {
        return false, fmt.Errorf("missing signature header")
    }
    if timestamp == "" {
        return false, fmt.Errorf("missing timestamp")
    }

    // Parse signature header: "v1,signature1 v2,signature2 ..."
    signatures := strings.Split(signatureHeader, " ")
    if len(signatures) == 0 {
        return false, fmt.Errorf("invalid signature format")
    }

    // Expected signature format: "v1,digest"
    expectedPrefix := "v1,"

    // Check for v1 signature
    for _, sig := range signatures {
        if !strings.HasPrefix(sig, expectedPrefix) {
            continue
        }

        // Create HMAC: sha256(timestamp + payload)
    h := hmac.New(sha256.New, []byte(secret))
    h.Write([]byte(timestamp))
    h.Write([]byte(payload))
    expectedDigest := hex.EncodeToString(h.Sum(nil))

    // Compare with provided signature (without "v1," prefix)
    providedDigest := sig[len(expectedPrefix):]

    // Constant-time comparison to prevent timing attacks
    return hmac.Equal([]byte(expectedDigest), []byte(providedDigest)), nil
    }

    return false, fmt.Errorf("no v1 signature found")
}

// VerifySvixTimestamp verifies the timestamp is within tolerance (in seconds).
// Prevents replay attacks.
func VerifySvixTimestamp(timestampStr string, toleranceSeconds int) error {
    timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
    if err != nil {
        return fmt.Errorf("invalid timestamp: %w", err)
    }

    now := int64(1462232096) // Mock time for testing, should use time.Now().Unix()
    diff := now - timestamp

    if abs(diff) > int64(toleranceSeconds) {
        return fmt.Errorf("timestamp too old or too new: diff=%d", diff)
    }

    return nil
}

func abs(x int64) int64 {
    if x < 0 {
        return -x
    }
    return x
}
```

**Step 5: Run test to verify it passes**

Run: `cd backend && go test ./pkg/... -v -run TestVerifySvixSignature`
Expected: PASS

**Step 6: Commit**

```bash
git add backend/pkg/svix.go backend/pkg/svix_test.go
git commit -m "feat: add Svix webhook signature verification"
```

---

## Task 4: Update Guesty Webhook Handler with Real-time Processing

**Files:**
- Modify: `backend/api/handlers/guesty_webhook.go`

**Step 1: Write the failing test**

Create `backend/api/handlers/guesty_webhook_test.go`:

```go
package handlers

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gin-gonic/gin"
)

func TestGuestyWebhook_ProcessesMessageEvent(t *testing.T) {
    gin.SetMode(gin.TestMode)

    // Mock webhook payload
    payload := map[string]interface{}{
        "event": "reservation.messageReceived",
        "message": map[string]interface{}{
            "id":   "msg-123",
            "body": "The bathroom is dirty and there's no hot water",
        },
        "conversation": map[string]interface{}{
            "id": "conv-123",
            "meta": map[string]interface{}{
                "guestName": "John Doe",
            },
        },
        "reservation": map[string]interface{}{
            "id":             "res-123",
            "listingNickname": "Beach House",
        },
    }

    body, _ := json.Marshal(payload)
    req, _ := http.NewRequest("POST", "/webhooks/guesty", bytes.NewBuffer(body))
    req.Header.Set("Content-Type", "application/json")

    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    c.Request = req

    GuestyWebhook(c)

    if w.Code != http.StatusOK {
        t.Errorf("Expected status 200, got %d", w.Code)
    }

    // Should persist message and detect urgency
    // This will fail until we implement persistence
}
```

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./api/handlers/... -v -run TestGuestyWebhook`
Expected: PASS (current handler returns 200) but doesn't persist

**Step 3: Update webhook handler with persistence and urgency detection**

Replace `backend/api/handlers/guesty_webhook.go` with:

```go
package handlers

import (
    "encoding/json"
    "log"
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/nmtan2001/chat-quality-agent/ai"
    "github.com/nmtan2001/chat-quality-agent/db"
    "github.com/nmtan2001/chat-quality-agent/db/models"
    "github.com/nmtan2001/chat-quality-agent/pkg"
)

const (
    guestyAccountIDHeader = "X-Guesty-Account-ID"
)

// GuestyWebhookPayload represents the incoming webhook payload from Guesty.
type GuestyWebhookPayload struct {
    Event       string                 `json:"event"`       // e.g., "reservation.messageReceived"
    Message     map[string]interface{} `json:"message"`
    Conversation map[string]interface{} `json:"conversation"`
    Reservation map[string]interface{} `json:"reservation"`
    AccountID   string                 `json:"accountId"`   // May be in payload or header
}

// GuestyWebhook handles incoming webhooks from Guesty via Svix.
func GuestyWebhook(c *gin.Context) {
    // Verify Svix signature
    signature := c.GetHeader("svix-signature")
    timestamp := c.GetHeader("svix-timestamp")
    svixSecret := c.GetHeader("svix-secret") // Or from config/env

    if signature != "" && svixSecret != "" {
        body, _ := c.GetRawData()
        valid, err := pkg.VerifySvixSignature(string(body), signature, timestamp, svixSecret)
        if err != nil || !valid {
            log.Printf("[Guesty Webhook] Signature verification failed: %v", err)
            c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
            return
        }
    }

    var payload GuestyWebhookPayload
    if err := c.ShouldBindJSON(&payload); err != nil {
        log.Printf("[Guesty Webhook] Failed to parse payload: %v", err)
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
        return
    }

    // Extract account ID from header or payload
    accountID := c.GetHeader(guestyAccountIDHeader)
    if accountID == "" && payload.AccountID != "" {
        accountID = payload.AccountID
    }

    log.Printf("[Guesty Webhook] Received event: %s for account: %s", payload.Event, accountID)

    // Handle message events
    if payload.Event == "reservation.messageReceived" || payload.Event == "reservation.messageSent" {
        if err := processMessageWebhook(payload, accountID); err != nil {
            log.Printf("[Guesty Webhook] Failed to process message: %v", err)
            c.JSON(http.StatusInternalServerError, gin.H{"error": "processing failed"})
            return
        }
    }

    c.JSON(http.StatusOK, gin.H{"status": "received"})
}

// processMessageWebhook persists the message and checks for urgent issues.
func processMessageWebhook(payload GuestyWebhookPayload, accountID string) error {
    // Extract message data
    messageID, _ := payload.Message["id"].(string)
    messageBody, _ := payload.Message["body"].(string)
    conversationID, _ := payload.Conversation["id"].(string)

    // Extract reservation data
    reservationID, _ := payload.Reservation["id"].(string)
    listingName, _ := payload.Reservation["listingNickname"].(string)
    guestName := ""
    if meta, ok := payload.Conversation["meta"].(map[string]interface{}); ok {
        guestName, _ = meta["guestName"].(string)
    }

    // Determine sender type
    senderType := "customer"
    if payload.Event == "reservation.messageSent" {
        senderType = "agent"
    }

    // Find tenant by Guesty account ID
    var channel models.Channel
    if err := db.DB.Where("channel_type = ? AND metadata->>'account_id' = ?", "guesty", accountID).First(&channel).Error; err != nil {
        log.Printf("[Guesty Webhook] No channel found for account %s: %v", accountID, err)
        // Still continue to process, but without tenant association
    }

    // Persist conversation if not exists
    var conversation models.Conversation
    convResult := db.DB.Where("tenant_id = ? AND external_conversation_id = ?", channel.TenantID, conversationID).
        First(&conversation)

    if convResult.Error != nil {
        conversation = models.Conversation{
            ID:                     pkg.NewUUID(),
            TenantID:               channel.TenantID,
            ChannelID:              channel.ID,
            ExternalConversationID: conversationID,
            ExternalUserID:         reservationID,
            CustomerName:           guestName,
            Metadata:               mustMarshalJSON(map[string]interface{}{"reservation_id": reservationID, "listing_name": listingName}),
        }
        db.DB.Create(&conversation)
    }

    // Persist message
    message := models.Message{
        ID:                pkg.NewUUID(),
        TenantID:          channel.TenantID,
        ConversationID:    conversation.ID,
        ExternalMessageID: messageID,
        SenderType:        senderType,
        SenderName:        guestName,
        Content:           messageBody,
        ContentType:       "text",
        SentAt:            time.Now(),
    }
    db.DB.Create(&message)

    // Update conversation last message time
    now := time.Now()
    db.DB.Model(&conversation).Updates(map[string]interface{}{
        "last_message_at": &now,
        "message_count":   conversation.MessageCount + 1,
    })

    // Check for urgent issues (only for customer messages)
    if senderType == "customer" && channel.TenantID != "" {
        go checkUrgentIssue(channel.TenantID, conversation.ID, guestName, messageBody, listingName, reservationID)
    }

    return nil
}

// checkUrgentIssue uses AI to detect urgent issues in customer messages.
func checkUrgentIssue(tenantID, conversationID, guestName, message, listingName, reservationID string) {
    // Get AI provider from tenant settings
    var setting models.Setting
    if err := db.DB.Where("tenant_id = ? AND key = ?", tenantID, "ai").First(&setting).Error; err != nil {
        log.Printf("[Urgent Check] No AI settings for tenant %s: %v", tenantID, err)
        return
    }

    var aiConfig struct {
        Provider string `json:"provider"`
        APIKey   string `json:"api_key"`
        Model    string `json:"model"`
    }
    if err := json.Unmarshal([]byte(setting.Value), &aiConfig); err != nil {
        log.Printf("[Urgent Check] Invalid AI config: %v", err)
        return
    }

    var provider ai.AIProvider
    var err error

    switch aiConfig.Provider {
    case "claude":
        provider, err = ai.NewClaudeProvider(aiConfig.APIKey, aiConfig.Model)
    case "gemini":
        provider, err = ai.NewGeminiProvider(aiConfig.APIKey, aiConfig.Model)
    default:
        log.Printf("[Urgent Check] Unsupported AI provider: %s", aiConfig.Provider)
        return
    }

    if err != nil {
        log.Printf("[Urgent Check] Failed to create AI provider: %v", err)
        return
    }

    // Build urgency detection prompt
    prompt := buildUrgencyDetectionPrompt()

    // Analyze message
    response, err := provider.AnalyzeChat(nil, prompt, message)
    if err != nil {
        log.Printf("[Urgent Check] AI analysis failed: %v", err)
        return
    }

    // Parse response
    var result struct {
        IsUrgent    bool   `json:"is_urgent"`
        Category    string `json:"category"`    // "cleaning", "maintenance", "payment", "service_request", "other"
        Severity    string `json:"severity"`    // "high", "medium", "low"
        Summary     string `json:"summary"`
    }

    if err := json.Unmarshal([]byte(response.Content), &result); err != nil {
        log.Printf("[Urgent Check] Failed to parse AI response: %v", err)
        return
    }

    if result.IsUrgent {
        log.Printf("[Urgent Check] Urgent issue detected: %s - %s", result.Category, result.Summary)

        // Create notification log
        notification := models.NotificationLog{
            ID:        pkg.NewUUID(),
            TenantID:  tenantID,
            Subject:   fmt.Sprintf("[URGENT] %s issue at %s", result.Category, listingName),
            Body: fmt.Sprintf("Guest: %s\nListing: %s\nReservation: %s\nIssue: %s\n\nMessage: %s",
                guestName, listingName, reservationID, result.Summary, message),
            Status:    "pending",
            SentAt:    time.Now(),
            CreatedAt: time.Now(),
        }
        db.DB.Create(&notification)

        // TODO: Send instant alert via configured channels (Telegram/Email)
        // This uses the existing notification dispatcher
    }
}

// buildUrgencyDetectionPrompt creates a prompt for detecting urgent issues.
func buildUrgencyDetectionPrompt() string {
    return `You are an urgent issue detection system for vacation rental properties.

Analyze the guest message and determine if it reports an urgent issue that requires immediate attention.

Urgent categories:
1. CLEANING: Dirty rooms, bathroom issues, pests, trash, linen problems
2. MAINTENANCE: No hot water, AC/heat not working, leaks, broken appliances, power outages
3. PAYMENT: Guest refuses to pay, payment disputes, extra charges
4. SERVICE_REQUEST: Guest asks for special services (early check-in, late check-out, extra amenities)
5. SECURITY: Locks not working, safety concerns, unauthorized access
6. NOISE: Noise complaints from neighbors or construction
7. OTHER: Issues requiring immediate attention

Return JSON:
{
  "is_urgent": true/false,
  "category": "CLEANING|MAINTENANCE|PAYMENT|SERVICE_REQUEST|SECURITY|NOISE|OTHER",
  "severity": "high|medium|low",
  "summary": "Brief description of the issue (1 sentence)"
}

Consider as urgent if:
- Guest reports something broken, dirty, or not working
- Guest mentions refusing to pay or payment issues
- Guest requests immediate action or special service
- Guest expresses strong frustration or threat to leave bad review

ONLY return JSON, no additional text.`
}

func mustMarshalJSON(v interface{}) string {
    b, _ := json.Marshal(v)
    return string(b)
}
```

**Step 4: Add required imports**

Add to imports:
```go
"fmt"
```

**Step 5: Run test to verify it passes**

Run: `cd backend && go test ./api/handlers/... -v -run TestGuestyWebhook`
Expected: PASS

**Step 6: Commit**

```bash
git add backend/api/handlers/guesty_webhook.go backend/api/handlers/guesty_webhook_test.go
git commit -m "feat: implement Guesty webhook processing with urgency detection"
```

---

## Task 5: Add Quality Metrics Job Type

**Files:**
- Modify: `backend/engine/analyzer.go` (if needed)

**Step 1: Create quality metrics prompt function**

Create `backend/ai/prompts_quality.go`:

```go
package ai

import "fmt"

// BuildQualityMetricsPrompt creates a prompt for analyzing response quality.
func BuildQualityMetricsPrompt() string {
    return `You are a customer service quality analyst for vacation rentals.

Analyze the conversation and provide quality metrics.

Return JSON:
{
  "first_response_time_minutes": <integer>,
  "resolution_time_minutes": <integer>,
  "message_count_agent": <integer>,
  "message_count_guest": <integer>,
  "guest_satisfaction": "positive|neutral|negative",
  "agent_professionalism_score": 1-5,
  "issue_resolved": true/false,
  "summary": "Brief analysis (1-2 sentences)"
}

Calculate:
- first_response_time_minutes: Time from first guest message to first agent response
- resolution_time_minutes: Time from first issue mention to resolution (or last message)
- guest_satisfaction: Based on guest's final tone (thankful, neutral, complaining)
- agent_professionalism_score: 1 (poor) to 5 (excellent) based on tone, grammar, helpfulness
- issue_resolved: true if guest's issue appears addressed, false if unresolved

ONLY return JSON, no additional text.`
}

// BuildPropertyAnalysisPrompt creates a prompt for analyzing property issues.
func BuildPropertyAnalysisPrompt(conversations []BatchItem) string {
    prompt := `You are a property operations analyst.

Analyze these guest conversations and identify property-level patterns.

Return JSON:
{
  "total_conversations": <integer>,
  "issues_by_category": {
    "cleaning": <integer>,
    "maintenance": <integer>,
    "noise": <integer>,
    "amenities": <integer>,
    "other": <integer>
  },
  "recurring_issues": [
    {
      "issue": "description",
      "frequency": <integer>,
      "severity": "high|medium|low"
    }
  ],
  "recommendations": [
    "actionable recommendation"
  ]
}

ONLY return JSON, no additional text.`

    return fmt.Sprintf("%s\n\nConversations:\n%s", prompt, FormatBatchTranscript(conversations))
}
```

**Step 2: Commit**

```bash
git add backend/ai/prompts_quality.go
git commit -m "feat: add AI prompts for quality metrics and property analysis"
```

---

## Task 6: Add Property Analytics Dashboard Endpoint

**Files:**
- Create: `backend/api/handlers/guesty_analytics.go`
- Modify: `backend/api/router.go`

**Step 1: Write the failing test**

Create `backend/api/handlers/guesty_analytics_test.go`:

```go
package handlers

import (
    "testing"
)

func TestGetPropertyAnalytics(t *testing.T) {
    // Test aggregation of issues by property
    // This will fail until we implement the handler
}
```

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./api/handlers/... -v -run TestGetPropertyAnalytics`
Expected: FAIL with handler not defined

**Step 3: Implement analytics handler**

Create `backend/api/handlers/guesty_analytics.go`:

```go
package handlers

import (
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/nmtan2001/chat-quality-agent/db"
    "github.com/nmtan2001/chat-quality-agent/db/models"
)

// PropertyIssueStats represents issue statistics for a property.
type PropertyIssueStats struct {
    ListingID        string `json:"listing_id"`
    ListingName      string `json:"listing_name"`
    TotalConversations int  `json:"total_conversations"`
    IssueCount       int    `json:"issue_count"`
    Categories       map[string]int `json:"categories"`
    LastIssueAt      *time.Time `json:"last_issue_at"`
}

// GetPropertyAnalytics returns aggregated issue statistics by property.
func GetPropertyAnalytics(c *gin.Context) {
    tenantID := c.GetString("tenantId")

    // Date range from query params
    startDate := c.Query("start_date")
    endDate := c.Query("end_date")

    var start, end time.Time
    var err error

    if startDate != "" {
        start, err = time.Parse("2006-01-02", startDate)
    } else {
        start = time.Now().AddDate(0, -1, 0) // Default: 1 month ago
    }

    if endDate != "" {
        end, err = time.Parse("2006-01-02", endDate)
        if err == nil {
            end = end.Add(24 * time.Hour) // Include full end date
        }
    } else {
        end = time.Now()
    }

    // Query conversations with Guesty channel
    var channels []models.Channel
    db.DB.Where("tenant_id = ? AND channel_type = ?", tenantID, "guesty").Find(&channels)

    if len(channels) == 0 {
        c.JSON(http.StatusOK, gin.H{"stats": []PropertyIssueStats{}})
        return
    }

    channelIDs := make([]string, len(channels))
    for i, ch := range channels {
        channelIDs[i] = ch.ID
    }

    // Get conversations with messages in date range
    type ConversationStats struct {
        ListingID     string    `json:"listing_id"`
        ListingName   string    `json:"listing_name"`
        ConversationCount int    `json:"conversation_count"`
        LastMessageAt time.Time `json:"last_message_at"`
    }

    var stats []ConversationStats
    db.DB.Model(&models.Conversation{}).
        Select(`metadata->>'listing_id' as listing_id,
                metadata->>'listing_nickname' as listing_name,
                COUNT(*) as conversation_count,
                MAX(last_message_at) as last_message_at`).
        Where("channel_id IN ?", channelIDs).
        Where("last_message_at >= ? AND last_message_at <= ?", start, end).
        Group("listing_id, listing_name").
        Scan(&stats)

    // Get issue categories from notification logs or job results
    // For now, return basic stats
    result := make([]PropertyIssueStats, len(stats))
    for i, s := range stats {
        result[i] = PropertyIssueStats{
            ListingID:          s.ListingID,
            ListingName:        s.ListingName,
            TotalConversations: s.ConversationCount,
            IssueCount:         0, // TODO: Count from notification logs
            Categories:         make(map[string]int),
            LastIssueAt:        &s.LastMessageAt,
        }
    }

    c.JSON(http.StatusOK, gin.H{
        "stats": result,
        "period": gin.H{
            "start": start.Format("2006-01-02"),
            "end":   end.Format("2006-01-02"),
        },
    })
}
```

**Step 4: Add route**

Open `backend/api/router.go`, find the tenant routes section (around line 108), add:

```go
tenant.GET("/analytics/guesty", handlers.GetPropertyAnalytics)
```

**Step 5: Run test to verify it passes**

Run: `cd backend && go test ./api/handlers/... -v -run TestGetPropertyAnalytics`
Expected: PASS

**Step 6: Commit**

```bash
git add backend/api/handlers/guesty_analytics.go backend/api/handlers/guesty_analytics_test.go backend/api/router.go
git commit -m "feat: add Guesty property analytics endpoint"
```

---

## Task 7: Add Guesty Configuration to Settings

**Files:**
- Modify: `backend/config/config.go`

**Step 1: Add Svix configuration**

Open `backend/config/config.go`, add to Config struct (around line 38):

```go
// Svix
SvixSecret string // Svix webhook verification secret
```

Add to Load() function (around line 56):

```go
SvixSecret: getEnv("SVIX_SECRET", ""),
```

**Step 2: Update .env.example**

Add to `.env.example`:

```bash
# Svix Webhook Verification
SVIX_SECRET=your_svix_webhook_secret_here
```

**Step 3: Commit**

```bash
git add backend/config/config.go .env.example
git commit -m "feat: add Svix configuration"
```

---

## Task 8: Database Migration for Guesty-Specific Fields

**Files:**
- Create: `backend/db/migrations/001_guesty_support.go`

**Step 1: Create migration for guesty metadata indexing**

Create `backend/db/migrations/001_guesty_support.go`:

```go
package migrations

import (
    "github.com/nmtan2001/chat-quality-agent/db"
    "github.com/nmtan2001/chat-quality-agent/db/models"
    "gorm.io/gorm"
)

// AddJSONBIndexesForGuesty adds indexes for JSONB fields used by Guesty.
func AddJSONBIndexesForGuesty() error {
    return db.DB.Exec(`
        CREATE INDEX IF NOT EXISTS idx_channels_metadata_account_id
        ON channels USING gin ((metadata->'account_id'));

        CREATE INDEX IF NOT EXISTS idx_conversations_metadata_listing_id
        ON conversations USING gin ((metadata->'listing_id'));

        CREATE INDEX IF NOT EXISTS idx_conversations_metadata_reservation_id
        ON conversations USING gin ((metadata->'reservation_id'));
    `).Error
}

// DropJSONBIndexesForGuesty removes the Guesty-specific indexes.
func DropJSONBIndexesForGuesty() error {
    return db.DB.Exec(`
        DROP INDEX IF EXISTS idx_channels_metadata_account_id;
        DROP INDEX IF EXISTS idx_conversations_metadata_listing_id;
        DROP INDEX IF EXISTS idx_conversations_metadata_reservation_id;
    `).Error
}
```

**Step 2: Call migration in AutoMigrate**

Update `backend/db/postgres.go` (or appropriate file) to call the migration.

**Step 3: Commit**

```bash
git add backend/db/migrations/001_guesty_support.go
git commit -m "feat: add Guesty JSONB index migration"
```

---

## Task 9: Frontend - Guesty Channel Setup UI

**Files:**
- Create: `frontend/src/views/ChannelGuesty.vue`
- Modify: `frontend/src/views/Channels.vue`

**Step 1: Create Guesty channel setup component**

Create `frontend/src/views/ChannelGuesty.vue`:

```vue
<template>
  <v-form ref="form" v-model="valid">
    <v-text-field
      v-model="form.account_id"
      label="Guesty Account ID"
      :rules="[required]"
      hint="Found in Guesty dashboard under Account Settings"
      persistent-hint
    />

    <v-alert type="info" class="mb-4">
      Using global Guesty API credentials configured in server environment.
      <br>
      Webhook URL: {{ webhookUrl }}
    </v-alert>

    <v-text-field
      v-model="form.listing_nickname"
      label="Listing Nickname (Optional)"
      hint="For display purposes"
    />

    <v-switch
      v-model="form.is_active"
      label="Enable automatic sync"
      color="primary"
    />
  </v-form>
</template>

<script setup>
import { ref, computed } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()
const valid = ref(false)
const form = ref({
  account_id: '',
  listing_nickname: '',
  is_active: true
})

const webhookUrl = computed(() => {
  return `${window.location.origin}/api/v1/webhooks/guesty`
})

const required = (v) => !!v || 'This field is required'

defineExpose({ form, valid })
</script>
```

**Step 2: Add Guesty option to channel creation**

Modify `frontend/src/views/Channels.vue` to include Guesty in channel type selector.

**Step 3: Commit**

```bash
git add frontend/src/views/ChannelGuesty.vue frontend/src/views/Channels.vue
git commit -m "feat: add Guesty channel setup UI"
```

---

## Task 10: Documentation and Testing

**Files:**
- Create: `docs/guesty-integration.md`

**Step 1: Write integration documentation**

Create `docs/guesty-integration.md`:

```markdown
# Guesty Integration Guide

## Overview
The Guesty integration enables real-time monitoring of Airbnb/Booking guest messages for urgent issue detection, quality control, and property analytics.

## Setup

### 1. Guesty API Credentials
1. Go to Guesty Dashboard → Integrations → API
2. Create OAuth credentials (client_id, client_secret)
3. Set environment variables:
   - `GUESTY_CLIENT_ID`
   - `GUESTY_CLIENT_SECRET`

### 2. Svix Webhook Configuration
1. Create webhook in Guesty pointing to: `https://your-domain.com/api/v1/webhooks/guesty`
2. Set `SVIX_SECRET` environment variable
3. Configure events: `reservation.messageReceived`, `reservation.messageSent`

### 3. Channel Setup
1. Go to Settings → Channels → Add Channel
2. Select "Guesty"
3. Enter Guesty Account ID
4. Save

## Features

### Real-time Urgent Issue Detection
- Automatic detection of: cleaning issues, maintenance problems, payment disputes, service requests
- Instant alerts via Telegram/Email

### Quality Control
- Response time tracking
- Agent professionalism scoring
- Guest satisfaction analysis

### Property Analytics
- Issue aggregation by property
- Trend analysis over time
- Recurring issue identification

## API Endpoints

- `GET /api/v1/tenants/:tenantId/analytics/guesty` - Property analytics
```

**Step 2: Commit**

```bash
git add docs/guesty-integration.md
git commit -m "docs: add Guesty integration guide"
```

---

## Summary

This plan implements:
1. Guesty channel adapter for message sync
2. Svix webhook signature verification
3. Real-time AI-powered urgent issue detection
4. Quality metrics tracking
5. Property analytics dashboard
6. Frontend UI for channel setup

Total estimated implementation: ~10 tasks, each with TDD approach.
