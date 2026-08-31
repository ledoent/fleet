package tables

import (
	"bytes"
	"crypto/md5" //nolint:gosec // non-cryptographic row-lookup checksum
	"database/sql"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/reflectx"
)

func init() {
	MigrationClient.AddMigration(Up_20260810160000, Down_20260810160000)
}

// canonicalSoftwareChecksum is fleet.Software.ComputeRawChecksum expressed in SQL.
const canonicalSoftwareChecksum = "UNHEX(MD5(CONCAT_WS(CHAR(0), " +
	"version, source, COALESCE(bundle_identifier, ''), `release`, arch, vendor, " +
	"extension_for, extension_id, name, " +
	"NULLIF(COALESCE(application_id, ''), ''), NULLIF(COALESCE(upgrade_code, ''), ''))))"

// softwareChecksumMapping is a duplicate software row and the row it merges into.
type softwareChecksumMapping struct {
	StaleID    uint64 `db:"stale_id"`
	SurvivorID uint64 `db:"survivor_id"`
}

// Vars so tests can lower them to exercise the batching.
var (
	dedupeSoftwareBatchSize    = 1000
	recomputeSoftwareBatchSize = uint64(50000)
)

func Up_20260810160000(tx *sql.Tx) error {
	if isPostgres() {
		return up20260810160000PG(tx)
	}
	// v4.76.0 moved `name` from the front to the back of the software checksum. Rows
	// created before that upgrade kept their old checksum, so when a host reported the
	// same software again it no longer matched and a second `software` row was inserted:
	// the same software twice, with the same CVEs but split host counts. Merge those
	// duplicates, then recompute every checksum so pre-upgrade rows that have not
	// duplicated yet cannot duplicate later.
	txx := sqlx.Tx{Tx: tx, Mapper: reflectx.NewMapperFunc("db", sqlx.NameMapper)}

	// A window function maps duplicates to their survivor in one pass; a GROUP BY would
	// have to join its result back, which has no index. Partitioning by the canonical
	// checksum rather than the columns it hashes sorts a 16-byte binary key instead of
	// 11 utf8mb4 strings (~8x faster), and matches exactly the equivalence the recompute
	// below must not collide under (CONCAT_WS skips NULLs, so distinct column tuples can
	// share a checksum).
	var mappings []softwareChecksumMapping
	if err := txx.Select(&mappings, `
		SELECT stale_id, survivor_id FROM (
			SELECT id AS stale_id,
				MIN(id) OVER (PARTITION BY `+canonicalSoftwareChecksum+`) AS survivor_id
			FROM software
		) m WHERE stale_id <> survivor_id`); err != nil {
		return fmt.Errorf("selecting duplicate software rows: %w", err)
	}

	logger.Info.Printf("deduplicating software: found %d duplicate software rows\n", len(mappings))

	var recomputeMaxID uint64
	return withSteps([]migrationStep{
		// Merge each duplicate onto its survivor. incrementalMigrationStep skips a step
		// whose total is zero, so instances with no duplicates never touch the large host
		// software tables.
		incrementalMigrationStep(
			func(*sql.Tx) (uint64, error) { return uint64(len(mappings)), nil },
			mergeDuplicateSoftware(mappings),
		),
		// Recompute every checksum to the current formula. The merge made canonical
		// checksums unique, so this cannot collide on idx_software_checksum. Progress is
		// counted in id-range batches, which keeps each statement bounded on this large
		// table.
		incrementalMigrationStep(
			func(tx *sql.Tx) (uint64, error) {
				if err := tx.QueryRow("SELECT COALESCE(MAX(id), 0) FROM software").Scan(&recomputeMaxID); err != nil {
					return 0, fmt.Errorf("getting max software id: %w", err)
				}
				return recomputeMaxID/recomputeSoftwareBatchSize + 1, nil
			},
			func(tx *sql.Tx, increment incrementCountFn) error {
				var recomputed int64
				for start := uint64(0); start <= recomputeMaxID; start += recomputeSoftwareBatchSize {
					res, err := tx.Exec(
						"UPDATE software SET checksum = "+canonicalSoftwareChecksum+
							" WHERE id >= ? AND id < ? AND checksum <> "+canonicalSoftwareChecksum,
						start, start+recomputeSoftwareBatchSize,
					)
					if err != nil {
						return fmt.Errorf("recomputing software checksums: %w", err)
					}
					n, err := res.RowsAffected()
					if err != nil {
						return fmt.Errorf("counting recomputed software checksums: %w", err)
					}
					recomputed += n
					increment()
				}
				logger.Info.Printf("deduplicating software: recomputed %d software checksums\n", recomputed)
				return nil
			},
		),
	}, tx)
}

