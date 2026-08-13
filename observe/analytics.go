package observe

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/kalverra/octometrics/gather"
)

// CriticalPathNode represents a job node within the workflow execution DAG.
type CriticalPathNode struct {
	JobID        int64         `json:"job_id"`
	JobName      string        `json:"job_name"`
	Duration     time.Duration `json:"duration"`
	QueueTime    time.Duration `json:"queue_time"`
	StartTime    time.Time     `json:"start_time"`
	CompletedAt  time.Time     `json:"completed_at"`
	Slack        time.Duration `json:"slack"`
	IsCritical   bool          `json:"is_critical"`
	BlockingNeed string        `json:"blocking_need,omitempty"`
}

// CriticalPathInfo holds the analysis of critical path and job slack across a workflow run.
type CriticalPathInfo struct {
	Nodes              []CriticalPathNode `json:"nodes"`
	CriticalNodes      []CriticalPathNode `json:"critical_nodes"`
	NearCriticalNodes  []CriticalPathNode `json:"near_critical_nodes"`
	TotalDuration      time.Duration      `json:"total_duration"`
	TotalExecution     time.Duration      `json:"total_execution"`
	TotalQueue         time.Duration      `json:"total_queue"`
	MedianQueueFinding string             `json:"median_queue_finding,omitempty"`
}

// StepSummary aggregates step durations with identical names across jobs.
type StepSummary struct {
	Name           string        `json:"name"`
	Count          int           `json:"count"`
	TotalDuration  time.Duration `json:"total_duration"`
	MinDuration    time.Duration `json:"min_duration"`
	MedianDuration time.Duration `json:"median_duration"`
	MaxDuration    time.Duration `json:"max_duration"`
	PctTotal       float64       `json:"pct_total"`
}

// StepDetail describes a single step timing within a job.
type StepDetail struct {
	Name       string        `json:"name"`
	Duration   time.Duration `json:"duration"`
	Status     string        `json:"status"`
	Conclusion string        `json:"conclusion"`
}

// JobStepBreakdown holds step-level timing breakdown for a specific job.
type JobStepBreakdown struct {
	JobID           int64         `json:"job_id"`
	JobName         string        `json:"job_name"`
	Duration        time.Duration `json:"duration"`
	Steps           []StepDetail  `json:"steps"`
	MinorStepsCount int           `json:"minor_steps_count"`
	MinorStepsTotal time.Duration `json:"minor_steps_total"`
}

// HasNonSuccessSteps reports whether any step has a non-success conclusion.
func (j JobStepBreakdown) HasNonSuccessSteps() bool {
	for _, s := range j.Steps {
		if s.Conclusion != "" && s.Conclusion != "success" {
			return true
		}
	}
	return false
}

// CalculateCriticalPath computes the critical path and slack for all jobs in a workflow run.
func CalculateCriticalPath(jobs []*gather.JobData, def *gather.WorkflowDef) *CriticalPathInfo {
	if len(jobs) == 0 {
		return nil
	}

	runStart, runEnd := getRunTimeBounds(jobs)
	if runStart.IsZero() || runEnd.IsZero() {
		return nil
	}

	needsMap := buildNeedsMap(def)
	nodes, criticalNodes, nearCriticalNodes := analyzeJobNodes(jobs, needsMap, runEnd)
	if len(nodes) == 0 {
		return nil
	}

	var (
		totalExec  time.Duration
		totalQueue time.Duration
		queues     []time.Duration
	)
	for _, n := range criticalNodes {
		totalExec += n.Duration
		totalQueue += n.QueueTime
	}

	for _, j := range jobs {
		if j != nil && !j.GetCreatedAt().IsZero() && !j.GetStartedAt().IsZero() && j.GetConclusion() != "skipped" {
			if j.GetStartedAt().After(j.GetCreatedAt().Time) {
				queues = append(queues, j.GetStartedAt().Sub(j.GetCreatedAt().Time))
			}
		}
	}

	var finding string
	if len(queues) > 0 {
		slices.Sort(queues)
		medianQueue := queues[len(queues)/2]
		//nolint:staticcheck,revive // zero duration initialization
		totalRunnerSecs := time.Duration(0)
		for range queues {
			totalRunnerSecs += medianQueue
		}
		finding = fmt.Sprintf("Median queue %s × %d jobs ≈ %s runner-time waiting.",
			medianQueue.Round(time.Second), len(queues), totalRunnerSecs.Round(time.Minute))
	}

	return &CriticalPathInfo{
		Nodes:              nodes,
		CriticalNodes:      criticalNodes,
		NearCriticalNodes:  nearCriticalNodes,
		TotalDuration:      runEnd.Sub(runStart),
		TotalExecution:     totalExec,
		TotalQueue:         totalQueue,
		MedianQueueFinding: finding,
	}
}

