package monitor

import (
	"testing"

	"github.com/kalverra/octometrics/internal/testhelpers"
)

func BenchmarkObserveAll(b *testing.B) {
	log, _ := testhelpers.Setup(b, testhelpers.Silent())
	opts := defaultOptions()
	for b.Loop() {
		if err := observe(log, opts); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkObserveCPU(b *testing.B) {
	log, _ := testhelpers.Setup(b, testhelpers.Silent())

	for b.Loop() {
		if err := observeCPU(log); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkObserveMemory(b *testing.B) {
	log, _ := testhelpers.Setup(b, testhelpers.Silent())

	for b.Loop() {
		if err := observeMemory(log); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkObserveDisk(b *testing.B) {
	log, _ := testhelpers.Setup(b, testhelpers.Silent())

	for b.Loop() {
		if err := observeDisk(log); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkObserveIO(b *testing.B) {
	log, _ := testhelpers.Setup(b, testhelpers.Silent())

	for b.Loop() {
		if err := observeIO(log); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGitHubActionsEnvVars(b *testing.B) {
	log, _ := testhelpers.Setup(b, testhelpers.Silent())
	b.Setenv("GITHUB_ACTIONS", "true")

	for b.Loop() {
		if err := observeGitHubActionsEnvVars(log); err != nil {
			b.Fatal(err)
		}
	}
}
