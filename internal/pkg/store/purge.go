package store

import (
	"os"
	"path/filepath"

	"gorm.io/gorm"
)

// PurgeRemovedLogPipe removes data left behind by the built-in log collector,
// which was removed in v1.4.3.
//
// It is idempotent: a missing table or directory is a no-op, so it can run on
// every start. It deliberately touches nothing else under the data directory,
// and removes the logs/ directory itself only when it ends up empty —
// os.Remove refuses non-empty directories, so a file the operator put there by
// hand survives.
//
// Call this only on the normal startup path. store.Init is also used by account
// recovery, and resetting a password must not delete data.
func PurgeRemovedLogPipe(db *gorm.DB, dataDir string) error {
	if err := db.Exec("DROP TABLE IF EXISTS log_pipe_registries").Error; err != nil {
		return err
	}
	logsDir := filepath.Join(dataDir, "logs")
	for _, name := range []string{"tasks.d", "spool", "registry.db", "opsmini.yml"} {
		if err := os.RemoveAll(filepath.Join(logsDir, name)); err != nil {
			return err
		}
	}
	_ = os.Remove(logsDir)
	return nil
}
