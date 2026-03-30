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
	ListingID        string            `json:"listing_id"`
	ListingName      string            `json:"listing_name"`
	TotalConversations int             `json:"total_conversations"`
	IssueCount       int               `json:"issue_count"`
	Categories       map[string]int    `json:"categories"`
	LastIssueAt      *time.Time        `json:"last_issue_at"`
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
		ListingID       string    `json:"listing_id"`
		ListingName     string    `json:"listing_name"`
		ConversationCount int      `json:"conversation_count"`
		LastMessageAt   time.Time `json:"last_message_at"`
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
