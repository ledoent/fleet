package tables

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20260807120050, Down_20260807120050)
	m := MigrationClient.Migrations[len(MigrationClient.Migrations)-1]
	m.UpFnPG = Up_20260807120050_PG
}

// Up_20260807120050_PG creates the same two tables with native boolean
// columns instead of the driver's tinyint(1)→smallint translation. The
// writers (apple_mdm_device_vitals.go) bind *bool via named params, which
// pgx encodes as boolean natively. This also sidesteps the
// awaiting_configuration name split: mdm_windows_enrollments.
// awaiting_configuration is a tri-state uint deliberately excluded from
// bool rewriting (see known_bool_col_splits.txt), so a smallint Apple
// column of the same name could never be covered by the name-keyed
// smallintBoolColumns machinery.
func Up_20260807120050_PG(tx *sql.Tx) error {
	_, err := tx.Exec(`
CREATE TABLE host_mdm_apple_device_vitals (
  host_uuid                         varchar(255) NOT NULL,
  udid                              varchar(255) DEFAULT NULL,
  model_number                      varchar(255) DEFAULT NULL,
  modem_firmware_version            varchar(255) DEFAULT NULL,
  supplemental_build_version        varchar(255) DEFAULT NULL,
  supplemental_os_version_extra     varchar(255) DEFAULT NULL,
  bluetooth_mac                     varchar(255) DEFAULT NULL,
  wifi_mac                          varchar(255) DEFAULT NULL,
  eas_device_identifier             varchar(255) DEFAULT NULL,
  itunes_store_account_hash         varchar(255) DEFAULT NULL,
  push_token                        bytea,
  battery_level                     double precision DEFAULT NULL,
  cellular_technology               integer DEFAULT NULL,
  app_analytics_enabled             boolean DEFAULT NULL,
  awaiting_configuration            boolean DEFAULT NULL,
  data_roaming_enabled              boolean DEFAULT NULL,
  diagnostic_submission_enabled     boolean DEFAULT NULL,
  is_cloud_backup_enabled           boolean DEFAULT NULL,
  is_device_locator_service_enabled boolean DEFAULT NULL,
  is_do_not_disturb_in_effect       boolean DEFAULT NULL,
  is_mdm_lost_mode_enabled          boolean DEFAULT NULL,
  is_network_tethered               boolean DEFAULT NULL,
  itunes_store_account_is_active    boolean DEFAULT NULL,
  personal_hotspot_enabled          boolean DEFAULT NULL,
  last_cloud_backup_date            timestamp DEFAULT NULL,
  accessibility_settings            jsonb DEFAULT NULL,
  organization_info                 jsonb DEFAULT NULL,
  mdm_options                       jsonb DEFAULT NULL,
  device_properties_attestation     jsonb DEFAULT NULL,
  created_at                        timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at                        timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (host_uuid)
)`)
	if err != nil {
		return fmt.Errorf("creating host_mdm_apple_device_vitals table: %w", err)
	}

	_, err = tx.Exec(`
CREATE TABLE host_mdm_apple_service_subscriptions (
  host_uuid                  varchar(255) NOT NULL,
  slot                       varchar(255) NOT NULL,
  carrier_settings_version   varchar(255) DEFAULT NULL,
  current_carrier_network   varchar(255) DEFAULT NULL,
  current_mcc                varchar(255) DEFAULT NULL,
  current_mnc                varchar(255) DEFAULT NULL,
  eid                        varchar(255) DEFAULT NULL,
  iccid                      varchar(255) DEFAULT NULL,
  imei                       varchar(255) DEFAULT NULL,
  is_data_preferred          boolean DEFAULT NULL,
  is_roaming                 boolean DEFAULT NULL,
  is_voice_preferred         boolean DEFAULT NULL,
  label                      varchar(255) DEFAULT NULL,
  label_id                   varchar(255) DEFAULT NULL,
  meid                       varchar(255) DEFAULT NULL,
  phone_number               varchar(255) DEFAULT NULL,
  subscriber_carrier_network varchar(255) DEFAULT NULL,
  created_at                 timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at                 timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (host_uuid, slot)
)`)
	if err != nil {
		return fmt.Errorf("creating host_mdm_apple_service_subscriptions table: %w", err)
	}
	return nil
}

// Up_20260807120050 creates host_mdm_apple_device_vitals and
// host_mdm_apple_service_subscriptions, which hold the additional
// iOS/iPadOS host vitals collected via the expanded DeviceInformation MDM
// command (see #49984). Both tables are keyed by host_uuid, no FK, and are
// registered in additionalHostRefsByUUID for host-deletion cleanup.
func Up_20260807120050(tx *sql.Tx) error {
	_, err := tx.Exec(`
CREATE TABLE host_mdm_apple_device_vitals (
  host_uuid                         varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  udid                              varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  model_number                      varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  modem_firmware_version            varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  supplemental_build_version        varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  supplemental_os_version_extra     varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  bluetooth_mac                     varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  wifi_mac                          varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  eas_device_identifier             varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  itunes_store_account_hash         varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  push_token                        blob,
  battery_level                     double DEFAULT NULL,
  cellular_technology               int DEFAULT NULL,
  app_analytics_enabled             tinyint(1) DEFAULT NULL,
  awaiting_configuration            tinyint(1) DEFAULT NULL,
  data_roaming_enabled              tinyint(1) DEFAULT NULL,
  diagnostic_submission_enabled     tinyint(1) DEFAULT NULL,
  is_cloud_backup_enabled           tinyint(1) DEFAULT NULL,
  is_device_locator_service_enabled tinyint(1) DEFAULT NULL,
  is_do_not_disturb_in_effect       tinyint(1) DEFAULT NULL,
  is_mdm_lost_mode_enabled          tinyint(1) DEFAULT NULL,
  is_network_tethered               tinyint(1) DEFAULT NULL,
  itunes_store_account_is_active    tinyint(1) DEFAULT NULL,
  personal_hotspot_enabled          tinyint(1) DEFAULT NULL,
  last_cloud_backup_date            datetime(6) DEFAULT NULL,
  accessibility_settings            json DEFAULT NULL,
  organization_info                 json DEFAULT NULL,
  mdm_options                       json DEFAULT NULL,
  device_properties_attestation     json DEFAULT NULL,
  created_at                        datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at                        datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (host_uuid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
`)
	if err != nil {
		return fmt.Errorf("creating host_mdm_apple_device_vitals table: %w", err)
	}

	_, err = tx.Exec(`
CREATE TABLE host_mdm_apple_service_subscriptions (
  host_uuid                  varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  slot                       varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  carrier_settings_version   varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  current_carrier_network   varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  current_mcc                varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  current_mnc                varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  eid                        varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  iccid                      varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  imei                       varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  is_data_preferred          tinyint(1) DEFAULT NULL,
  is_roaming                 tinyint(1) DEFAULT NULL,
  is_voice_preferred         tinyint(1) DEFAULT NULL,
  label                      varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  label_id                   varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  meid                       varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  phone_number               varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  subscriber_carrier_network varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  created_at                 datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at                 datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (host_uuid, slot)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
`)
	if err != nil {
		return fmt.Errorf("creating host_mdm_apple_service_subscriptions table: %w", err)
	}
	return nil
}

func Down_20260807120050(tx *sql.Tx) error {
	return nil
}
