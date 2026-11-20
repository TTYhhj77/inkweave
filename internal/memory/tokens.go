package memory

import "strings"

// EstimateTokens provides fast, zero-dependency token estimation calibrated for
// both Thai/CJK (dense subword tokenization ~1.5-1.7 chars/token) and English (~4 chars/token).
func EstimateTokens(text string) int {
	if strings.TrimSpace(text) == "" {
		return 0
	}
	var thaiOrCJK int
	var latinOrOther int
	for _, r := range text {
		switch {
		case r >= 0x0E00 && r <= 0x0E7F: // Thai block
			thaiOrCJK++
		case (r >= 0x4E00 && r <= 0x9FFF) || (r >= 0x3040 && r <= 0x30FF) || (r >= 0xAC00 && r <= 0xD7AF):
			thaiOrCJK++
		default:
			latinOrOther++
		}
	}
	// Thai typically maps ~1.55 runes per LLM token; English averages ~3.8 runes per token.
	thaiTok := (thaiOrCJK*10 + 15) / 16
	latinTok := (latinOrOther + 3) / 4
	total := thaiTok + latinTok
	if total == 0 && len(text) > 0 {
		return 1
	}
	return total
}

// TailByTokens returns the trailing suffix of text fitting inside maxTokens,
// preferring clean paragraph or sentence boundaries when possible.
func TailByTokens(text string, maxTokens int) string {
	text = strings.TrimSpace(text)
	if text == "" || maxTokens <= 0 {
		return ""
	}
	if EstimateTokens(text) <= maxTokens {
		return text
	}
	paragraphs := strings.Split(text, "\n\n")
	if len(paragraphs) > 1 {
		var kept []string
		used := 0
		for i := len(paragraphs) - 1; i >= 0; i-- {
			p := strings.TrimSpace(paragraphs[i])
			if p == "" {
				continue
			}
			t := EstimateTokens(p) + 2
			if used+t > maxTokens && len(kept) > 0 {
				break
			}
			if used+t > maxTokens && len(kept) == 0 {
				return tailRunes(p, maxTokens)
			}
			kept = append([]string{p}, kept...)
			used += t
		}
		if len(kept) > 0 {
			return strings.Join(kept, "\n\n")
		}
	}
	return tailRunes(text, maxTokens)
}

func tailRunes(text string, maxTokens int) string {
	runes := []rune(text)
	// Binary search the longest trailing rune slice <= maxTokens
	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi) / 2
		candidate := string(runes[mid:])
		if EstimateTokens(candidate) <= maxTokens {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return strings.TrimSpace(string(runes[lo:]))
}

// TrimToTokens truncates text from the end to fit within maxTokens.
func TrimToTokens(text string, maxTokens int) string {
	text = strings.TrimSpace(text)
	if text == "" || maxTokens <= 0 {
		return ""
	}
	if EstimateTokens(text) <= maxTokens {
		return text
	}
	runes := []rune(text)
	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if EstimateTokens(string(runes[:mid])) <= maxTokens {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	if lo == 0 {
		return ""
	}
	return strings.TrimSpace(string(runes[:lo])) + "…"
}
