// Token accounting: the provider's usage when it reports one, and a
// character-based estimate when it does not (docs/AI-功能规划与设计.md §F7).
package ai

import (
	"math"
	"unicode"
)

// EstimateTokens approximates a text's token count without a tokenizer:
// about 2.5 characters per token for CJK scripts and 4 for Latin text.
func EstimateTokens(text string) int {
	cjk, other := 0, 0
	for _, r := range text {
		if IsCJK(r) {
			cjk++
		} else {
			other++
		}
	}
	tokens := 0
	if cjk > 0 {
		tokens += int(math.Ceil(float64(cjk) / 2.5))
	}
	tokens += (other + 3) / 4
	return tokens
}

// IsCJK reports whether the rune belongs to a CJK block.
func IsCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hangul, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r)
}

// EstimateMessages approximates the request's prompt tokens, roles and the
// per-message framing included.
func EstimateMessages(msgs []Message) int {
	total := 0
	for _, m := range msgs {
		total += 4 + EstimateTokens(m.Content) + EstimateTokens(m.ToolCallID)
		for _, tc := range m.ToolCalls {
			total += 4 + EstimateTokens(tc.Name) + EstimateTokens(tc.Args)
		}
	}
	return total
}
