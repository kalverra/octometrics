package monitor

import (
	"testing"

	"github.com/kalverra/octometrics/internal/testhelpers"
)

func BenchmarkObserveAll(b *testing.B) {
	log, _ := testhelpers.Setup(b, testhelpers.Silent())
	opts := defaultOptions()
	for b.Loop() {
		if err := spot(log, opts); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkObserveCPU(b *testing.B) {
	log, _ := testhelpers.Setup(b, testhelpers.Silent())

	for b.Loop() {
		if err := spotCPU(log); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkObserveMemory(b *testing.B) {
	log, _ := testhelpers.Setup(b, testhelpers.Silent())

	for b.Loop() {
		if err := spotMemory(log); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkObserveDisk(b *testing.B) {
	log, _ := testhelpers.Setup(b, testhelpers.Silent())

	for b.Loop() {
		if err := spotDisk(log); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkObserveIO(b *testing.B) {
	log, _ := testhelpers.Setup(b, testhelpers.Silent())

	for b.Loop() {
		if err := spotIO(log); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSystemInfo(b *testing.B) {
	log, _ := testhelpers.Setup(b, testhelpers.Silent())

	for b.Loop() {
		if err := systemInfo(log); err != nil {
			b.Fatal(err)
		}
	}
}
