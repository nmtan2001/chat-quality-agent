package ai

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// BuildQualityMetricsPromptEnhanced creates an optimized prompt for analyzing response quality.
// Improvements: Few-shot examples, confidence scoring, multilingual support, token optimization.
func BuildQualityMetricsPromptEnhanced() string {
	return `Analyze vacation rental conversation quality. Return JSON:

{
  "first_response_time_minutes": <int|0>,
  "resolution_time_minutes": <int|0>,
  "message_count_agent": <int>,
  "message_count_guest": <int>,
  "guest_satisfaction": "positive|neutral|negative",
  "agent_professionalism_score": <1-5>,
  "issue_resolved": <bool>,
  "confidence": <0.0-1.0>,
  "summary": "<1-2 sentences>"
}

RULES:
- first_response_time_minutes: guest msg1 → agent msg1 (0 if N/A)
- resolution_time_minutes: issue mention → resolution (0 if unresolved)
- guest_satisfaction: final tone (positive=thankful, neutral=neutral, negative=complaining)
- agent_professionalism_score: 1=poor, 5=excellent (tone, grammar, helpfulness)
- confidence: assessment certainty (low <0.6, medium 0.6-0.8, high >0.8)

EXAMPLES:
Input: "Where's the key?" → "Under mat" 5min later
Output: {"first_response_time_minutes":5,"resolution_time_minutes":5,"guest_satisfaction":"positive","agent_professionalism_score":5,"issue_resolved":true,"confidence":1.0,"summary":"Quick key resolution"}

Input: "AC broken" → 24h silence → "Checking" → no fix
Output: {"first_response_time_minutes":1440,"resolution_time_minutes":0,"guest_satisfaction":"negative","agent_professionalism_score":2,"issue_resolved":false,"confidence":0.9,"summary":"Slow unresolved AC issue"}

Edge cases:
- Missing timestamps → use 0
- Ambiguous satisfaction → confidence < 0.7, neutral
- Multiple issues → resolution_time = last resolved

ONLY valid JSON. No markdown.`
}

// BuildPropertyAnalysisPromptEnhanced creates an optimized prompt for property-level analysis.
// Improvements: Better structure definition, confidence metrics, actionable recommendations.
func BuildPropertyAnalysisPromptEnhanced(transcript string) string {
	prompt := `Identify property-level patterns from guest conversations. Return JSON:

{
  "total_conversations": <int>,
  "issues_by_category": {
    "cleaning": <int>,
    "maintenance": <int>,
    "noise": <int>,
    "amenities": <int>,
    "other": <int>
  },
  "recurring_issues": [
    {"issue": "<desc>", "frequency": <int>, "severity": "high|medium|low", "confidence": <0.0-1.0>}
  ],
  "recommendations": ["<actionable>"],
  "analysis_confidence": <0.0-1.0>
}

SEVERITY GUIDE:
- high: affects stay/safety (water, power, locks)
- medium: affects comfort (noise, weak WiFi)
- low: minor inconveniences

EXAMPLE:
Input: 5 convs mentioning "no hot water", 2 "dirty towels", 1 "noisy"
Output: {"total_conversations":8,"issues_by_category":{"cleaning":2,"maintenance":5,"noise":1,"amenities":0,"other":0},"recurring_issues":[{"issue":"Hot water failure","frequency":5,"severity":"high","confidence":0.95}],"recommendations":["Inspect water heater immediately","Check towel inventory"],"analysis_confidence":0.9}

ONLY valid JSON.`

	return fmt.Sprintf("%s\n\nConversations:\n%s", prompt, transcript)
}

// BuildUrgencyDetectionPromptEnhanced creates an optimized prompt for detecting urgent issues.
// Improvements: Few-shot examples, confidence scoring, multilingual detection, better categorization.
func BuildUrgencyDetectionPromptEnhanced() string {
	return `Detect urgent vacation rental issues. Return JSON:

{
  "is_urgent": <bool>,
  "category": "CLEANING|MAINTENANCE|PAYMENT|SERVICE_REQUEST|SECURITY|NOISE|OTHER",
  "severity": "high|medium|low",
  "confidence": <0.0-1.0>,
  "summary": "<1 sentence>"
}

CATEGORIES:
- CLEANING: dirty, pests, trash, linens, bathroom
- MAINTENANCE: water, AC/heat, leaks, appliances, power
- PAYMENT: refuses to pay, disputes, extra charges
- SERVICE_REQUEST: early check-in, late checkout, amenities
- SECURITY: locks, safety, unauthorized access
- NOISE: neighbors, construction
- OTHER: urgent but unclassified

URGENCY INDICATORS:
- broken/not working/dirty/failure
- immediate action needed/asap/urgent
- refusing payment/charge dispute
- strong frustration/threats to leave review
- emergency/critical/serious

EXAMPLES:
Input: "No hot water, shower freezing!"
Output: {"is_urgent":true,"category":"MAINTENANCE","severity":"high","confidence":1.0,"summary":"No hot water in shower"}

Input: "Can we check in 2 hours early?"
Output: {"is_urgent":true,"category":"SERVICE_REQUEST","severity":"low","confidence":0.9,"summary":"Early check-in request"}

Input: "Great place, thanks!"
Output: {"is_urgent":false,"category":"OTHER","severity":"low","confidence":0.95,"summary":"Positive feedback, no issues"}

Input: "The toilet doesn't flush and there's a weird smell"
Output: {"is_urgent":true,"category":"CLEANING","severity":"high","confidence":0.85,"summary":"Toilet malfunction + odor"}

MULTILINGUAL: Detect urgency in any language using context clues (tone, keywords, exclamation marks).

EDGE CASES:
- Ambiguous → confidence < 0.7, default false
- Multiple issues → highest severity, PRIMARY category
- Vague complaints → confidence < 0.6

ONLY valid JSON. No markdown.`
}

