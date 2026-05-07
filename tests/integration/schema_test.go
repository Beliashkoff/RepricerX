//go:build integration

package integration

import (
	"context"
	"testing"
)

func TestImportLogTimestampColumnsUseTimeZone(t *testing.T) {
	rows, err := testPool.Query(context.Background(), `
		SELECT column_name, data_type
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND table_name = 'import_log'
		  AND column_name IN ('started_at', 'finished_at', 'requested_at')`)
	if err != nil {
		t.Fatalf("query import_log column types: %v", err)
	}
	defer rows.Close()

	got := map[string]string{}
	for rows.Next() {
		var name, dataType string
		if err := rows.Scan(&name, &dataType); err != nil {
			t.Fatalf("scan column type: %v", err)
		}
		got[name] = dataType
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}

	for _, column := range []string{"started_at", "finished_at", "requested_at"} {
		if got[column] != "timestamp with time zone" {
			t.Fatalf("import_log.%s type = %q, want timestamp with time zone", column, got[column])
		}
	}
}
