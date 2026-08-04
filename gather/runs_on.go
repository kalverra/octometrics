package gather

import (
	"math"
	"regexp"
	"time"
)

// runsOnRates maps runs-on runner labels to per-minute rates in tenths-of-cent.
// Rates are spot prices (us-east-1, incl. 30GB gp3 @ 400 MB/s) from runs-on.com/pricing.
// These are estimates; actual AWS billing varies by region, spot market, and storage.
var runsOnRates = map[string]int64{
	// x64 intel/amd (auto generation)
	"1cpu-linux-x64":    6,
	"2cpu-linux-x64":    10,
	"4cpu-linux-x64":    17,
	"8cpu-linux-x64":    27,
	"16cpu-linux-x64":   43,
	"32cpu-linux-x64":   92,
	"48cpu-linux-x64":   88,
	"64cpu-linux-x64":   133,
	"96cpu-linux-x64":   223,
	"1cpu-windows-x64":  20,
	"2cpu-windows-x64":  20,
	"4cpu-windows-x64":  38,
	"8cpu-windows-x64":  71,
	"16cpu-windows-x64": 138,
	"32cpu-windows-x64": 273,
	"64cpu-windows-x64": 544,
	"96cpu-windows-x64": 814,

	// x64 intel/amd (specific generations)
	"2cpu-linux-x64-6":     9,
	"4cpu-linux-x64-6":     13,
	"8cpu-linux-x64-6":     30,
	"16cpu-linux-x64-6":    54,
	"32cpu-linux-x64-6":    79,
	"64cpu-linux-x64-6":    135,
	"96cpu-linux-x64-6":    195,
	"2cpu-linux-x64-7a":    8,
	"4cpu-linux-x64-7a":    14,
	"8cpu-linux-x64-7a":    27,
	"16cpu-linux-x64-7a":   54,
	"32cpu-linux-x64-7a":   98,
	"64cpu-linux-x64-7a":   184,
	"96cpu-linux-x64-7a":   253,
	"2cpu-linux-x64-7i":    8,
	"4cpu-linux-x64-7i":    13,
	"8cpu-linux-x64-7i":    29,
	"16cpu-linux-x64-7i":   51,
	"32cpu-linux-x64-7i":   108,
	"64cpu-linux-x64-7i":   141,
	"96cpu-linux-x64-7i":   218,
	"2cpu-linux-x64-8a":    9,
	"4cpu-linux-x64-8a":    16,
	"8cpu-linux-x64-8a":    35,
	"16cpu-linux-x64-8a":   63,
	"32cpu-linux-x64-8a":   122,
	"64cpu-linux-x64-8a":   154,
	"96cpu-linux-x64-8a":   279,
	"2cpu-linux-x64-8i":    9,
	"4cpu-linux-x64-8i":    15,
	"8cpu-linux-x64-8i":    32,
	"16cpu-linux-x64-8i":   38,
	"32cpu-linux-x64-8i":   91,
	"64cpu-linux-x64-8i":   169,
	"96cpu-linux-x64-8i":   217,
	"1cpu-linux-x64-8azn":  8,
	"2cpu-linux-x64-8azn":  12,
	"4cpu-linux-x64-8azn":  23,
	"48cpu-linux-x64-8azn": 272,
	"96cpu-linux-x64-8azn": 275,

	// arm64 (graviton)
	"1cpu-linux-arm64":  5,
	"2cpu-linux-arm64":  9,
	"4cpu-linux-arm64":  14,
	"8cpu-linux-arm64":  23,
	"16cpu-linux-arm64": 43,
	"32cpu-linux-arm64": 66,
	"48cpu-linux-arm64": 89,
	"64cpu-linux-arm64": 114,
	"96cpu-linux-arm64": 176,

	// arm64 specific generations
	"2cpu-linux-arm64-7g":  7,
	"4cpu-linux-arm64-7g":  12,
	"8cpu-linux-arm64-7g":  23,
	"16cpu-linux-arm64-7g": 36,
	"32cpu-linux-arm64-7g": 61,
	"64cpu-linux-arm64-7g": 110,
	"2cpu-linux-arm64-8g":  7,
	"4cpu-linux-arm64-8g":  13,
	"8cpu-linux-arm64-8g":  23,
	"16cpu-linux-arm64-8g": 43,
	"32cpu-linux-arm64-8g": 66,
	"64cpu-linux-arm64-8g": 122,
	"96cpu-linux-arm64-8g": 163,
	"1cpu-linux-arm64-8g":  5,
	"2cpu-linux-arm64-9g":  9,
	"4cpu-linux-arm64-9g":  14,
	"8cpu-linux-arm64-9g":  26,
	"16cpu-linux-arm64-9g": 49,
	"32cpu-linux-arm64-9g": 97,
	"64cpu-linux-arm64-9g": 188,
	"96cpu-linux-arm64-9g": 287,

	// GPU
	"4cpu-linux-x64-gpu-t4":    41,
	"8cpu-linux-x64-gpu-t4":    50,
	"16cpu-linux-x64-gpu-a10":  128,
	"48cpu-linux-x64-gpu-4xt4": 210,
}

