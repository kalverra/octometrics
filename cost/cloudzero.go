package cost

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// CloudzeroResult holds parsed Cloudzero Runs-On cost data broken down by repository.
type CloudzeroResult struct {
	RepoCosts    map[string]float64 `json:"repo_costs"`
	UntaggedCost float64            `json:"untagged_cost"`
	TotalCost    float64            `json:"total_cost"`
}

// ParseCloudzeroCSV parses an exported Cloudzero daily cost report CSV.
func ParseCloudzeroCSV(r io.Reader) (*CloudzeroResult, error) {
	bufReader := bufio.NewReader(r)

	// Strip UTF-8 BOM if present
	prefix, err := bufReader.Peek(3)
	if err == nil && bytes.Equal(prefix, utf8BOM) {
		_, _ = bufReader.Discard(3)
	}

	reader := csv.NewReader(bufReader)
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	res := &CloudzeroResult{
		RepoCosts: make(map[string]float64),
	}

	for lineNum := 1; ; lineNum++ {
		row, readErr := reader.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("CSV read error on line %d: %w", lineNum, readErr)
		}
		if len(row) == 0 {
			continue
		}

		firstCol := strings.TrimSpace(row[0])
		if firstCol == "" || strings.HasPrefix(firstCol, "#") || strings.HasPrefix(firstCol, "tag:") {
			continue
		}

		// Calculate sum of all date columns
		var rowSum float64
		for _, col := range row[1:] {
			valStr := strings.TrimSpace(col)
			if valStr == "" {
				continue
			}
			val, parseErr := strconv.ParseFloat(valStr, 64)
			if parseErr == nil {
				rowSum += val
			}
		}

		res.TotalCost += rowSum

		if strings.Contains(firstCol, "Without The Specified Tag") {
			res.UntaggedCost += rowSum
			continue
		}

		// Extract repo name (strip org/ prefix if present)
		repo := firstCol
		if slashIdx := strings.LastIndex(repo, "/"); slashIdx >= 0 {
			repo = repo[slashIdx+1:]
		}
		res.RepoCosts[repo] += rowSum
	}

	return res, nil
}
