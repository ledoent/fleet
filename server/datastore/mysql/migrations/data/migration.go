package data

import (
	"fmt"

	"github.com/fleetdm/fleet/v4/server/goose"
)

var MigrationClient = goose.New("migration_status_data", goose.MySqlDialect{})

// SetDialect updates the migration client's SQL dialect.
func SetDialect(driver string) {
	if err := MigrationClient.SetDialect(driver); err != nil {
		panic(fmt.Sprintf("migrations/data: unsupported dialect %q: %v", driver, err))
	}
}