// up20260810160000PG is the PostgreSQL path. The canonical checksum joins its
// inputs with a NUL byte, which PG text expressions cannot carry (chr(0) is
// rejected), so instead of expressing the formula in SQL the checksums are
// computed in Go — the same conditional-append semantics as the MySQL
// CONCAT_WS expression (skip SQL NULLs, keep empty strings, except the two
// NULLIF-wrapped trailing columns which are skipped when NULL or empty).
// Row volume is bounded by the software inventory, and the merge reuses the
// batched mergeDuplicateSoftware pass.
func up20260810160000PG(tx *sql.Tx) error {
	type softwareRow struct {
		id       uint64
		checksum []byte
		computed []byte
	}
	rows, err := tx.Query(`
		SELECT id, checksum, version, source, COALESCE(bundle_identifier, ''),
			release, arch, vendor, extension_for, extension_id, name,
			application_id, rtrim(upgrade_code)
		FROM software ORDER BY id`)
	if err != nil {
		return fmt.Errorf("selecting software rows for checksum recompute: %w", err)
	}
	defer rows.Close()

	var all []softwareRow
	for rows.Next() {
		var r softwareRow
		var version, source, bundleID, release, arch, vendor, extensionFor, extensionID, name string
		var applicationID, upgradeCode *string
		if err := rows.Scan(&r.id, &r.checksum, &version, &source, &bundleID, &release,
			&arch, &vendor, &extensionFor, &extensionID, &name, &applicationID, &upgradeCode); err != nil {
			return fmt.Errorf("scanning software row: %w", err)
		}
		// Mirrors fleet.Software.ComputeRawChecksum (the sole source of truth
		// for this formula — keep in lockstep with it and with
		// canonicalSoftwareChecksum above).
		cols := []string{version, source, bundleID, release, arch, vendor, extensionFor, extensionID, name}
		if applicationID != nil && *applicationID != "" {
			cols = append(cols, *applicationID)
		}
		if upgradeCode != nil && *upgradeCode != "" {
			cols = append(cols, *upgradeCode)
		}
		sum := md5.Sum([]byte(strings.Join(cols, "\x00"))) //nolint:gosec // non-cryptographic row-lookup checksum
		r.computed = sum[:]
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating software rows: %w", err)
	}

	// Duplicates map to the lowest id sharing their canonical checksum, same
	// as the MySQL window-function pass (rows were read in id order).
	survivorByChecksum := make(map[string]uint64, len(all))
	var mappings []softwareChecksumMapping
	for _, r := range all {
		key := string(r.computed)
		if survivor, ok := survivorByChecksum[key]; ok {
			mappings = append(mappings, softwareChecksumMapping{StaleID: r.id, SurvivorID: survivor})
		} else {
			survivorByChecksum[key] = r.id
		}
	}
	logger.Info.Printf("deduplicating software: found %d duplicate software rows\n", len(mappings))

	staleSet := make(map[uint64]struct{}, len(mappings))
	for _, m := range mappings {
		staleSet[m.StaleID] = struct{}{}
	}

	return withSteps([]migrationStep{
		incrementalMigrationStep(
			func(*sql.Tx) (uint64, error) { return uint64(len(mappings)), nil },
			mergeDuplicateSoftware(mappings),
		),
		func(tx *sql.Tx) error {
			var recomputed int64
			for _, r := range all {
				if _, stale := staleSet[r.id]; stale {
					continue // row deleted by the merge
				}
				if bytes.Equal(r.checksum, r.computed) {
					continue
				}
				if _, err := tx.Exec(`UPDATE software SET checksum = ? WHERE id = ?`, r.computed, r.id); err != nil {
					return fmt.Errorf("recomputing software checksum for id %d: %w", r.id, err)
				}
				recomputed++
			}
			logger.Info.Printf("deduplicating software: recomputed %d software checksums\n", recomputed)
			return nil
		},
	}, tx)
}

