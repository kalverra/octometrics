package observe

import (
	"github.com/rs/zerolog"

	"github.com/kalverra/octometrics/gather"
)

// ManifestRecord aliases gather.ManifestRecord.
type ManifestRecord = gather.ManifestRecord

// AppendManifestRecord delegates to gather.AppendManifestRecord.
func AppendManifestRecord(dataDir, owner, repo string, rec ManifestRecord) error {
	return gather.AppendManifestRecord(dataDir, owner, repo, rec)
}

// LoadManifest delegates to gather.LoadManifest.
func LoadManifest(dataDir, owner, repo string) ([]ManifestRecord, error) {
	return gather.LoadManifest(dataDir, owner, repo)
}

// RebuildManifest delegates to gather.RebuildManifest.
func RebuildManifest(log zerolog.Logger, dataDir string) error {
	return gather.RebuildManifest(log, dataDir)
}
