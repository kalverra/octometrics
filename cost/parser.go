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
	"time"
)

var utf8BOM = []byte{0xef, 0xbb, 0xbf}

// ParseCSV parses a GitHub Actions billing usage report CSV.
// Filters to product="actions" runner minutes (ignoring storage, packages, copilot).
// If orgFilter is non-empty, only rows matching the specified organization are returned.
func ParseCSV(r io.Reader, orgFilter string) ([]*UsageRecord, error) {
	bufReader := bufio.NewReader(r)

	// Strip UTF-8 BOM if present
	prefix, err := bufReader.Peek(3)
	if err == nil && bytes.Equal(prefix, utf8BOM) {
		_, _ = bufReader.Discard(3)
	}

	reader := csv.NewReader(bufReader)
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV header: %w", err)
	}

	colMap := make(map[string]int, len(header))
	for i, col := range header {
		cleanCol := strings.TrimSpace(strings.ToLower(col))
		colMap[cleanCol] = i
	}

	requiredCols := []string{
		"date", "product", "sku", "quantity", "unit_type",
		"applied_cost_per_quantity", "gross_amount", "discount_amount", "net_amount",
		"organization", "repository", "workflow_path",
	}
	for _, req := range requiredCols {
		if _, ok := colMap[req]; !ok {
			return nil, fmt.Errorf("missing required column: %s", req)
		}
	}

	var records []*UsageRecord

	for lineNum := 2; ; lineNum++ {
		row, readErr := reader.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("CSV read error on line %d: %w", lineNum, readErr)
		}

		product := getField(row, colMap["product"])
		if product != "actions" {
			continue
		}

		sku := getField(row, colMap["sku"])
		unitType := getField(row, colMap["unit_type"])
		// Focus on compute minutes; skip storage and data transfer
		if unitType != "minutes" || strings.Contains(sku, "storage") {
			continue
		}

		org := getField(row, colMap["organization"])
		if orgFilter != "" && !strings.EqualFold(org, orgFilter) {
			continue
		}

		dateStr := getField(row, colMap["date"])
		dateVal, _ := time.Parse("2006-01-02", dateStr)

		qty, _ := strconv.ParseFloat(getField(row, colMap["quantity"]), 64)
		appliedCost, _ := strconv.ParseFloat(getField(row, colMap["applied_cost_per_quantity"]), 64)
		gross, _ := strconv.ParseFloat(getField(row, colMap["gross_amount"]), 64)
		discount, _ := strconv.ParseFloat(getField(row, colMap["discount_amount"]), 64)
		net, _ := strconv.ParseFloat(getField(row, colMap["net_amount"]), 64)

		repo := getField(row, colMap["repository"])
		wfPath := getField(row, colMap["workflow_path"])

		runnerType := RunnerTypeGHANative
		if sku == "actions_self_hosted_linux" {
			runnerType = RunnerTypeRunsOn
		}

		records = append(records, &UsageRecord{
			Date:               dateVal,
			Product:            product,
			SKU:                sku,
			Quantity:           qty,
			UnitType:           unitType,
			AppliedCostPerUnit: appliedCost,
			GrossAmount:        gross,
			DiscountAmount:     discount,
			NetAmount:          net,
			Organization:       org,
			Repository:         repo,
			WorkflowPath:       wfPath,
			RunnerType:         runnerType,
		})
	}

	return records, nil
}

func getField(row []string, idx int) string {
	if idx >= 0 && idx < len(row) {
		return strings.TrimSpace(row[idx])
	}
	return ""
}
