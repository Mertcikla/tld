package store

import (
	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/codeindex/indexer"
)

// Compatibility is computed when reading, never persisted: an upgrade can make
// an immutable snapshot incompatible without changing its stored stamps.
func setSnapshotCompatibility(snap *pb.Snapshot) {
	err := indexer.CheckSnapshot(snap)
	snap.Compatible = err == nil
	snap.IncompatibilityReason = ""
	if err != nil {
		snap.IncompatibilityReason = err.Error()
	}
}