func getRunTimeBounds(jobs []*gather.JobData) (runStart, runEnd time.Time) {
	first := true
	for _, j := range jobs {
		if j == nil || j.GetStartedAt().IsZero() || j.GetConclusion() == "skipped" {
			continue
		}
		start := j.GetStartedAt().Time
		end := j.GetCompletedAt().Time
		if end.IsZero() {
			end = start.Add(time.Second)
		}
		if first || start.Before(runStart) {
			runStart = start
		}
		if first || end.After(runEnd) {
			runEnd = end
		}
		first = false
	}
	return runStart, runEnd
}

func buildNeedsMap(def *gather.WorkflowDef) map[string][]string {
	needsMap := make(map[string][]string)
	if def == nil {
		return needsMap
	}
	for key, jobDef := range def.Jobs {
		keyTrim := strings.TrimSpace(key)
		nameTrim := strings.TrimSpace(jobDef.Name)
		if nameTrim == "" {
			nameTrim = keyTrim
		}
		rawNeeds := make([]string, len(jobDef.Needs))
		for i, n := range jobDef.Needs {
			rawNeeds[i] = strings.TrimSpace(n)
		}

		needsMap[nameTrim] = rawNeeds
		needsMap[keyTrim] = rawNeeds

		for i, needKey := range jobDef.Needs {
			needKeyTrim := strings.TrimSpace(needKey)
			if targetDef, ok := def.Jobs[needKeyTrim]; ok && targetDef.Name != "" {
				needsMap[nameTrim][i] = strings.TrimSpace(targetDef.Name)
			}
		}
	}
	return needsMap
}

func analyzeJobNodes(
	jobs []*gather.JobData,
	needsMap map[string][]string,
	runEnd time.Time,
) (nodes, criticalNodes, nearCriticalNodes []CriticalPathNode) {
	nodes = buildJobNodes(jobs, needsMap)
	if len(nodes) == 0 {
		return nil, nil, nil
	}

	criticalNodes, nearCriticalNodes = traceCriticalPath(nodes, needsMap, runEnd)

	sort.Slice(criticalNodes, func(i, j int) bool {
		return criticalNodes[i].StartTime.Before(criticalNodes[j].StartTime)
	})

	sort.Slice(nearCriticalNodes, func(i, j int) bool {
		return nearCriticalNodes[i].StartTime.Before(nearCriticalNodes[j].StartTime)
	})

	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].StartTime.Before(nodes[j].StartTime)
	})

	return nodes, criticalNodes, nearCriticalNodes
}

func findJobInMap(targetName string, jobMap map[string]*gather.JobData) (*gather.JobData, string) {
	targetName = strings.TrimSpace(targetName)
	if depJob, exists := jobMap[targetName]; exists {
		return depJob, targetName
	}
	for k, depJob := range jobMap {
		kTrim := strings.TrimSpace(k)
		if strings.EqualFold(kTrim, targetName) || strings.HasPrefix(kTrim, targetName) ||
			strings.HasSuffix(kTrim, targetName) ||
			strings.Contains(kTrim, targetName) {
			return depJob, k
		}
	}
	return nil, ""
}