// mergeDuplicateSoftware repoints every host reference from each duplicate row onto its
// survivor, then deletes the duplicates.
func mergeDuplicateSoftware(mappings []softwareChecksumMapping) executeWithProgressFn {
	return func(tx *sql.Tx, increment incrementCountFn) error {
		for start := 0; start < len(mappings); start += dedupeSoftwareBatchSize {
			end := min(start+dedupeSoftwareBatchSize, len(mappings))
			batch := mappings[start:end]

			// One CASE per batch, so each table below is touched once instead of once per
			// duplicate.
			var whens strings.Builder
			caseArgs := make([]any, 0, len(batch)*2)
			staleIDs := make([]any, 0, len(batch))
			for _, m := range batch {
				whens.WriteString(" WHEN ? THEN ?")
				caseArgs = append(caseArgs, m.StaleID, m.SurvivorID)
				staleIDs = append(staleIDs, m.StaleID)
			}
			mapCase := func(col string) string { return "CASE " + col + whens.String() + " END" }

			// Give the survivor a link for every host on a duplicate row. INSERT IGNORE skips
			// hosts already linked to it; the duplicate links are deleted below.
			stmt, args, err := sqlx.In(
				"INSERT IGNORE INTO host_software (host_id, software_id, last_opened_at) "+
					"SELECT host_id, "+mapCase("software_id")+", last_opened_at "+
					"FROM host_software WHERE software_id IN (?)",
				append(append([]any{}, caseArgs...), staleIDs)...)
			if err != nil {
				return fmt.Errorf("building host_software repoint: %w", err)
			}
			if _, err := tx.Exec(stmt, args...); err != nil {
				return fmt.Errorf("repointing host_software onto surviving software rows: %w", err)
			}

			// host_software_installed_paths is only indexed on (host_id, software_id), so
			// filtering it by software_id alone scans this large table. Drive off host_software
			// instead, which is indexed on software_id and still holds the duplicate links
			// here, to reach each path row by the full key. STRAIGHT_JOIN and FORCE INDEX pin
			// that plan, since mid-migration index statistics are unreliable.
			//
			// This skips a path row whose host_software link is already missing, which only
			// happens when a host's paths were not reconciled after its software was (separate
			// transactions). Such a row is already stale: nothing joining software can see it,
			// and the host's next detail query deletes it.
			installedPathsStmt := "UPDATE host_software hs " +
				"STRAIGHT_JOIN host_software_installed_paths hsip FORCE INDEX (host_id_software_id_idx) " +
				"  ON hsip.host_id = hs.host_id AND hsip.software_id = hs.software_id " +
				"SET hsip.software_id = " + mapCase("hs.software_id") +
				" WHERE hs.software_id IN (?)"
			if isPostgres() {
				// PG updates the target table via FROM; the join to
				// host_software preserves the drive-off-the-link semantics
				// (a path row without a live link is skipped, same as MySQL).
				installedPathsStmt = "UPDATE host_software_installed_paths hsip " +
					"SET software_id = " + mapCase("hsip.software_id") +
					" FROM host_software hs " +
					"WHERE hs.host_id = hsip.host_id AND hs.software_id = hsip.software_id " +
					"AND hsip.software_id IN (?)"
			}
			stmt, args, err = sqlx.In(installedPathsStmt,
				append(append([]any{}, caseArgs...), staleIDs)...)
			if err != nil {
				return fmt.Errorf("building host_software_installed_paths repoint: %w", err)
			}
			if _, err := tx.Exec(stmt, args...); err != nil {
				return fmt.Errorf("repointing host_software_installed_paths onto surviving software rows: %w", err)
			}

			// The counts are recomputed by the software host-count crons, and deleting a
			// software row cascades software_cpe.
			for _, del := range []struct{ desc, query string }{
				{"host_software links", "DELETE FROM host_software WHERE software_id IN (?)"},
				{"kernel host counts", "DELETE FROM kernel_host_counts WHERE software_id IN (?)"},
				{"software CVEs", "DELETE FROM software_cve WHERE software_id IN (?)"},
				{"software host counts", "DELETE FROM software_host_counts WHERE software_id IN (?)"},
				{"software rows", "DELETE FROM software WHERE id IN (?)"},
			} {
				stmt, args, err := sqlx.In(del.query, staleIDs)
				if err != nil {
					return fmt.Errorf("building duplicate %s delete: %w", del.desc, err)
				}
				if _, err := tx.Exec(stmt, args...); err != nil {
					return fmt.Errorf("deleting duplicate %s: %w", del.desc, err)
				}
			}
			for range batch {
				increment()
			}
		}
		return nil
	}
}

func Down_20260810160000(tx *sql.Tx) error {
	return nil
}
