package search

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// ErrFTS5Unavailable is returned when the SQLite build does not include FTS5.
var ErrFTS5Unavailable = errors.New("FTS5 module not available")

// EnsureFTS5Schema creates the FTS5 virtual table and triggers that keep
// it in sync with the business tables.
//
// Design:
//   - search_index is a single FTS5 virtual table that indexes both files and folders.
//   - Columns: entity_type ('file'|'folder'), entity_id, name (file title or folder name), tags.
//   - The content is maintained by application-level sync helpers (RebuildFileIndex, etc.)
//     rather than SQLite triggers, because tag associations span multiple tables and
//     triggers cannot easily capture the denormalized tag string.
//   - The `content=""` form (external-content-less) is used so we have full control
//     over INSERT/DELETE without SQLite trying to read from a content table.
func EnsureFTS5Schema(db *gorm.DB) error {
	ddl := `
		CREATE VIRTUAL TABLE IF NOT EXISTS search_index USING fts5(
			entity_type,
			entity_id,
			name,
			tags,
			content='',
			contentless_delete=1,
			tokenize='unicode61 remove_diacritics 2'
		);
	`
	if err := db.Exec(ddl).Error; err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			return ErrFTS5Unavailable
		}
		return fmt.Errorf("create FTS5 search_index table: %w", err)
	}
	return nil
}
