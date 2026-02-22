package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

type Runner struct {
	FS fs.FS
}

func NewRunner(files fs.FS) *Runner {
	return &Runner{FS: files}
}

func (r *Runner) Apply(ctx context.Context, db *sql.DB, dialect string) error {
	if db == nil {
		return fmt.Errorf("nil db")
	}
	if dialect == "" {
		return fmt.Errorf("empty dialect")
	}
	base := filepath.ToSlash(filepath.Join("db", "migrations", dialect))
	entries, err := fs.ReadDir(r.FS, base)
	if err != nil {
		return err
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		files = append(files, filepath.ToSlash(filepath.Join(base, e.Name())))
	}
	sort.Strings(files)
	for _, path := range files {
		sqlBytes, err := fs.ReadFile(r.FS, path)
		if err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("apply %s: %w", path, err)
		}
	}
	return nil
}
