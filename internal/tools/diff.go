package tools

import (
	"strings"

	"github.com/inkweave/inkweave/internal/model"
)

// ComputeLineDiff produces structured line-level diff hunks between before and after text
// using an LCS DP table (capped cleanly for large chapters).
func ComputeLineDiff(before, after string) []model.DiffHunk {
	a := splitLines(before)
	b := splitLines(after)

	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	if len(a) == 0 {
		return []model.DiffHunk{{Type: "insert", Text: after}}
	}
	if len(b) == 0 {
		return []model.DiffHunk{{Type: "delete", Text: before}}
	}

	// Guard against quadratic blowup on huge inputs by falling back to coarse block diff
	if len(a)*len(b) > 250000 {
		return []model.DiffHunk{
			{Type: "delete", Text: before},
			{Type: "insert", Text: after},
		}
	}

	m, n := len(a), len(b)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	var raw []model.DiffHunk
	i, j := 0, 0
	for i < m && j < n {
		if a[i] == b[j] {
			raw = append(raw, model.DiffHunk{Type: "equal", Text: a[i]})
			i++
			j++
		} else if dp[i+1][j] >= dp[i][j+1] {
			raw = append(raw, model.DiffHunk{Type: "delete", Text: a[i]})
			i++
		} else {
			raw = append(raw, model.DiffHunk{Type: "insert", Text: b[j]})
			j++
		}
	}
	for ; i < m; i++ {
		raw = append(raw, model.DiffHunk{Type: "delete", Text: a[i]})
	}
	for ; j < n; j++ {
		raw = append(raw, model.DiffHunk{Type: "insert", Text: b[j]})
	}

	// Coalesce adjacent hunks of the same type
	var merged []model.DiffHunk
	for _, h := range raw {
		if len(merged) > 0 && merged[len(merged)-1].Type == h.Type {
			merged[len(merged)-1].Text += "\n" + h.Text
		} else {
			merged = append(merged, h)
		}
	}
	return merged
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