func jobDur(j *gather.JobData) time.Duration {
	if j == nil || j.GetStartedAt().IsZero() || j.GetCompletedAt().IsZero() {
		return 0
	}
	return j.GetCompletedAt().Sub(j.GetStartedAt().Time)
}

func resolveExplicitNeeds(jNameTrim string, needsMap map[string][]string, jobMap map[string]*gather.JobData) string {
	needs, ok := needsMap[jNameTrim]
	if !ok {
		for name, nList := range needsMap {
			nameTrim := strings.TrimSpace(name)
			if strings.Contains(jNameTrim, nameTrim) || strings.Contains(nameTrim, jNameTrim) {
				needs = nList
				ok = true
				break
			}
		}
	}
	if !ok {
		return ""
	}

	var blockingNeed string
	var latestDepEnd time.Time
	var bestDepJob *gather.JobData
	for _, needName := range needs {
		needNameTrim := strings.TrimSpace(needName)
		depJob, foundName := findJobInMap(needNameTrim, jobMap)
		if depJob != nil && !depJob.GetCompletedAt().IsZero() {
			depEnd := depJob.GetCompletedAt().Time
			if latestDepEnd.IsZero() || depEnd.After(latestDepEnd) ||
				(depEnd.Equal(latestDepEnd) && bestDepJob != nil && jobDur(depJob) > jobDur(bestDepJob)) ||
				(depEnd.Equal(latestDepEnd) && bestDepJob != nil && jobDur(depJob) == jobDur(bestDepJob) && foundName < blockingNeed) {
				latestDepEnd = depEnd
				bestDepJob = depJob
				blockingNeed = foundName
			}
		}
	}
	return blockingNeed
}

func resolveFallbackNeeds(jNameTrim string, start time.Time, jobMap map[string]*gather.JobData) string {
	var blockingNeed string
	var latestDepEnd time.Time
	var bestDepJob *gather.JobData
	names := make([]string, 0, len(jobMap))
	for name := range jobMap {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		depJob := jobMap[name]
		if strings.TrimSpace(name) == jNameTrim || depJob.GetCompletedAt().IsZero() {
			continue
		}
		depEnd := depJob.GetCompletedAt().Time
		if depEnd.Before(start) || depEnd.Equal(start) {
			if latestDepEnd.IsZero() || depEnd.After(latestDepEnd) ||
				(depEnd.Equal(latestDepEnd) && bestDepJob != nil && jobDur(depJob) > jobDur(bestDepJob)) {
				latestDepEnd = depEnd
				bestDepJob = depJob
				blockingNeed = name
			}
		}
	}
	return blockingNeed
}

func resolveBlockingNeed(
	j *gather.JobData,
	start time.Time,
	jobs []*gather.JobData,
	jobMap map[string]*gather.JobData,
	needsMap map[string][]string,
) string {
	jNameTrim := strings.TrimSpace(j.GetName())
	blockingNeed := resolveExplicitNeeds(jNameTrim, needsMap, jobMap)
	if blockingNeed == "" && len(jobs) > 1 {
		blockingNeed = resolveFallbackNeeds(jNameTrim, start, jobMap)
	}
	return blockingNeed
}

func buildJobNodes(jobs []*gather.JobData, needsMap map[string][]string) []CriticalPathNode {
	jobMap := make(map[string]*gather.JobData)
	for _, j := range jobs {
		if j != nil && j.WorkflowJob != nil && j.GetConclusion() != "skipped" {
			jobMap[strings.TrimSpace(j.GetName())] = j
		}
	}

	nodes := make([]CriticalPathNode, 0, len(jobs))
	for _, j := range jobs {
		if j == nil || j.GetStartedAt().IsZero() || j.GetConclusion() == "skipped" {
			continue
		}
		start := j.GetStartedAt().Time
		end := j.GetCompletedAt().Time
		if end.IsZero() {
			end = start.Add(time.Second)
		}

		var queueTime time.Duration
		if !j.GetCreatedAt().IsZero() && j.GetCreatedAt().Before(j.GetStartedAt().Time) {
			queueTime = j.GetStartedAt().Sub(j.GetCreatedAt().Time)
		}

		blockingNeed := resolveBlockingNeed(j, start, jobs, jobMap, needsMap)

		nodes = append(nodes, CriticalPathNode{
			JobID:        j.GetID(),
			JobName:      strings.TrimSpace(j.GetName()),
			Duration:     end.Sub(start),
			QueueTime:    queueTime,
			StartTime:    start,
			CompletedAt:  end,
			BlockingNeed: blockingNeed,
		})
	}
	return nodes
}

