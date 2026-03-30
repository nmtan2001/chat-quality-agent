# Guesty Integration Guide

## Overview
The Guesty integration enables real-time monitoring of Airbnb/Booking guest messages for urgent issue detection, quality control, and property analytics.

## Architecture

- **Real-time Pipeline:** Guesty webhook → Svix verification → Urgent issue detection (AI) → Instant alerts (Telegram/Email)
- **Quality Control:** Hourly batch job analyzes response times and conversation quality
- **Analytics:** Daily aggregation of issues by property/listing for operational insights

## Setup

### 1. Guesty API Credentials

1. Go to [Guesty Dashboard](https://dashboard.guesty.com/)
2. Navigate to **Integrations** → **API**
3. Create OAuth credentials (client_id, client_secret)
4. Set environment variables:
   ```bash
   GUESTY_CLIENT_ID=your_client_id
   GUESTY_CLIENT_SECRET=your_client_secret
   ```

### 2. Svix Webhook Configuration

1. In Guesty Dashboard, create a webhook pointing to:
   ```
   https://your-domain.com/api/v1/webhooks/guesty
   ```

2. Set the `SVIX_SECRET` environment variable:
   ```bash
   SVIX_SECRET=your_svix_webhook_secret_here
   ```

3. Configure events to subscribe:
   - `reservation.messageReceived` - Incoming guest messages
   - `reservation.messageSent` - Outgoing host messages

### 3. Channel Setup

1. Go to **Settings** → **Channels** → **Add Channel**
2. Select **Guesty** from the channel type dropdown
3. Enter your **Guesty Account ID** (found in Guesty Dashboard under Account Settings)
4. Save

The system will use the globally configured Guesty API credentials for API calls.

## Features

### Real-time Urgent Issue Detection

Automatic detection of urgent guest issues:

- **CLEANING:** Dirty rooms, bathroom issues, pests, trash, linen problems
- **MAINTENANCE:** No hot water, AC/heat not working, leaks, broken appliances
- **PAYMENT:** Guest refuses to pay, payment disputes, extra charges
- **SERVICE_REQUEST:** Early check-in, late check-out, extra amenities
- **SECURITY:** Locks not working, safety concerns
- **NOISE:** Noise complaints from neighbors or construction
- **OTHER:** Issues requiring immediate attention

When urgent issues are detected:
- Creates notification log entry
- Sends instant alert via configured channels (Telegram/Email)
- Tags conversation with urgency category and severity

### Quality Control Metrics

Analyzed metrics for each conversation:

- **First Response Time:** Time from first guest message to first agent response
- **Resolution Time:** Time from issue mention to resolution
- **Message Counts:** Agent and guest message counts
- **Guest Satisfaction:** Positive/neutral/negative sentiment analysis
- **Agent Professionalism Score:** 1-5 rating based on tone and helpfulness
- **Issue Resolved:** Whether the guest's issue was addressed

### Property Analytics

Aggregated statistics by property/listing:

- Total conversations per property
- Issue count by category
- Last issue timestamp
- Recurring issue identification
- Trend analysis over time

## API Endpoints

### Webhook
- `POST /api/v1/webhooks/guesty` - Receives Guesty webhook events
- `GET /api/v1/webhooks/guesty` - Webhook verification challenge

### Analytics
- `GET /api/v1/tenants/:tenantId/analytics/guesty` - Property analytics
  - Query params: `start_date` (YYYY-MM-DD), `end_date` (YYYY-MM-DD)
  - Returns issue statistics grouped by listing

## Environment Variables

Required:
```bash
GUESTY_CLIENT_ID=your_client_id
GUESTY_CLIENT_SECRET=your_client_secret
SVIX_SECRET=your_svix_secret
JWT_SECRET=your_jwt_secret_min_32_chars
ENCRYPTION_KEY=exactly_32_bytes_for_aes_256_gcm
```

Optional:
```bash
SVIX_SECRET=your_svix_webhook_secret_here
```

## Database Schema

### Channel Metadata (JSONB)
```json
{
  "account_id": "guesty-account-123",
  "sync_files": false,
  "sync_interval": 15
}
```

### Conversation Metadata (JSONB)
```json
{
  "reservation_id": "reservation-123",
  "guest_name": "John Doe",
  "check_in": "2024-03-15",
  "check_out": "2024-03-20",
  "listing_id": "listing-456",
  "listing_name": "Beach House"
}
```

## Troubleshooting

### Webhook Not Receiving Events

1. Check Guesty dashboard for webhook delivery status
2. Verify `SVIX_SECRET` is set correctly
3. Check server logs for signature verification errors
4. Ensure firewall allows incoming webhooks

### Messages Not Syncing

1. Verify Guesty API credentials are valid
2. Check server logs for authentication errors
3. Ensure channel is active (`is_active = true`)

### Urgent Issues Not Detected

1. Verify AI settings are configured
2. Check AI provider API key is valid
3. Review notification logs for errors
4. Test AI provider connectivity in Settings

## Migration

The system includes database migrations for Guesty-specific indexes:

```bash
# Run migrations (automatic on first start)
# Or manually via API
POST /api/v1/tenants/:tenantId/migrations/guesty-indexes
```

## Security Considerations

- All webhook signatures are verified using HMAC-SHA256
- Timestamp verification prevents replay attacks (5-minute tolerance)
- OAuth tokens are automatically refreshed when expired
- Sensitive credentials are encrypted at rest using AES-256-GCM
