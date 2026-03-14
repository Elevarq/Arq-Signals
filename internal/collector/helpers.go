package collector

import (
	"context"
	"regexp"
	"strconv"

	"github.com/jackc/pgx/v5"
)

var pgVersionRe = regexp.MustCompile(`PostgreSQL (\d+)`)

// parsePGMajorVersion extracts the major version number from a PostgreSQL version string.
// "PostgreSQL 16.2 on ..." -> 16. Returns 0 if not parseable.
func parsePGMajorVersion(full string) int {
	m := pgVersionRe.FindStringSubmatch(full)
	if len(m) < 2 {
		return 0
	}
	v, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	return v
}

// detectExtensions queries pg_extension and returns a list of installed extension names.
func detectExtensions(ctx context.Context, tx pgx.Tx) []string {
	rows, err := tx.Query(ctx, "SELECT extname FROM pg_extension")
	if err != nil {
		return nil
	}
	defer rows.Close()

	var exts []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		exts = append(exts, name)
	}
	return exts
}

// populateSnapshotField maps query results into the SnapshotData struct fields
// for backward compatibility with the monolithic snapshot format.
func populateSnapshotField(data *SnapshotData, queryID string, rows []map[string]any) {
	switch queryID {
	case "pg_version_v1":
		if len(rows) > 0 {
			if v, ok := rows[0]["version"]; ok {
				if s, ok := v.(string); ok {
					data.Version = s
				}
			}
		}
	case "pg_settings_v1":
		data.Settings = rows
	case "pg_stat_activity_v1":
		data.Activity = rows
	case "pg_stat_database_v1":
		data.Database = rows
	case "pg_stat_user_tables_v1":
		data.UserTables = rows
	case "pg_stat_user_indexes_v1":
		data.UserIndexes = rows
	case "pg_statio_user_tables_v1":
		data.StatioTables = rows
	case "pg_statio_user_indexes_v1":
		data.StatioIndexes = rows
	case "pg_stat_statements_v1":
		data.StatStatements = rows
	}
}