func traceCriticalPath(
	nodes []CriticalPathNode,
	needsMap map[string][]string,
	runEnd time.Time,
) (criticalNodes, nearCriticalNodes []CriticalPathNode) {
	successors := make(map[string][]string)
	for jobName, needs := range needsMap {
		jobNameTrim := strings.TrimSpace(jobName)
		for _, need := range needs {
			needTrim := strings.TrimSpace(need)
			successors[needTrim] = append(successors[needTrim], jobNameTrim)
		}
	}

	nodeByName := make(map[string]*CriticalPathNode)
	for i := range nodes {
		nodeByName[strings.TrimSpace(nodes[i].JobName)] = &nodes[i]
	}

	var endNode *CriticalPathNode
	for i := range nodes {
		n := &nodes[i]
		nTrim := strings.TrimSpace(n.JobName)
		succs := successors[nTrim]
		if len(succs) == 0 {
			n.Slack = runEnd.Sub(n.CompletedAt)
			if endNode == nil || n.CompletedAt.After(endNode.CompletedAt) ||
				(n.CompletedAt.Equal(endNode.CompletedAt) && n.Duration > endNode.Duration) ||
				(n.CompletedAt.Equal(endNode.CompletedAt) && n.Duration == endNode.Duration && n.JobName < endNode.JobName) {
				endNode = n
			}
		} else {
			n.Slack = computeNodeSlack(n, succs, nodeByName, runEnd)
		}
		if n.Slack < 0 {
			n.Slack = 0
		}
	}

	curr := endNode
	for curr != nil {
		curr.IsCritical = true
		curr.Slack = 0
		if curr.BlockingNeed != "" {
			curr = nodeByName[strings.TrimSpace(curr.BlockingNeed)]
		} else {
			curr = nil
		}
	}

	for _, n := range nodes {
		if n.IsCritical {
			criticalNodes = append(criticalNodes, n)
		} else if n.Slack <= 60*time.Second {
			nearCriticalNodes = append(nearCriticalNodes, n)
		}
	}
	return criticalNodes, nearCriticalNodes
}

func computeNodeSlack(
	n *CriticalPathNode,
	succs []string,
	nodeByName map[string]*CriticalPathNode,
	runEnd time.Time,
) time.Duration {
	var minSlack time.Duration
	firstSucc := true
	for _, succName := range succs {
		if succNode, ok := nodeByName[succName]; ok {
			succSlack := succNode.StartTime.Sub(n.CompletedAt) + succNode.Slack
			if firstSucc || succSlack < minSlack {
				minSlack = succSlack
			}
			firstSucc = false
		}
	}
	if !firstSucc {
		return minSlack
	}
	return runEnd.Sub(n.CompletedAt)
}

