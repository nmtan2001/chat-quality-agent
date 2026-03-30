package notifications

import (
	"bytes"
	"fmt"
	"html"
	"regexp"
	"strings"
	"unicode"
)

// Allowed HTML tags for custom templates (for email notifications)
var allowedTags = map[string]bool{
	"a": true, "b": true, "strong": true, "i": true, "em": true,
	"u": true, "br": true, "p": true, "div": true, "span": true,
	"ul": true, "ol": true, "li": true, "h1": true, "h2": true,
	"h3": true, "h4": true, "h5": true, "h6": true,
}

// SanitizeCustomTemplate sanitizes a custom template for security.
// - Removes script tags and event handlers
// - Allows only safe HTML tags
// - Preserves template variables like {{variable}}
func SanitizeCustomTemplate(template string) string {
	if template == "" {
		return ""
	}

	// Remove script tags and their content
	scriptRegex := regexp.MustCompile(`(?i)<script\b[^>]*>.*?</script>`)
	template = scriptRegex.ReplaceAllString(template, "")

	// Remove event handlers (onclick, onerror, etc.)
	eventRegex := regexp.MustCompile(`(?i)\s+on\w+\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)`)
	template = eventRegex.ReplaceAllString(template, "")

	// Remove javascript: protocol links
	javascriptRegex := regexp.MustCompile(`(?i)javascript:[^"'\s]*`)
	template = javascriptRegex.ReplaceAllString(template, "")

	// Protect template variables from HTML escaping
	variableRegex := regexp.MustCompile(`\{\{[a-zA-Z0-9_]+\}\}`)
	variables := variableRegex.FindAllString(template, -1)

	// Create placeholder for each variable
	variableMap := make(map[string]string)
	for i, v := range variables {
		placeholder := fmt.Sprintf("__PLACEHOLDER_%d__", i)
		variableMap[placeholder] = v
		template = strings.ReplaceAll(template, v, placeholder)
	}

	// Sanitize HTML for email (escape user input, keep allowed tags)
	template = sanitizeHTML(template)

	// Restore template variables
	for placeholder, variable := range variableMap {
		template = strings.ReplaceAll(template, placeholder, variable)
	}

	// Limit length to prevent abuse
	if len(template) > 10000 {
		template = template[:10000]
	}

	return template
}

// sanitizeHTML escapes HTML but allows safe tags
func sanitizeHTML(input string) string {
	var buf bytes.Buffer
	var tagBuf bytes.Buffer
	inTag := false
	escapeNext := false

	for _, r := range input {
		switch {
		case r == '<':
			if inTag {
				// Malformed HTML, escape the previous <
				buf.WriteString("&lt;")
			}
			inTag = true
			tagBuf.Reset()
			tagBuf.WriteRune(r)

		case r == '>':
			if inTag {
				tagBuf.WriteRune(r)
				tag := tagBuf.String()
				// Check if it's a closing tag or allowed tag
				if isAllowedTag(tag) {
					buf.WriteString(tag)
				} else {
					// Not allowed, escape the whole thing
					buf.WriteString(html.EscapeString(tag))
				}
				inTag = false
			} else {
				buf.WriteString("&gt;")
			}

		case inTag:
			// Only allow alphanumeric and certain characters in tags
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '/' || r == ' ' || r == '=' || r == '"' || r == '\'' {
				tagBuf.WriteRune(r)
			} else {
				// Invalid character in tag, treat as malformed
				buf.WriteString(html.EscapeString(tagBuf.String()))
				buf.WriteRune(r)
				inTag = false
			}

		default:
			buf.WriteRune(r)
		}
	}

	// Handle unclosed tag
	if inTag {
		buf.WriteString(html.EscapeString(tagBuf.String()))
	}

	return buf.String()
}

// isAllowedTag checks if an HTML tag is in the allowed list
func isAllowedTag(tag string) bool {
	tag = strings.TrimSpace(tag)
	tag = strings.ToLower(tag)

	// Closing tags are allowed if opening tag is allowed
	if strings.HasPrefix(tag, "</") {
		tagName := strings.TrimPrefix(tag, "</")
		tagName = strings.TrimSuffix(tagName, ">")
		return allowedTags[tagName]
	}

	// Opening tags
	tagName := strings.TrimPrefix(tag, "<")
	tagName = strings.TrimSuffix(tagName, ">")
	tagName = strings.Split(tagName, " ")[0] // Handle attributes

	return allowedTags[tagName]
}

// SanitizeForTelegram sanitizes content for Telegram (no HTML allowed)
func SanitizeForTelegram(content string) string {
	// Remove all HTML tags
	htmlRegex := regexp.MustCompile(`<[^>]*>`)
	content = htmlRegex.ReplaceAllString(content, "")

	// Decode HTML entities
	content = html.UnescapeString(content)

	// Limit length
	if len(content) > 4096 {
		content = content[:4093] + "..."
	}

	return content
}
