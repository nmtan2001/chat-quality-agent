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
func BuildPropertyAnalysisPrompt(transcript string) string {
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

	return fmt.Sprintf("%s\n\nConversations:\n%s", prompt, transcript)
}

// BuildUrgencyDetectionPrompt creates a prompt for detecting urgent issues in guest messages.
func BuildUrgencyDetectionPrompt() string {
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
