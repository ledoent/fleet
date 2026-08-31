package tables

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20260723181404, Down_20260723181404)
}

func Up_20260723181404(tx *sql.Tx) error {
	if isPostgres() {
		// Native PG DDL: the token generated column needs a ::text cast on the
		// timestamp (PG won't coalesce a timestamp with ''), mirroring the
		// baseline's mdm_apple_declarations.checksum expression.
		for _, stmt := range []string{
			`CREATE TABLE IF NOT EXISTS mdm_apple_declaration_assets (
				asset_uuid varchar(37) NOT NULL,
				team_id int NOT NULL,
				identifier varchar(255) NOT NULL,
				name varchar(255) NOT NULL,
				raw_json text NOT NULL,
				secrets_updated_at timestamp(6) NULL DEFAULT NULL,
				token bytea GENERATED ALWAYS AS (decode(md5(raw_json || COALESCE(extract(epoch from secrets_updated_at)::text, '')), 'hex')) STORED,
				created_at timestamp(6) NOT NULL DEFAULT CURRENT_TIMESTAMP,
				uploaded_at timestamp(6) NULL DEFAULT NULL,
				PRIMARY KEY (asset_uuid),
				CONSTRAINT idx_mdm_apple_decl_asset_team_identifier UNIQUE (team_id, identifier),
				CONSTRAINT idx_mdm_apple_decl_asset_team_name UNIQUE (team_id, name)
			)`,
			`CREATE TABLE IF NOT EXISTS mdm_apple_declaration_asset_references (
				declaration_uuid varchar(37) NOT NULL,
				asset_uuid varchar(37) NOT NULL,
				PRIMARY KEY (declaration_uuid, asset_uuid),
				FOREIGN KEY (declaration_uuid) REFERENCES mdm_apple_declarations (declaration_uuid) ON DELETE CASCADE,
				FOREIGN KEY (asset_uuid) REFERENCES mdm_apple_declaration_assets (asset_uuid)
			)`,
			`ALTER TABLE host_mdm_apple_declarations ADD COLUMN IF NOT EXISTS assets_updated_at timestamp(6) NULL DEFAULT NULL`,
		} {
			if _, err := tx.Exec(stmt); err != nil {
				return fmt.Errorf("creating DDM asset tables: %w", err)
			}
		}
		return nil
	}
	_, err := tx.Exec(`
		CREATE TABLE mdm_apple_declaration_assets (
			asset_uuid varchar(37) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
			team_id int unsigned NOT NULL,
			identifier varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
			name varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
			raw_json mediumtext CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
			secrets_updated_at datetime(6) NULL DEFAULT NULL,
			-- generated token drives DDM ServerToken/sync; mirrors mdm_apple_declarations.token
			token binary(16) GENERATED ALWAYS AS (UNHEX(MD5(CONCAT(raw_json, IFNULL(secrets_updated_at, ''))))) STORED,
			created_at timestamp(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
			uploaded_at timestamp(6) NULL DEFAULT NULL,
			PRIMARY KEY (asset_uuid),
			UNIQUE KEY idx_mdm_apple_decl_asset_team_identifier (team_id, identifier),
			UNIQUE KEY idx_mdm_apple_decl_asset_team_name (team_id, name)
		);
	`)
	if err != nil {
		return fmt.Errorf("creating mdm_apple_declaration_assets table: %w", err)
	}

	_, err = tx.Exec(`
		CREATE TABLE mdm_apple_declaration_asset_references (
			declaration_uuid varchar(37) COLLATE utf8mb4_unicode_ci NOT NULL,
			asset_uuid varchar(37) COLLATE utf8mb4_unicode_ci NOT NULL,
			PRIMARY KEY (declaration_uuid, asset_uuid),
			
			-- deleting the referencing config drops the edge; the asset FK is RESTRICT (default)
			CONSTRAINT FOREIGN KEY (declaration_uuid) REFERENCES mdm_apple_declarations (declaration_uuid) ON DELETE CASCADE,
			CONSTRAINT FOREIGN KEY (asset_uuid) REFERENCES mdm_apple_declaration_assets (asset_uuid)
		);
	`)
	if err != nil {
		return fmt.Errorf("creating mdm_apple_declaration_asset_references table: %w", err)
	}

	_, err = tx.Exec(`
		-- ties a referencing config's per-host token to its assets (mirrors variables_updated_at); the reconciler
		-- sets it to max(referenced assets' uploaded_at) so an asset edit changes the config token and re-syncs the host
		ALTER TABLE host_mdm_apple_declarations
			ADD COLUMN assets_updated_at datetime(6) NULL DEFAULT NULL;
	`)
	if err != nil {
		return fmt.Errorf("adding assets_updated_at column to host_mdm_apple_declarations table: %w", err)
	}

	return nil
}

func Down_20260723181404(tx *sql.Tx) error {
	return nil
}
