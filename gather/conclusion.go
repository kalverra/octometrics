package gather

var conclusionPriority = map[string]int{
	"failure":         100,
	"timed_out":       90,
	"cancelled":       80,
	"action_required": 70,
	"stale":           60,
	"in_progress":     50,
	"neutral":         40,
	"success":         30,
	"skipped":         20,
}

// establishPRChecksConclusion returns the higher-priority workflow conclusion.
func establishPRChecksConclusion(current, newStatus string) string {
	if conclusionPriority[newStatus] > conclusionPriority[current] {
		return newStatus
	}
	return current
}
