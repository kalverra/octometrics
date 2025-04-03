package gather

import (
	"time"

	"github.com/google/go-github/v70/github"
)

var (
	startTime       = time.Date(2025, 04, 20, 0, 1, 0, 0, time.UTC)
	createdTime     = time.Date(2025, 04, 20, 0, 0, 0, 0, time.UTC)
	endTime         = time.Date(2025, 04, 20, 1, 1, 0, 0, time.UTC)
	mockWorkflowRun = &github.WorkflowRun{
		ID:               github.Ptr(int64(1)),
		Name:             github.Ptr("mocked-workflow-run"),
		NodeID:           github.Ptr("mocked-node-id"),
		HeadBranch:       github.Ptr("mocked-head-branch"),
		HeadSHA:          github.Ptr("mocked-sha"),
		Path:             github.Ptr("mocked-workflow-path.yml"),
		RunNumber:        github.Ptr(1),
		Event:            github.Ptr("push"),
		DisplayTitle:     github.Ptr("mocked-display-title"),
		Status:           github.Ptr("completed"),
		Conclusion:       github.Ptr("success"),
		WorkflowID:       github.Ptr(int64(1)),
		CheckSuiteID:     github.Ptr(int64(1)),
		CheckSuiteNodeID: github.Ptr("mocked-check-suite-node-id"),
		URL:              github.Ptr("https://api.github.com/repos/kalverra/octometrics/actions/runs/1"),
		RunStartedAt:     github.Ptr(github.Timestamp{Time: startTime}),
		CreatedAt:        github.Ptr(github.Timestamp{Time: createdTime}),
		UpdatedAt:        github.Ptr(github.Timestamp{Time: endTime}),
		WorkflowURL:      github.Ptr("https://api.github.com/repos/kalverra/octometrics/actions/workflows/1"),
		Repository: &github.Repository{
			ID:       github.Ptr(int64(1)),
			Name:     github.Ptr("octometrics"),
			FullName: github.Ptr("kalverra/octometrics"),
		},
	}
	mockJobs = []*github.WorkflowJob{
		{
			ID:          github.Ptr(int64(1)),
			RunID:       github.Ptr(int64(1)),
			HeadBranch:  github.Ptr("mocked-head-branch"),
			HeadSHA:     github.Ptr("mocked-sha"),
			Status:      github.Ptr("completed"),
			Conclusion:  github.Ptr("success"),
			CreatedAt:   github.Ptr(github.Timestamp{Time: createdTime}),
			StartedAt:   github.Ptr(github.Timestamp{Time: startTime}),
			CompletedAt: github.Ptr(github.Timestamp{Time: endTime}),
			Name:        github.Ptr("mocked-job-1"),
			Steps: []*github.TaskStep{
				{
					Name:        github.Ptr("mocked-step-1"),
					Status:      github.Ptr("completed"),
					Conclusion:  github.Ptr("success"),
					StartedAt:   github.Ptr(github.Timestamp{Time: startTime}),
					CompletedAt: github.Ptr(github.Timestamp{Time: endTime}),
				},
			},
		},
		{
			ID:          github.Ptr(int64(2)),
			RunID:       github.Ptr(int64(1)),
			HeadBranch:  github.Ptr("mocked-head-branch"),
			HeadSHA:     github.Ptr("mocked-sha"),
			Status:      github.Ptr("completed"),
			Conclusion:  github.Ptr("success"),
			CreatedAt:   github.Ptr(github.Timestamp{Time: createdTime}),
			StartedAt:   github.Ptr(github.Timestamp{Time: startTime}),
			CompletedAt: github.Ptr(github.Timestamp{Time: endTime}),
			Name:        github.Ptr("mocked-job-2"),
			Steps: []*github.TaskStep{
				{
					Name:        github.Ptr("mocked-step-1"),
					Status:      github.Ptr("completed"),
					Conclusion:  github.Ptr("success"),
					StartedAt:   github.Ptr(github.Timestamp{Time: startTime}),
					CompletedAt: github.Ptr(github.Timestamp{Time: endTime}),
				},
				{
					Name:        github.Ptr("mocked-step-2"),
					Status:      github.Ptr("completed"),
					Conclusion:  github.Ptr("success"),
					StartedAt:   github.Ptr(github.Timestamp{Time: startTime}),
					CompletedAt: github.Ptr(github.Timestamp{Time: endTime}),
				},
			},
		},
		{
			ID:          github.Ptr(int64(3)),
			RunID:       github.Ptr(int64(1)),
			HeadBranch:  github.Ptr("mocked-head-branch"),
			HeadSHA:     github.Ptr("mocked-sha"),
			Status:      github.Ptr("completed"),
			Conclusion:  github.Ptr("success"),
			CreatedAt:   github.Ptr(github.Timestamp{Time: createdTime}),
			StartedAt:   github.Ptr(github.Timestamp{Time: startTime}),
			CompletedAt: github.Ptr(github.Timestamp{Time: endTime}),
			Name:        github.Ptr("mocked-job-3"),
			Steps: []*github.TaskStep{
				{
					Name:        github.Ptr("mocked-step-1"),
					Status:      github.Ptr("completed"),
					Conclusion:  github.Ptr("success"),
					StartedAt:   github.Ptr(github.Timestamp{Time: startTime}),
					CompletedAt: github.Ptr(github.Timestamp{Time: endTime}),
				},
				{
					Name:        github.Ptr("mocked-step-2"),
					Status:      github.Ptr("completed"),
					Conclusion:  github.Ptr("success"),
					StartedAt:   github.Ptr(github.Timestamp{Time: startTime}),
					CompletedAt: github.Ptr(github.Timestamp{Time: endTime}),
				},
				{
					Name:        github.Ptr("mocked-step-3"),
					Status:      github.Ptr("completed"),
					Conclusion:  github.Ptr("success"),
					StartedAt:   github.Ptr(github.Timestamp{Time: startTime}),
					CompletedAt: github.Ptr(github.Timestamp{Time: endTime}),
				},
			},
		},
		{
			ID:          github.Ptr(int64(4)),
			RunID:       github.Ptr(int64(1)),
			HeadBranch:  github.Ptr("mocked-head-branch"),
			HeadSHA:     github.Ptr("mocked-sha"),
			Status:      github.Ptr("completed"),
			Conclusion:  github.Ptr("success"),
			CreatedAt:   github.Ptr(github.Timestamp{Time: createdTime}),
			StartedAt:   github.Ptr(github.Timestamp{Time: startTime}),
			CompletedAt: github.Ptr(github.Timestamp{Time: endTime}),
			Name:        github.Ptr("mocked-job-4"),
			Steps: []*github.TaskStep{
				{
					Name:        github.Ptr("mocked-step-1"),
					Status:      github.Ptr("completed"),
					Conclusion:  github.Ptr("success"),
					StartedAt:   github.Ptr(github.Timestamp{Time: startTime}),
					CompletedAt: github.Ptr(github.Timestamp{Time: endTime}),
				},
				{
					Name:        github.Ptr("mocked-step-2"),
					Status:      github.Ptr("completed"),
					Conclusion:  github.Ptr("success"),
					StartedAt:   github.Ptr(github.Timestamp{Time: startTime}),
					CompletedAt: github.Ptr(github.Timestamp{Time: endTime}),
				},
				{
					Name:        github.Ptr("mocked-step-3"),
					Status:      github.Ptr("completed"),
					Conclusion:  github.Ptr("success"),
					StartedAt:   github.Ptr(github.Timestamp{Time: startTime}),
					CompletedAt: github.Ptr(github.Timestamp{Time: endTime}),
				},
				{
					Name:        github.Ptr("mocked-step-4"),
					Status:      github.Ptr("completed"),
					Conclusion:  github.Ptr("success"),
					StartedAt:   github.Ptr(github.Timestamp{Time: startTime}),
					CompletedAt: github.Ptr(github.Timestamp{Time: endTime}),
				},
			},
		},
	}
	mockWorkflowRunUsage = github.WorkflowRunUsage{
		Billable: &github.WorkflowRunBillMap{
			"UBUNTU": &github.WorkflowRunBill{ // Free
				TotalMS: github.Ptr(int64(0)),
				Jobs:    github.Ptr(1),
				JobRuns: []*github.WorkflowRunJobRun{
					{
						JobID:      github.Ptr(1),
						DurationMS: github.Ptr(int64(0)),
					},
				},
			},
			"UBUNTU_16_CORE": &github.WorkflowRunBill{
				TotalMS: github.Ptr(int64(endTime.Sub(startTime).Milliseconds() * 2)),
				Jobs:    github.Ptr(2),
				JobRuns: []*github.WorkflowRunJobRun{
					{
						JobID:      github.Ptr(2),
						DurationMS: github.Ptr(endTime.Sub(startTime).Milliseconds()),
					},
					{
						JobID:      github.Ptr(3),
						DurationMS: github.Ptr(endTime.Sub(startTime).Milliseconds()),
					},
				},
			},
			"UBUNTU_8_CORE_ARM": &github.WorkflowRunBill{
				TotalMS: github.Ptr(int64(endTime.Sub(startTime).Milliseconds())),
				Jobs:    github.Ptr(1),
				JobRuns: []*github.WorkflowRunJobRun{
					{
						JobID:      github.Ptr(4),
						DurationMS: github.Ptr(endTime.Sub(startTime).Milliseconds()),
					},
				},
			},
		},
	}
)