// BuildUrgencyDetectionPromptMultilingual adds language detection to urgency detection.
func BuildUrgencyDetectionPromptMultilingual(detectedLanguage string) string {
	base := BuildUrgencyDetectionPromptEnhanced()

	if detectedLanguage == "" || detectedLanguage == "en" {
		return base
	}

	// Add language-specific guidance
	langHints := map[string]string{
		"es": `SPANISH KEYWORDS: urgente, emergencia, no funciona, roto, sucio, asap, inmediatamente, problema`,
		"fr": `FRENCH KEYWORDS: urgence,urgence, ne marche pas, cassé, sale, problème, immédiat`,
		"de": `GERMAN KEYWORDS: dringend, notfall, funktioniert nicht, kaputt, schmutzig, sofort, problem`,
		"pt": `PORTUGUESE KEYWORDS: urgente, emergência, não funciona, quebrado, sujo, asap, problema`,
		"zh": `CHINESE KEYWORDS: 紧急, 坏了, 脏, 不工作, 问题, 马上`,
		"ja": `JAPANESE KEYWORDS: 緊急, 壊れた, 汚い, すぐ, 問題`,
		"ko": `KOREAN KEYWORDS: 긴급, 고장, 더러워, 당장, 문제`,
	}

	if hint, ok := langHints[detectedLanguage]; ok {
		return base + fmt.Sprintf("\n\nDETECTED LANGUAGE: %s\n%s", detectedLanguage, hint)
	}

	return base
}

// ParseAIResponseWithRetry provides robust JSON parsing with fallback handling.
func ParseAIResponseWithRetry(response string, target interface{}) error {
	// First attempt: direct parse
	if err := parseJSON(response, target); err == nil {
		return nil
	}

	// Clean common issues:
	// 1. Remove markdown code blocks
	cleaned := removeMarkdownBlocks(response)

	// 2. Fix common JSON errors (trailing commas, quotes, etc.)
	cleaned = fixCommonJSONErrors(cleaned)

	// Second attempt: cleaned parse
	if err := parseJSON(cleaned, target); err == nil {
		return nil
	}

	// Third attempt: extract JSON from mixed content
	if extracted := extractJSON(response); extracted != "" {
		if err := parseJSON(extracted, target); err == nil {
			return nil
		}
	}

	return fmt.Errorf("failed to parse AI response after 3 attempts")
}

// parseJSON attempts to parse a string as JSON.
func parseJSON(s string, target interface{}) error {
	decoder := json.NewDecoder(strings.NewReader(s))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

// removeMarkdownBlocks removes ```json and ``` markers.
func removeMarkdownBlocks(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

// fixCommonJSONErrors fixes common AI-generated JSON issues.
func fixCommonJSONErrors(s string) string {
	// Remove trailing commas before closing brackets/braces
	re := regexp.MustCompile(`,\s*([}\]])`)
	s = re.ReplaceAllString(s, "$1")

	// Fix unquoted property names (basic cases)
	re = regexp.MustCompile(`([{,]\s*)([a-zA-Z_][a-zA-Z0-9_]*)(\s*:)`)
	s = re.ReplaceAllString(s, "$1\"$2\"$3")

	return s
}

// extractJSON attempts to find JSON within mixed content.
func extractJSON(s string) string {
	// Find first { and last }
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")

	if start >= 0 && end > start {
		return s[start : end+1]
	}

	// Find first [ and last ]
	start = strings.Index(s, "[")
	end = strings.LastIndex(s, "]")

	if start >= 0 && end > start {
		return s[start : end+1]
	}

	return ""
}

// CalculatePromptTokenSavings estimates token reduction from optimization.
func CalculatePromptTokenSavings(original, enhanced string) float64 {
	// Rough estimate: ~4 chars per token
	originalTokens := len(original) / 4
	enhancedTokens := len(enhanced) / 4

	if originalTokens == 0 {
		return 0
	}

	reduction := (float64(originalTokens-enhancedTokens) / float64(originalTokens)) * 100
	return reduction
}