// AggregateSteps aggregates step metrics across all jobs in a workflow run.
func AggregateSteps(jobs []*gather.JobData) ([]StepSummary, map[string]float64) {
	type stepStats struct {
		name      string
		durations []time.Duration
	}

	statsMap := make(map[string]*stepStats)
	var grandTotalStepDuration time.Duration

	for _, j := range jobs {
		if j == nil || j.WorkflowJob == nil || j.GetConclusion() == "skipped" {
			continue
		}
		for _, step := range j.Steps {
			if step == nil || step.GetName() == "" || step.GetStartedAt().IsZero() || step.GetCompletedAt().IsZero() {
				continue
			}
			dur := step.GetCompletedAt().Sub(step.GetStartedAt().Time)
			if dur <= 0 {
				continue
			}
			name := step.GetName()
			st, ok := statsMap[name]
			if !ok {
				st = &stepStats{name: name}
				statsMap[name] = st
			}
			st.durations = append(st.durations, dur)
			grandTotalStepDuration += dur
		}
	}

	summaries := make([]StepSummary, 0, len(statsMap))
	pctMap := make(map[string]float64)

	for _, st := range statsMap {
		slices.Sort(st.durations)

		var total time.Duration
		for _, d := range st.durations {
			total += d
		}
		count := len(st.durations)
		minDur := st.durations[0]
		maxDur := st.durations[count-1]
		medianDur := st.durations[count/2]
		if count%2 == 0 && count > 1 {
			medianDur = (st.durations[count/2-1] + st.durations[count/2]) / 2
		}

		pct := 0.0
		if grandTotalStepDuration > 0 {
			pct = (float64(total) / float64(grandTotalStepDuration)) * 100.0
		}
		pctMap[st.name] = pct

		summaries = append(summaries, StepSummary{
			Name:           st.name,
			Count:          count,
			TotalDuration:  total,
			MinDuration:    minDur,
			MedianDuration: medianDur,
			MaxDuration:    maxDur,
			PctTotal:       pct,
		})
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].TotalDuration > summaries[j].TotalDuration
	})

	return summaries, pctMap
}

// GetSlowestJobSteps returns step breakdowns for top N slowest completed jobs.
func GetSlowestJobSteps(jobs []*gather.JobData, topN int) []JobStepBreakdown {
	if topN <= 0 {
		return nil
	}

	sortedJobs := make([]*gather.JobData, 0, len(jobs))
	for _, j := range jobs {
		if j == nil || j.WorkflowJob == nil || j.GetStartedAt().IsZero() || j.GetCompletedAt().IsZero() ||
			j.GetConclusion() == "skipped" {
			continue
		}
		sortedJobs = append(sortedJobs, j)
	}

	sort.Slice(sortedJobs, func(i, j int) bool {
		durI := sortedJobs[i].GetCompletedAt().Sub(sortedJobs[i].GetStartedAt().Time)
		durJ := sortedJobs[j].GetCompletedAt().Sub(sortedJobs[j].GetStartedAt().Time)
		return durI > durJ
	})

	if len(sortedJobs) > topN {
		sortedJobs = sortedJobs[:topN]
	}

	result := make([]JobStepBreakdown, 0, len(sortedJobs))
	for _, j := range sortedJobs {
		jobDur := j.GetCompletedAt().Sub(j.GetStartedAt().Time)
		var (
			steps      []StepDetail
			minorCount int
			minorTotal time.Duration
		)

		for _, step := range j.Steps {
			if step == nil || step.GetName() == "" {
				continue
			}
			var stepDur time.Duration
			if !step.GetStartedAt().IsZero() && !step.GetCompletedAt().IsZero() {
				stepDur = step.GetCompletedAt().Sub(step.GetStartedAt().Time)
			}

			if stepDur <= time.Second {
				minorCount++
				minorTotal += stepDur
				continue
			}

			steps = append(steps, StepDetail{
				Name:       step.GetName(),
				Duration:   stepDur,
				Status:     step.GetStatus(),
				Conclusion: step.GetConclusion(),
			})
		}

		sort.Slice(steps, func(i, j int) bool {
			return steps[i].Duration > steps[j].Duration
		})

		result = append(result, JobStepBreakdown{
			JobID:           j.GetID(),
			JobName:         j.GetName(),
			Duration:        jobDur,
			Steps:           steps,
			MinorStepsCount: minorCount,
			MinorStepsTotal: minorTotal,
		})
	}

	return result
}
