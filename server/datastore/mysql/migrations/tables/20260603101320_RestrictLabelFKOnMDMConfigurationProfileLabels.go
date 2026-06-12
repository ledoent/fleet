package tables

import (
	"database/sql"
	"fmt"

	"github.com/pkg/errors"
)

func init() {
	MigrationClient.AddMigration(Up_20260603101320, Down_20260603101320)
}

func Up_20260603101320(tx *sql.Tx) error {
	// Pre-check: ensure no orphaned label_id values exist that would cause the
	// new RESTRICT constraint to fail. In practice this shouldn't happen since
	// the previous ON DELETE SET NULL would have nulled them out, but guard
	// against any unexpected data inconsistency.
	for _, table := range []string{"mdm_configuration_profile_labels", "mdm_declaration_labels"} {
		var count int
		if err := tx.QueryRow(fmt.Sprintf(`
			SELECT COUNT(*) FROM %s WHERE label_id IS NOT NULL AND label_id NOT IN (SELECT id FROM labels)
		`, table)).Scan(&count); err != nil {
			return fmt.Errorf("checking orphaned label_id in %s: %w", table, err)
		}
		if count > 0 {
			return fmt.Errorf("cannot migrate: %s has %d row(s) with a label_id that no longer exists in labels", table, count)
		}
	}

	if isPostgres() {
		// PG: introspect FK names via pg_constraint and use DROP CONSTRAINT
		// (MySQL's information_schema columns / DROP FOREIGN KEY don't exist).
		for _, table := range []string{"mdm_configuration_profile_labels", "mdm_declaration_labels"} {
			// The PG baseline never carried the old ON DELETE SET NULL FK to
			// labels, so the drop is conditional rather than required.
			rows, err := tx.Query(`
				SELECT con.conname
				FROM pg_constraint con
				JOIN pg_class rel ON rel.oid = con.conrelid
				JOIN pg_class frel ON frel.oid = con.confrelid
				WHERE con.contype = 'f' AND rel.relname = $1 AND frel.relname = 'labels'
			`, table)
			if err != nil {
				return fmt.Errorf("querying %s foreign keys to labels: %w", table, err)
			}
			var connames []string
			for rows.Next() {
				var conname string
				if err := rows.Scan(&conname); err != nil {
					rows.Close()
					return fmt.Errorf("scanning %s foreign key name: %w", table, err)
				}
				connames = append(connames, conname)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return fmt.Errorf("iterating %s foreign key names: %w", table, err)
			}
			for _, conname := range connames {
				if _, err := tx.Exec(fmt.Sprintf(`ALTER TABLE %s DROP CONSTRAINT %s`, table, conname)); err != nil {
					return fmt.Errorf("dropping %s label_id foreign key: %w", table, err)
				}
			}
			newName := table + "_ibfk_label"
			if constraintExists(tx, table, newName) {
				continue
			}
			if _, err := tx.Exec(fmt.Sprintf(`ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (label_id) REFERENCES labels (id) ON DELETE RESTRICT`, table, newName)); err != nil {
				return fmt.Errorf("adding %s RESTRICT foreign key: %w", table, err)
			}
		}
		return nil
	}

	// mdm_configuration_profile_labels
	cpConstraints, err := constraintsForTable(tx, "mdm_configuration_profile_labels", map[string]struct{}{"labels": {}})
	if err != nil {
		return err
	}
	if len(cpConstraints) != 1 {
		return errors.New("mdm_configuration_profile_labels foreign key to labels not found")
	}
	if _, err := tx.Exec(fmt.Sprintf(`ALTER TABLE mdm_configuration_profile_labels DROP FOREIGN KEY %s`, cpConstraints[0])); err != nil {
		return fmt.Errorf("dropping mdm_configuration_profile_labels label_id foreign key: %w", err)
	}
	if _, err := tx.Exec(`ALTER TABLE mdm_configuration_profile_labels ADD CONSTRAINT mdm_configuration_profile_labels_ibfk_label FOREIGN KEY (label_id) REFERENCES labels (id) ON DELETE RESTRICT`); err != nil {
		return fmt.Errorf("adding mdm_configuration_profile_labels RESTRICT foreign key: %w", err)
	}

	// mdm_declaration_labels
	declConstraints, err := constraintsForTable(tx, "mdm_declaration_labels", map[string]struct{}{"labels": {}})
	if err != nil {
		return err
	}
	if len(declConstraints) != 1 {
		return errors.New("mdm_declaration_labels foreign key to labels not found")
	}
	if _, err := tx.Exec(fmt.Sprintf(`ALTER TABLE mdm_declaration_labels DROP FOREIGN KEY %s`, declConstraints[0])); err != nil {
		return fmt.Errorf("dropping mdm_declaration_labels label_id foreign key: %w", err)
	}
	if _, err := tx.Exec(`ALTER TABLE mdm_declaration_labels ADD CONSTRAINT mdm_declaration_labels_ibfk_label FOREIGN KEY (label_id) REFERENCES labels (id) ON DELETE RESTRICT`); err != nil {
		return fmt.Errorf("adding mdm_declaration_labels RESTRICT foreign key: %w", err)
	}

	return nil
}

func Down_20260603101320(tx *sql.Tx) error {
	return nil
}
