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
	AccountID    string `json:"account_id"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// GuestyAdapter implements ChannelAdapter for Guesty platform.
type GuestyAdapter struct {
	creds      GuestyCredentials
	client     *guesty.Client
	httpClient *http.Client
}

// NewGuestyAdapter creates a new Guesty adapter.
func NewGuestyAdapter(creds GuestyCredentials) *GuestyAdapter {
	return &GuestyAdapter{
		creds:      creds,
		client:     guesty.GlobalClient(),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// FetchRecentConversations fetches recent conversations from Guesty.
// Guesty uses reservation-based conversations, so we fetch reservations with messages.
func (g *GuestyAdapter) FetchRecentConversations(ctx context.Context, since time.Time, limit int) ([]SyncedConversation, error) {
	path := fmt.Sprintf("%s/reservations?filters={\"status\":[\"booked\",\"checked-in\",\"checked-out\"]}&limit=%d", guestyAPIBase, limit)

	req, err := http.NewRequestWithContext(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := g.client.Do(ctx, req.Method, req.URL.String(), nil)
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
			ID            string                 `json:"id"`
			GuestName     string                 `json:"guestName"`
			CheckIn       string                 `json:"checkIn"`
			CheckOut      string                 `json:"checkOut"`
			LastMessageAt string                 `json:"lastMessageAt"`
			Listing       map[string]interface{} `json:"listing"`
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
			if nickname, ok := res.Listing["nickname"].(string); ok {
				listingNickname = nickname
			}
			if id, ok := res.Listing["id"].(string); ok {
				listingID = id
			}
		}

		conversations = append(conversations, SyncedConversation{
			ExternalID:     res.ID,
			ExternalUserID: res.ID,
			CustomerName:   res.GuestName,
			LastMessageAt:  lastMsgTime,
			Metadata: map[string]interface{}{
				"reservation_id":   res.ID,
				"guest_name":       res.GuestName,
				"check_in":         res.CheckIn,
				"check_out":        res.CheckOut,
				"listing_id":       listingID,
				"listing_nickname": listingNickname,
			},
		})
	}

	return conversations, nil
}

// FetchMessages fetches messages for a specific reservation/conversation.
func (g *GuestyAdapter) FetchMessages(ctx context.Context, conversationID string, since time.Time) ([]SyncedMessage, error) {
	path := fmt.Sprintf("%s/reservations/%s/messages", guestyAPIBase, conversationID)

	req, err := http.NewRequestWithContext(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := g.client.Do(ctx, req.Method, req.URL.String(), nil)
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
			Direction string                 `json:"direction"`
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
			if firstName, ok := msg.Sender["firstName"].(string); ok {
				senderName = firstName
			}
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
	path := fmt.Sprintf("%s/reservations?limit=1", guestyAPIBase)

	req, err := http.NewRequestWithContext(ctx, "GET", path, nil)
	if err != nil {
		return err
	}

	resp, err := g.client.Do(ctx, req.Method, req.URL.String(), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed: status %d", resp.StatusCode)
	}

	return nil
}