// runsOnDocsPattern matches the docs-style label "2cpu-linux-x64" or "runner=2cpu-linux-x64".
var runsOnDocsPattern = regexp.MustCompile(`^(?:runner=)?(\d+)cpu-(linux|windows|macos)-(x64|arm64)(?:-\w+)?$`)

// runsOnRealPattern matches the real-world label "runs-on=<id>/cpu=N/...".
var runsOnRealPattern = regexp.MustCompile(`^runs-on=`)

// cpuFromLabelPattern extracts cpu=N from the real runs-on label format.
var cpuFromLabelPattern = regexp.MustCompile(`cpu=(\d+)`)

// archFromLabelPattern extracts arch from image= in the real runs-on label format.
var archFromLabelPattern = regexp.MustCompile(`image=\w+-?(x64|arm64)`)

// parseRunsOnLabel detects whether a job uses runs-on and extracts a key for rate lookup.
// Supports two label formats:
//   - Docs format: "2cpu-linux-x64" or "runner=2cpu-linux-x64"
//   - Real format: "runs-on=<run-id>/cpu=8/ram=32/family=m6i/spot=false/image=ubuntu24-full-x64/extras=..."
//
// Returns a normalized key and true if the job uses runs-on.
func parseRunsOnLabel(labels []string) (string, bool) {
	for _, label := range labels {
		// Try docs format first
		if matches := runsOnDocsPattern.FindStringSubmatch(label); len(matches) > 0 {
			return matches[1] + "cpu-" + matches[2] + "-" + matches[3], true
		}

		// Try real runs-on format: "runs-on=..."
		if runsOnRealPattern.MatchString(label) {
			return parseRealRunsOnLabel(label), true
		}
	}
	return "", false
}

// parseRealRunsOnLabel extracts a rate-lookup key from a real runs-on label.
// Format: runs-on=<id>/cpu=N/ram=N/family=X/spot=bool/image=.../extras=...
// Falls back to "Ncpu-linux-x64" style key using cpu count and arch from image.
func parseRealRunsOnLabel(label string) string {
	cpuMatches := cpuFromLabelPattern.FindStringSubmatch(label)
	if len(cpuMatches) < 2 {
		return label
	}
	cpu := cpuMatches[1]

	arch := "x64"
	if archMatches := archFromLabelPattern.FindStringSubmatch(label); len(archMatches) > 1 {
		arch = archMatches[1]
	}

	return cpu + "cpu-linux-" + arch
}

// runsOnRate returns the per-minute rate in tenths-of-cent for a runs-on runner key.
func runsOnRate(key string) (int64, bool) {
	rate, ok := runsOnRates[key]
	return rate, ok
}

// calculateRunsOnCost computes the estimated cost for a runs-on runner.
// Cost is based on per-second AWS billing (no minute rounding like GitHub-hosted).
// Returns (cost in tenths-of-cent, isEstimate).
func calculateRunsOnCost(labels []string, duration time.Duration) (int64, bool) {
	key, ok := parseRunsOnLabel(labels)
	if !ok {
		return 0, false
	}

	rate, ok := runsOnRate(key)
	if !ok {
		return 0, false
	}

	if duration <= 0 {
		return 0, false
	}

	// Per-second billing: ceil(duration_seconds / 60) * rate
	durationMinutes := duration.Seconds() / 60.0
	cost := int64(math.Ceil(durationMinutes * float64(rate)))
	if cost == 0 {
		cost = rate
	}

	return cost, true
}

// runsOnRunnerName returns a human-readable name for the runs-on runner.
func runsOnRunnerName(labels []string) string {
	key, ok := parseRunsOnLabel(labels)
	if !ok {
		return ""
	}
	return "runs-on:" + key
}
