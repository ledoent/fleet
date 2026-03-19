Generated 192 tables
est Baseline Schema
-- Auto-generated from MySQL test schema

CREATE TABLE IF NOT EXISTS "abm_tokens" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "organization_name" varchar(255) NOT NULL,
  "apple_id" varchar(255) NOT NULL,
  "terms_expired" boolean NOT NULL DEFAULT FALSE,
  "renew_at" timestamp NOT NULL,
  "token" bytea NOT NULL,
  "macos_default_team_id" int  DEFAULT NULL,
  "ios_default_team_id" int  DEFAULT NULL,
  "ipados_default_team_id" int  DEFAULT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_abm_tokens_organization_name" UNIQUE ("organization_name")
);

CREATE TABLE IF NOT EXISTS "activities" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "user_id" int  DEFAULT NULL,
  "user_name" varchar(255) DEFAULT NULL,
  "activity_type" varchar(255) NOT NULL,
  "details" jsonb DEFAULT NULL,
  "streamed" boolean NOT NULL DEFAULT FALSE,
  "user_email" varchar(255) NOT NULL DEFAULT '',
  "fleet_initiated" boolean NOT NULL DEFAULT FALSE,
  "host_only" boolean NOT NULL DEFAULT FALSE,
  PRIMARY KEY ("id")
);

CREATE TABLE IF NOT EXISTS "aggregated_stats" (
  "id" bigint  NOT NULL,
  "type" varchar(255) NOT NULL,
  "json_value" jsonb NOT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  "global_stats" boolean NOT NULL DEFAULT FALSE,
  PRIMARY KEY ("id","type","global_stats")
);

CREATE TABLE IF NOT EXISTS "android_app_configurations" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "application_id" varchar(255) NOT NULL,
  "team_id" int  DEFAULT NULL,
  "global_or_team_id" int NOT NULL DEFAULT '0',
  "configuration" jsonb NOT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_global_or_team_id_application_id" UNIQUE ("global_or_team_id","application_id")
);

CREATE TABLE IF NOT EXISTS "android_devices" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "host_id" int  NOT NULL,
  "device_id" varchar(32) NOT NULL,
  "enterprise_specific_id" varchar(64) DEFAULT NULL,
  "last_policy_sync_time" timestamp DEFAULT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  "applied_policy_id" varchar(100) DEFAULT NULL,
  "applied_policy_version" int DEFAULT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_android_devices_host_id" UNIQUE ("host_id"),
  CONSTRAINT "idx_android_devices_device_id" UNIQUE ("device_id"),
  CONSTRAINT "idx_android_devices_enterprise_specific_id" UNIQUE ("enterprise_specific_id")
);

CREATE TABLE IF NOT EXISTS "android_enterprises" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "signup_name" varchar(63) NOT NULL DEFAULT '',
  "enterprise_id" varchar(63) NOT NULL DEFAULT '',
  "created_at" timestamp DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp DEFAULT CURRENT_TIMESTAMP ,
  "signup_token" varchar(64) NOT NULL DEFAULT '',
  "pubsub_topic_id" varchar(64) NOT NULL DEFAULT '',
  "user_id" int  NOT NULL DEFAULT '0',
  PRIMARY KEY ("id")
);

CREATE TABLE IF NOT EXISTS "android_policy_requests" (
  "request_uuid" varchar(36) NOT NULL,
  "request_name" varchar(255) NOT NULL,
  "policy_id" varchar(100) NOT NULL,
  "payload" jsonb NOT NULL,
  "status_code" int NOT NULL,
  "error_details" text,
  "applied_policy_version" int DEFAULT NULL,
  "policy_version" int DEFAULT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("request_uuid")
);

CREATE TABLE IF NOT EXISTS "app_config_json" (
  "id" int  NOT NULL DEFAULT '1',
  "json_value" jsonb NOT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id")
);

CREATE TABLE IF NOT EXISTS "batch_activities" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "script_id" int  NOT NULL,
  "execution_id" varchar(255) NOT NULL,
  "user_id" int  DEFAULT NULL,
  "job_id" int  DEFAULT NULL,
  "status" varchar(255) DEFAULT NULL,
  "activity_type" varchar(255) DEFAULT NULL,
  "num_targeted" int  DEFAULT NULL,
  "num_pending" int  DEFAULT NULL,
  "num_ran" int  DEFAULT NULL,
  "num_errored" int  DEFAULT NULL,
  "num_incompatible" int  DEFAULT NULL,
  "num_canceled" int  DEFAULT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  "started_at" timestamp DEFAULT NULL,
  "finished_at" timestamp DEFAULT NULL,
  "canceled" boolean DEFAULT FALSE,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_batch_script_executions_execution_id" UNIQUE ("execution_id")
);

CREATE TABLE IF NOT EXISTS "batch_activity_host_results" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "batch_execution_id" varchar(255) NOT NULL,
  "host_id" int  NOT NULL,
  "host_execution_id" varchar(255) DEFAULT NULL,
  "error" varchar(255) DEFAULT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id"),
  CONSTRAINT "unique_batch_host_results_execution_hostid" UNIQUE ("batch_execution_id","host_id")
);

CREATE TABLE IF NOT EXISTS "ca_config_assets" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "type" text NOT NULL,
  "name" varchar(255) NOT NULL,
  "value" bytea NOT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_ca_config_assets_name" UNIQUE ("name")
);

CREATE TABLE IF NOT EXISTS "calendar_events" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "email" varchar(255) NOT NULL,
  "start_time" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "end_time" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "event" jsonb NOT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  "timezone" varchar(64) DEFAULT NULL,
  "uuid_bin" bytea NOT NULL,
  "uuid" text,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_one_calendar_event_per_email" UNIQUE ("email"),
  CONSTRAINT "idx_calendar_events_uuid_bin_unique" UNIQUE ("uuid_bin")
);

CREATE TABLE IF NOT EXISTS "carve_blocks" (
  "metadata_id" int  NOT NULL,
  "block_id" int NOT NULL,
  "data" bytea,
  PRIMARY KEY ("metadata_id","block_id")
);

CREATE TABLE IF NOT EXISTS "carve_metadata" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "host_id" int  NOT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "name" varchar(255) DEFAULT NULL,
  "block_count" int  NOT NULL,
  "block_size" int  NOT NULL,
  "carve_size" bigint  NOT NULL,
  "carve_id" varchar(64) NOT NULL,
  "request_id" varchar(64) NOT NULL,
  "session_id" varchar(255) NOT NULL,
  "expired" smallint DEFAULT '0',
  "max_block" int DEFAULT '-1',
  "error" text,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_session_id" UNIQUE ("session_id"),
  CONSTRAINT "idx_name" UNIQUE ("name")
);

CREATE TABLE IF NOT EXISTS "certificate_authorities" (
  "id" int NOT NULL GENERATED ALWAYS AS IDENTITY,
  "type" text NOT NULL,
  "name" varchar(255) NOT NULL,
  "url" text NOT NULL,
  "api_token_encrypted" bytea,
  "profile_id" varchar(255) DEFAULT NULL,
  "certificate_common_name" varchar(255) DEFAULT NULL,
  "certificate_user_principal_names" jsonb DEFAULT NULL,
  "certificate_seat_id" varchar(255) DEFAULT NULL,
  "admin_url" text,
  "username" varchar(255) DEFAULT NULL,
  "password_encrypted" bytea,
  "challenge_url" text,
  "challenge_encrypted" bytea,
  "client_id" varchar(255) DEFAULT NULL,
  "client_secret_encrypted" bytea,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_ca_type_name" UNIQUE ("type","name")
);

CREATE TABLE IF NOT EXISTS "certificate_templates" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "team_id" int  NOT NULL,
  "certificate_authority_id" int NOT NULL,
  "name" varchar(255) NOT NULL,
  "subject_name" text NOT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_cert_team_name" UNIQUE ("team_id","name")
);

CREATE TABLE IF NOT EXISTS "challenges" (
  "challenge" char(32) NOT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("challenge")
);

CREATE TABLE IF NOT EXISTS "conditional_access_scep_certificates" (
  "serial" bigint  NOT NULL,
  "host_id" int  NOT NULL,
  "name" varchar(64) NOT NULL,
  "not_valid_before" timestamp NOT NULL,
  "not_valid_after" timestamp NOT NULL,
  "certificate_pem" text NOT NULL,
  "revoked" boolean NOT NULL DEFAULT FALSE,
  "created_at" timestamp DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp DEFAULT CURRENT_TIMESTAMP ,
  CONSTRAINT "conditional_access_scep_certificates_chk_1" CHECK ((substr("certificate_pem",1,27) = '-----BEGIN CERTIFICATE-----')),
  PRIMARY KEY ("serial")
);

CREATE TABLE IF NOT EXISTS "conditional_access_scep_serials" (
  "serial" bigint GENERATED ALWAYS AS IDENTITY,
  "created_at" timestamp DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY ("serial")
);

CREATE TABLE IF NOT EXISTS "cron_stats" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "name" varchar(255) NOT NULL,
  "instance" varchar(255) NOT NULL,
  "stats_type" varchar(255) NOT NULL,
  "status" varchar(255) NOT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  "errors" jsonb DEFAULT NULL,
  PRIMARY KEY ("id")
);

CREATE TABLE IF NOT EXISTS "cve_meta" (
  "cve" varchar(20) NOT NULL,
  "cvss_score" double precision DEFAULT NULL,
  "epss_probability" double precision DEFAULT NULL,
  "cisa_known_exploit" boolean DEFAULT NULL,
  "published" timestamp NULL DEFAULT NULL,
  "description" text,
  PRIMARY KEY ("cve")
);

CREATE TABLE IF NOT EXISTS "default_team_config_json" (
  "id" int  NOT NULL DEFAULT '1',
  "json_value" jsonb NOT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  CONSTRAINT "default_team_config_id" CHECK (("id" = 1)),
  PRIMARY KEY ("id"),
  CONSTRAINT "id" UNIQUE ("id")
);

CREATE TABLE IF NOT EXISTS "distributed_query_campaign_targets" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "type" int DEFAULT NULL,
  "distributed_query_campaign_id" int  DEFAULT NULL,
  "target_id" int  DEFAULT NULL,
  PRIMARY KEY ("id")
);

CREATE TABLE IF NOT EXISTS "distributed_query_campaigns" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  "query_id" int  DEFAULT NULL,
  "status" int DEFAULT NULL,
  "user_id" int  DEFAULT NULL,
  PRIMARY KEY ("id")
);

CREATE TABLE IF NOT EXISTS "email_changes" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "user_id" int  NOT NULL,
  "token" varchar(128) NOT NULL,
  "new_email" varchar(255) NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_unique_email_changes_token" UNIQUE ("token")
);

CREATE TABLE IF NOT EXISTS "enroll_secrets" (
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "secret" varchar(255) NOT NULL,
  "team_id" int  DEFAULT NULL,
  PRIMARY KEY ("secret")
);

CREATE TABLE IF NOT EXISTS "eulas" (
  "id" int  NOT NULL,
  "token" varchar(36) DEFAULT NULL,
  "name" varchar(255) DEFAULT NULL,
  "bytes" bytea,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "sha256" bytea DEFAULT NULL,
  PRIMARY KEY ("id")
);

CREATE TABLE IF NOT EXISTS "fleet_maintained_apps" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "name" varchar(255) NOT NULL,
  "slug" varchar(255) NOT NULL,
  "platform" varchar(255) NOT NULL,
  "unique_identifier" varchar(255) NOT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_fleet_library_apps_token" UNIQUE ("slug")
);

CREATE TABLE IF NOT EXISTS "fleet_variables" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "name" varchar(255) NOT NULL DEFAULT '',
  "is_prefix" boolean NOT NULL DEFAULT FALSE,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_fleet_variables_name_is_prefix" UNIQUE ("name","is_prefix")
);

CREATE TABLE IF NOT EXISTS "host_activities" (
  "host_id" int  NOT NULL,
  "activity_id" int  NOT NULL,
  PRIMARY KEY ("host_id","activity_id")
);

CREATE TABLE IF NOT EXISTS "host_additional" (
  "host_id" int  NOT NULL,
  "additional" jsonb DEFAULT NULL,
  PRIMARY KEY ("host_id")
);

CREATE TABLE IF NOT EXISTS "host_batteries" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "host_id" int  NOT NULL,
  "serial_number" varchar(255) NOT NULL,
  "cycle_count" int NOT NULL,
  "health" varchar(40) NOT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_host_batteries_host_id_serial_number" UNIQUE ("host_id","serial_number")
);

CREATE TABLE IF NOT EXISTS "host_calendar_events" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "host_id" int  NOT NULL,
  "calendar_event_id" int  NOT NULL,
  "webhook_status" smallint NOT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_one_calendar_event_per_host" UNIQUE ("host_id")
);

CREATE TABLE IF NOT EXISTS "host_certificate_sources" (
  "id" bigint GENERATED ALWAYS AS IDENTITY,
  "host_certificate_id" bigint  NOT NULL,
  "source" text NOT NULL,
  "username" varchar(255) NOT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_host_certificate_sources_unique" UNIQUE ("host_certificate_id","source","username")
);

CREATE TABLE IF NOT EXISTS "host_certificate_templates" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "host_uuid" varchar(255) NOT NULL,
  "certificate_template_id" int  NOT NULL,
  "fleet_challenge" char(32) DEFAULT NULL,
  "status" varchar(20) NOT NULL DEFAULT 'pending',
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  "detail" text,
  "operation_type" varchar(20) NOT NULL DEFAULT 'install',
  "name" varchar(255) NOT NULL,
  "uuid" bytea DEFAULT NULL,
  "not_valid_before" timestamp DEFAULT NULL,
  "not_valid_after" timestamp DEFAULT NULL,
  "serial" varchar(40) DEFAULT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_host_certificate_templates_host_template" UNIQUE ("host_uuid","certificate_template_id")
);

CREATE TABLE IF NOT EXISTS "host_certificates" (
  "id" bigint GENERATED ALWAYS AS IDENTITY,
  "host_id" int  NOT NULL,
  "not_valid_after" timestamp NOT NULL,
  "not_valid_before" timestamp NOT NULL,
  "certificate_authority" boolean NOT NULL,
  "common_name" varchar(255) NOT NULL,
  "key_algorithm" varchar(255) NOT NULL,
  "key_strength" int NOT NULL,
  "key_usage" varchar(255) NOT NULL,
  "serial" varchar(255) NOT NULL,
  "signing_algorithm" varchar(255) NOT NULL,
  "subject_country" varchar(32) NOT NULL,
  "subject_org" varchar(255) NOT NULL,
  "subject_org_unit" varchar(255) NOT NULL,
  "subject_common_name" varchar(255) NOT NULL,
  "issuer_country" varchar(32) NOT NULL,
  "issuer_org" varchar(255) NOT NULL,
  "issuer_org_unit" varchar(255) NOT NULL,
  "issuer_common_name" varchar(255) NOT NULL,
  "sha1_sum" bytea NOT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "deleted_at" timestamp DEFAULT NULL,
  PRIMARY KEY ("id")
);

CREATE TABLE IF NOT EXISTS "host_conditional_access" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "host_id" int  NOT NULL,
  "bypassed_at" timestamp NULL DEFAULT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_host_conditional_access_host_id" UNIQUE ("host_id")
);

CREATE TABLE IF NOT EXISTS "host_dep_assignments" (
  "host_id" int  NOT NULL,
  "added_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "deleted_at" timestamp NULL DEFAULT NULL,
  "profile_uuid" varchar(37) DEFAULT NULL,
  "assign_profile_response" varchar(15) DEFAULT NULL,
  "response_updated_at" timestamp NULL DEFAULT NULL,
  "retry_job_id" int  NOT NULL DEFAULT '0',
  "abm_token_id" int  DEFAULT NULL,
  "mdm_migration_deadline" timestamp NULL DEFAULT NULL,
  "mdm_migration_completed" timestamp NULL DEFAULT NULL,
  PRIMARY KEY ("host_id")
);

CREATE TABLE IF NOT EXISTS "host_device_auth" (
  "host_id" int  NOT NULL,
  "token" varchar(255) NOT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("host_id"),
  CONSTRAINT "idx_host_device_auth_token" UNIQUE ("token")
);

CREATE TABLE IF NOT EXISTS "host_disk_encryption_keys" (
  "host_id" int  NOT NULL,
  "base64_encrypted" text NOT NULL,
  "base64_encrypted_salt" varchar(255) NOT NULL DEFAULT '',
  "key_slot" smallint  DEFAULT NULL,
  "decryptable" boolean DEFAULT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  "reset_requested" boolean NOT NULL DEFAULT FALSE,
  "client_error" varchar(255) NOT NULL DEFAULT '',
  PRIMARY KEY ("host_id")
);

CREATE TABLE IF NOT EXISTS "host_disk_encryption_keys_archive" (
  "id" bigint GENERATED ALWAYS AS IDENTITY,
  "host_id" int  NOT NULL,
  "hardware_serial" varchar(255) NOT NULL DEFAULT '',
  "base64_encrypted" text NOT NULL,
  "base64_encrypted_salt" varchar(255) NOT NULL DEFAULT '',
  "key_slot" smallint  DEFAULT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY ("id")
);

CREATE TABLE IF NOT EXISTS "host_disks" (
  "host_id" int  NOT NULL,
  "gigs_disk_space_available" decimal(10,2) NOT NULL DEFAULT '0.00',
  "percent_disk_space_available" decimal(10,2) NOT NULL DEFAULT '0.00',
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  "encrypted" boolean DEFAULT NULL,
  "gigs_total_disk_space" decimal(10,2) NOT NULL DEFAULT '0.00',
  "tpm_pin_set" boolean DEFAULT FALSE,
  "gigs_all_disk_space" decimal(10,2) DEFAULT NULL,
  PRIMARY KEY ("host_id")
);

CREATE TABLE IF NOT EXISTS "host_display_names" (
  "host_id" int  NOT NULL,
  "display_name" varchar(255) NOT NULL,
  PRIMARY KEY ("host_id")
);

CREATE TABLE IF NOT EXISTS "host_emails" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "host_id" int  NOT NULL,
  "email" varchar(255) NOT NULL,
  "source" varchar(255) NOT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id")
);

CREATE TABLE IF NOT EXISTS "host_identity_scep_certificates" (
  "serial" bigint  NOT NULL,
  "host_id" int  DEFAULT NULL,
  "name" varchar(255) NOT NULL,
  "not_valid_before" timestamp NOT NULL,
  "not_valid_after" timestamp NOT NULL,
  "certificate_pem" text NOT NULL,
  "public_key_raw" bytea NOT NULL,
  "revoked" boolean NOT NULL DEFAULT FALSE,
  "created_at" timestamp DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp DEFAULT CURRENT_TIMESTAMP ,
  CONSTRAINT "host_identity_scep_certificates_chk_1" CHECK ((substr("certificate_pem",1,27) = '-----BEGIN CERTIFICATE-----')),
  PRIMARY KEY ("serial")
);

CREATE TABLE IF NOT EXISTS "host_identity_scep_serials" (
  "serial" bigint GENERATED ALWAYS AS IDENTITY,
  "created_at" timestamp DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY ("serial")
);

CREATE TABLE IF NOT EXISTS "host_in_house_software_installs" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "host_id" int  NOT NULL,
  "in_house_app_id" int  NOT NULL,
  "command_uuid" varchar(127) NOT NULL,
  "user_id" int  DEFAULT NULL,
  "platform" varchar(10) NOT NULL,
  "removed" smallint NOT NULL DEFAULT '0',
  "canceled" smallint NOT NULL DEFAULT '0',
  "verification_command_uuid" varchar(127) DEFAULT NULL,
  "verification_at" timestamp DEFAULT NULL,
  "verification_failed_at" timestamp DEFAULT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  "self_service" boolean NOT NULL DEFAULT FALSE,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_host_in_house_software_installs_command_uuid" UNIQUE ("command_uuid")
);

CREATE TABLE IF NOT EXISTS "host_issues" (
  "host_id" int  NOT NULL,
  "failing_policies_count" int  NOT NULL DEFAULT '0',
  "critical_vulnerabilities_count" int  NOT NULL DEFAULT '0',
  "total_issues_count" int  NOT NULL DEFAULT '0',
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("host_id")
);

CREATE TABLE IF NOT EXISTS "host_last_known_locations" (
  "host_id" int  NOT NULL,
  "latitude" decimal(10,8) DEFAULT NULL,
  "longitude" decimal(11,8) DEFAULT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("host_id")
);

CREATE TABLE IF NOT EXISTS "host_mdm" (
  "host_id" int  NOT NULL,
  "enrolled" boolean NOT NULL DEFAULT FALSE,
  "server_url" varchar(255) NOT NULL DEFAULT '',
  "installed_from_dep" boolean NOT NULL DEFAULT FALSE,
  "mdm_id" int  DEFAULT NULL,
  "is_server" boolean DEFAULT NULL,
  "fleet_enroll_ref" varchar(36) NOT NULL DEFAULT '',
  "enrollment_status" text,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  "is_personal_enrollment" boolean NOT NULL DEFAULT FALSE,
  PRIMARY KEY ("host_id")
);

CREATE TABLE IF NOT EXISTS "host_mdm_actions" (
  "host_id" int  NOT NULL,
  "lock_ref" varchar(36) DEFAULT NULL,
  "wipe_ref" varchar(36) DEFAULT NULL,
  "unlock_pin" varchar(6) DEFAULT NULL,
  "unlock_ref" varchar(36) DEFAULT NULL,
  "fleet_platform" varchar(255) NOT NULL DEFAULT '',
  PRIMARY KEY ("host_id")
);

CREATE TABLE IF NOT EXISTS "host_mdm_android_profiles" (
  "host_uuid" varchar(255) NOT NULL,
  "status" varchar(20) DEFAULT NULL,
  "operation_type" varchar(20) DEFAULT NULL,
  "detail" text,
  "profile_uuid" varchar(37) NOT NULL DEFAULT '',
  "profile_name" varchar(255) NOT NULL DEFAULT '',
  "policy_request_uuid" varchar(36) DEFAULT NULL,
  "device_request_uuid" varchar(36) DEFAULT NULL,
  "request_fail_count" smallint  NOT NULL DEFAULT '0',
  "included_in_policy_version" int DEFAULT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  "can_reverify" boolean NOT NULL DEFAULT FALSE,
  PRIMARY KEY ("host_uuid","profile_uuid")
);

CREATE TABLE IF NOT EXISTS "host_mdm_apple_awaiting_configuration" (
  "host_uuid" varchar(255) NOT NULL,
  "awaiting_configuration" boolean NOT NULL DEFAULT FALSE,
  PRIMARY KEY ("host_uuid")
);

CREATE TABLE IF NOT EXISTS "host_mdm_apple_bootstrap_packages" (
  "host_uuid" varchar(127) NOT NULL,
  "command_uuid" varchar(127) DEFAULT NULL,
  "skipped" boolean NOT NULL DEFAULT FALSE,
  CONSTRAINT "ck_skipped_or_commanduuid" CHECK ((("skipped" = 0) = ("command_uuid" is not null))),
  PRIMARY KEY ("host_uuid")
);

CREATE TABLE IF NOT EXISTS "host_mdm_apple_declarations" (
  "host_uuid" varchar(255) NOT NULL,
  "status" varchar(20) DEFAULT NULL,
  "operation_type" varchar(20) DEFAULT NULL,
  "detail" text,
  "token" bytea NOT NULL,
  "declaration_uuid" varchar(37) NOT NULL DEFAULT '',
  "declaration_identifier" varchar(255) NOT NULL,
  "declaration_name" varchar(255) NOT NULL DEFAULT '',
  "secrets_updated_at" timestamp DEFAULT NULL,
  "resync" boolean NOT NULL DEFAULT FALSE,
  "scope" text NOT NULL DEFAULT 'System',
  PRIMARY KEY ("host_uuid","declaration_uuid")
);

CREATE TABLE IF NOT EXISTS "host_mdm_apple_profiles" (
  "profile_identifier" varchar(255) NOT NULL,
  "host_uuid" varchar(255) NOT NULL,
  "status" varchar(20) DEFAULT NULL,
  "operation_type" varchar(20) DEFAULT NULL,
  "detail" text,
  "command_uuid" varchar(127) NOT NULL,
  "profile_name" varchar(255) NOT NULL DEFAULT '',
  "checksum" bytea NOT NULL,
  "retries" smallint  NOT NULL DEFAULT '0',
  "profile_uuid" varchar(37) NOT NULL DEFAULT '',
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  "secrets_updated_at" timestamp DEFAULT NULL,
  "ignore_error" boolean NOT NULL DEFAULT FALSE,
  "variables_updated_at" timestamp DEFAULT NULL,
  "scope" text NOT NULL DEFAULT 'System',
  PRIMARY KEY ("host_uuid","profile_uuid")
);

CREATE TABLE IF NOT EXISTS "host_mdm_commands" (
  "host_id" int  NOT NULL,
  "command_type" varchar(31) NOT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("host_id","command_type")
);

CREATE TABLE IF NOT EXISTS "host_mdm_idp_accounts" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "host_uuid" varchar(255) NOT NULL,
  "account_uuid" varchar(36) NOT NULL DEFAULT '',
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_host_mdm_idp_accounts" UNIQUE ("host_uuid")
);

CREATE TABLE IF NOT EXISTS "host_mdm_managed_certificates" (
  "host_uuid" varchar(255) NOT NULL,
  "profile_uuid" varchar(37) NOT NULL,
  "type" text NOT NULL DEFAULT 'ndes',
  "ca_name" varchar(255) NOT NULL DEFAULT 'NDES',
  "challenge_retrieved_at" timestamp NULL DEFAULT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  "not_valid_after" timestamp DEFAULT NULL,
  "serial" varchar(40) DEFAULT NULL,
  "not_valid_before" timestamp DEFAULT NULL,
  PRIMARY KEY ("host_uuid","profile_uuid","ca_name")
);

CREATE TABLE IF NOT EXISTS "host_mdm_windows_profiles" (
  "host_uuid" varchar(255) NOT NULL,
  "status" varchar(20) DEFAULT NULL,
  "operation_type" varchar(20) DEFAULT NULL,
  "detail" text,
  "command_uuid" varchar(127) NOT NULL,
  "profile_name" varchar(255) NOT NULL DEFAULT '',
  "retries" smallint  NOT NULL DEFAULT '0',
  "profile_uuid" varchar(37) NOT NULL DEFAULT '',
  "checksum" bytea NOT NULL DEFAULT '0\0\0\0\0\0\0\0\0\0\0\0\0\0\0\0',
  "secrets_updated_at" timestamp DEFAULT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("host_uuid","profile_uuid")
);

CREATE TABLE IF NOT EXISTS "host_munki_info" (
  "host_id" int  NOT NULL,
  "version" varchar(255) NOT NULL DEFAULT '',
  "deleted_at" timestamp NULL DEFAULT NULL,
  PRIMARY KEY ("host_id")
);

CREATE TABLE IF NOT EXISTS "host_munki_issues" (
  "host_id" int  NOT NULL,
  "munki_issue_id" int  NOT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY ("host_id","munki_issue_id")
);

CREATE TABLE IF NOT EXISTS "host_operating_system" (
  "host_id" int  NOT NULL,
  "os_id" int  NOT NULL,
  PRIMARY KEY ("host_id")
);

CREATE TABLE IF NOT EXISTS "host_orbit_info" (
  "host_id" int  NOT NULL,
  "version" varchar(50) NOT NULL,
  "desktop_version" varchar(50) DEFAULT NULL,
  "scripts_enabled" boolean DEFAULT NULL,
  PRIMARY KEY ("host_id")
);

CREATE TABLE IF NOT EXISTS "host_scim_user" (
  "host_id" int  NOT NULL,
  "scim_user_id" int  NOT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY ("host_id")
);

CREATE TABLE IF NOT EXISTS "host_script_results" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "host_id" int  NOT NULL,
  "execution_id" varchar(255) NOT NULL,
  "output" text NOT NULL,
  "runtime" int  NOT NULL DEFAULT '0',
  "exit_code" int DEFAULT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  "script_id" int  DEFAULT NULL,
  "user_id" int  DEFAULT NULL,
  "sync_request" boolean NOT NULL DEFAULT FALSE,
  "script_content_id" int  DEFAULT NULL,
  "host_deleted_at" timestamp NULL DEFAULT NULL,
  "timeout" int DEFAULT NULL,
  "policy_id" int  DEFAULT NULL,
  "setup_experience_script_id" int  DEFAULT NULL,
  "is_internal" boolean DEFAULT FALSE,
  "canceled" boolean NOT NULL DEFAULT FALSE,
  "attempt_number" int DEFAULT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_host_script_results_execution_id" UNIQUE ("execution_id")
);

CREATE TABLE IF NOT EXISTS "host_seen_times" (
  "host_id" int  NOT NULL,
  "seen_time" timestamp NULL DEFAULT NULL,
  PRIMARY KEY ("host_id")
);

CREATE TABLE IF NOT EXISTS "host_software" (
  "host_id" int  NOT NULL,
  "software_id" bigint  NOT NULL,
  "last_opened_at" timestamp NULL DEFAULT NULL,
  PRIMARY KEY ("host_id","software_id")
);

CREATE TABLE IF NOT EXISTS "host_software_installed_paths" (
  "id" bigint GENERATED ALWAYS AS IDENTITY,
  "host_id" int  NOT NULL,
  "software_id" bigint  NOT NULL,
  "installed_path" text NOT NULL,
  "team_identifier" varchar(10) NOT NULL DEFAULT '',
  "cdhash_sha256" char(64) DEFAULT NULL,
  "executable_sha256" char(64) DEFAULT NULL,
  "executable_path" text,
  PRIMARY KEY ("id")
);

CREATE TABLE IF NOT EXISTS "host_software_installs" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "execution_id" varchar(255) NOT NULL,
  "host_id" int  NOT NULL,
  "software_installer_id" int  DEFAULT NULL,
  "pre_install_query_output" text,
  "install_script_output" text,
  "install_script_exit_code" int DEFAULT NULL,
  "post_install_script_output" text,
  "post_install_script_exit_code" int DEFAULT NULL,
  "user_id" int  DEFAULT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  "self_service" boolean NOT NULL DEFAULT FALSE,
  "host_deleted_at" timestamp NULL DEFAULT NULL,
  "removed" smallint NOT NULL DEFAULT '0',
  "uninstall_script_output" text,
  "uninstall_script_exit_code" int DEFAULT NULL,
  "uninstall" smallint  NOT NULL DEFAULT '0',
  "status" text,
  "policy_id" int  DEFAULT NULL,
  "installer_filename" varchar(255) NOT NULL DEFAULT '[deleted installer]',
  "version" varchar(255) NOT NULL DEFAULT 'unknown',
  "software_title_id" int  DEFAULT NULL,
  "software_title_name" varchar(255) NOT NULL DEFAULT '[deleted title]',
  "execution_status" text,
  "canceled" boolean NOT NULL DEFAULT FALSE,
  "attempt_number" int DEFAULT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_host_software_installs_execution_id" UNIQUE ("execution_id")
);

CREATE TABLE IF NOT EXISTS "host_updates" (
  "host_id" int  NOT NULL,
  "software_updated_at" timestamp NULL DEFAULT NULL,
  PRIMARY KEY ("host_id")
);

CREATE TABLE IF NOT EXISTS "host_users" (
  "host_id" int  NOT NULL,
  "uid" int  NOT NULL,
  "username" varchar(255) NOT NULL,
  "groupname" varchar(255) DEFAULT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "removed_at" timestamp NULL DEFAULT NULL,
  "user_type" varchar(255) DEFAULT NULL,
  "shell" varchar(255) DEFAULT '',
  PRIMARY KEY ("host_id","uid","username")
);

CREATE TABLE IF NOT EXISTS "host_vpp_software_installs" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "host_id" int  NOT NULL,
  "adam_id" varchar(255) NOT NULL,
  "command_uuid" varchar(127) NOT NULL,
  "user_id" int  DEFAULT NULL,
  "self_service" boolean NOT NULL DEFAULT FALSE,
  "associated_event_id" varchar(36) DEFAULT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  "platform" varchar(10) NOT NULL,
  "removed" smallint NOT NULL DEFAULT '0',
  "vpp_token_id" int  DEFAULT NULL,
  "policy_id" int  DEFAULT NULL,
  "canceled" boolean NOT NULL DEFAULT FALSE,
  "verification_command_uuid" varchar(127) DEFAULT NULL,
  "verification_at" timestamp DEFAULT NULL,
  "verification_failed_at" timestamp DEFAULT NULL,
  "retry_count" int NOT NULL DEFAULT '0',
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_host_vpp_software_installs_command_uuid" UNIQUE ("command_uuid")
);

CREATE TABLE IF NOT EXISTS "hosts" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "osquery_host_id" varchar(255) DEFAULT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  "detail_updated_at" timestamp NULL DEFAULT NULL,
  "node_key" varchar(255) DEFAULT NULL,
  "hostname" varchar(255) NOT NULL DEFAULT '',
  "uuid" varchar(255) NOT NULL DEFAULT '',
  "platform" varchar(255) NOT NULL DEFAULT '',
  "osquery_version" varchar(255) NOT NULL DEFAULT '',
  "os_version" varchar(255) NOT NULL DEFAULT '',
  "build" varchar(255) NOT NULL DEFAULT '',
  "platform_like" varchar(255) NOT NULL DEFAULT '',
  "code_name" varchar(255) NOT NULL DEFAULT '',
  "uptime" bigint NOT NULL DEFAULT '0',
  "memory" bigint NOT NULL DEFAULT '0',
  "cpu_type" varchar(255) NOT NULL DEFAULT '',
  "cpu_subtype" varchar(255) NOT NULL DEFAULT '',
  "cpu_brand" varchar(255) NOT NULL DEFAULT '',
  "cpu_physical_cores" int NOT NULL DEFAULT '0',
  "cpu_logical_cores" int NOT NULL DEFAULT '0',
  "hardware_vendor" varchar(255) NOT NULL DEFAULT '',
  "hardware_model" varchar(255) NOT NULL DEFAULT '',
  "hardware_version" varchar(255) NOT NULL DEFAULT '',
  "hardware_serial" varchar(255) NOT NULL DEFAULT '',
  "computer_name" varchar(255) NOT NULL DEFAULT '',
  "primary_ip_id" int  DEFAULT NULL,
  "distributed_interval" int DEFAULT '0',
  "logger_tls_period" int DEFAULT '0',
  "config_tls_refresh" int DEFAULT '0',
  "primary_ip" varchar(45) NOT NULL DEFAULT '',
  "primary_mac" varchar(17) NOT NULL DEFAULT '',
  "label_updated_at" timestamp NOT NULL DEFAULT '2000-01-01 00:00:00',
  "last_enrolled_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "refetch_requested" boolean NOT NULL DEFAULT FALSE,
  "team_id" int  DEFAULT NULL,
  "policy_updated_at" timestamp NOT NULL DEFAULT '2000-01-01 00:00:00',
  "public_ip" varchar(45) NOT NULL DEFAULT '',
  "orbit_node_key" varchar(255) DEFAULT NULL,
  "refetch_critical_queries_until" timestamp NULL DEFAULT NULL,
  "last_restarted_at" timestamp DEFAULT '0001-01-01 00:00:00.000000',
  "timezone" varchar(255) DEFAULT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_osquery_host_id" UNIQUE ("osquery_host_id"),
  CONSTRAINT "idx_host_unique_nodekey" UNIQUE ("node_key"),
  CONSTRAINT "idx_host_unique_orbitnodekey" UNIQUE ("orbit_node_key")
);

CREATE TABLE IF NOT EXISTS "in_house_app_labels" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "in_house_app_id" int  NOT NULL,
  "label_id" int  NOT NULL,
  "exclude" boolean NOT NULL DEFAULT FALSE,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id"),
  CONSTRAINT "id_in_house_app_labels_in_house_app_id_label_id" UNIQUE ("in_house_app_id","label_id")
);

CREATE TABLE IF NOT EXISTS "in_house_app_software_categories" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "software_category_id" int  NOT NULL,
  "in_house_app_id" int  NOT NULL,
  "created_at" timestamp DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_unique_in_house_app_id_software_category_id" UNIQUE ("in_house_app_id","software_category_id")
);

CREATE TABLE IF NOT EXISTS "in_house_app_upcoming_activities" (
  "upcoming_activity_id" bigint  NOT NULL,
  "in_house_app_id" int  NOT NULL,
  "software_title_id" int  DEFAULT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("upcoming_activity_id")
);

CREATE TABLE IF NOT EXISTS "in_house_apps" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "title_id" int  DEFAULT NULL,
  "team_id" int  DEFAULT NULL,
  "global_or_team_id" int  NOT NULL DEFAULT '0',
  "filename" varchar(255) NOT NULL DEFAULT '',
  "version" varchar(255) NOT NULL DEFAULT '',
  "storage_id" varchar(64) NOT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  "platform" varchar(10) NOT NULL,
  "bundle_identifier" varchar(255) NOT NULL DEFAULT '',
  "self_service" boolean NOT NULL DEFAULT FALSE,
  "url" varchar(4095) NOT NULL DEFAULT '',
  PRIMARY KEY ("id"),
  CONSTRAINT "global_or_team_id" UNIQUE ("global_or_team_id","filename","platform")
);

CREATE TABLE IF NOT EXISTS "invite_teams" (
  "invite_id" int  NOT NULL,
  "team_id" int  NOT NULL,
  "role" varchar(64) NOT NULL,
  PRIMARY KEY ("invite_id","team_id")
);

CREATE TABLE IF NOT EXISTS "invites" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  "invited_by" int  NOT NULL,
  "email" varchar(255) NOT NULL,
  "name" varchar(255) DEFAULT NULL,
  "position" varchar(255) DEFAULT NULL,
  "token" varchar(255) NOT NULL,
  "sso_enabled" boolean NOT NULL DEFAULT FALSE,
  "global_role" varchar(64) DEFAULT NULL,
  "mfa_enabled" boolean NOT NULL DEFAULT FALSE,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_invite_unique_email" UNIQUE ("email"),
  CONSTRAINT "idx_invite_unique_key" UNIQUE ("token")
);

CREATE TABLE IF NOT EXISTS "jobs" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  "name" varchar(255) NOT NULL,
  "args" jsonb DEFAULT NULL,
  "state" varchar(255) NOT NULL,
  "retries" int NOT NULL DEFAULT '0',
  "error" text,
  "not_before" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY ("id")
);

CREATE TABLE IF NOT EXISTS "kernel_host_counts" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "software_title_id" int  DEFAULT NULL,
  "software_id" int  DEFAULT NULL,
  "os_version_id" int  DEFAULT NULL,
  "hosts_count" int  NOT NULL,
  "team_id" int  NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_kernels_unique_mapping" UNIQUE ("os_version_id","team_id","software_id")
);

CREATE TABLE IF NOT EXISTS "label_membership" (
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  "label_id" int  NOT NULL,
  "host_id" int  NOT NULL,
  PRIMARY KEY ("host_id","label_id")
);

CREATE TABLE IF NOT EXISTS "labels" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  "name" varchar(255) NOT NULL,
  "description" varchar(255) DEFAULT NULL,
  "query" text NOT NULL,
  "platform" varchar(255) DEFAULT NULL,
  "label_type" int  NOT NULL DEFAULT '1',
  "label_membership_type" int  NOT NULL DEFAULT '0',
  "author_id" int  DEFAULT NULL,
  "criteria" jsonb DEFAULT NULL,
  "team_id" int  DEFAULT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_label_unique_name" UNIQUE ("name")
);

CREATE TABLE IF NOT EXISTS "legacy_host_filevault_profiles" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "host_uuid" varchar(36) NOT NULL,
  "status" varchar(20) NOT NULL,
  "operation_type" varchar(20) NOT NULL,
  "profile_uuid" varchar(37) NOT NULL,
  "detail" text,
  "command_uuid" varchar(127) NOT NULL,
  "scope" text NOT NULL DEFAULT 'System',
  "created_at" timestamp NOT NULL,
  "updated_at" timestamp NOT NULL,
  PRIMARY KEY ("id")
);

CREATE TABLE IF NOT EXISTS "legacy_host_mdm_enroll_refs" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "host_uuid" varchar(255) NOT NULL,
  "enroll_ref" varchar(36) NOT NULL,
  PRIMARY KEY ("id")
);

CREATE TABLE IF NOT EXISTS "legacy_host_mdm_idp_accounts" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "host_uuid" varchar(255) NOT NULL,
  "email" varchar(255) NOT NULL,
  "account_uuid" varchar(36) DEFAULT NULL,
  "host_id" int  DEFAULT NULL,
  "email_id" int  DEFAULT NULL,
  "email_created_at" timestamp DEFAULT NULL,
  "email_updated_at" timestamp DEFAULT NULL,
  PRIMARY KEY ("id")
);

CREATE TABLE IF NOT EXISTS "locks" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "name" varchar(255) DEFAULT NULL,
  "owner" varchar(255) DEFAULT NULL,
  "expires_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_name" UNIQUE ("name")
);

CREATE TABLE IF NOT EXISTS "mdm_android_configuration_profiles" (
  "profile_uuid" varchar(37) NOT NULL DEFAULT '',
  "team_id" int  NOT NULL DEFAULT '0',
  "name" varchar(255) NOT NULL,
  "raw_json" jsonb NOT NULL,
  "GENERATED ALWAYS AS IDENTITY" bigint GENERATED ALWAYS AS IDENTITY,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "uploaded_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY ("profile_uuid"),
  CONSTRAINT UNIQUE (),
  CONSTRAINT "idx_mdm_android_configuration_profiles_team_id_name" UNIQUE ("team_id","name")
);

CREATE TABLE IF NOT EXISTS "mdm_apple_bootstrap_packages" (
  "team_id" int  NOT NULL,
  "name" varchar(255) DEFAULT NULL,
  "sha256" bytea NOT NULL,
  "bytes" bytea,
  "token" varchar(36) DEFAULT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("team_id"),
  CONSTRAINT "idx_token" UNIQUE ("token")
);

CREATE TABLE IF NOT EXISTS "mdm_apple_configuration_profiles" (
  "profile_id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "team_id" int  NOT NULL DEFAULT '0',
  "identifier" varchar(255) NOT NULL,
  "name" varchar(255) NOT NULL,
  "mobileconfig" bytea NOT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "uploaded_at" timestamp NULL DEFAULT NULL,
  "checksum" bytea NOT NULL,
  "profile_uuid" varchar(37) NOT NULL DEFAULT '',
  "secrets_updated_at" timestamp DEFAULT NULL,
  "scope" text NOT NULL DEFAULT 'System',
  PRIMARY KEY ("profile_uuid"),
  CONSTRAINT "idx_mdm_apple_config_prof_team_identifier" UNIQUE ("team_id","identifier"),
  CONSTRAINT "idx_mdm_apple_config_prof_team_name" UNIQUE ("team_id","name"),
  CONSTRAINT "idx_mdm_apple_config_prof_id" UNIQUE ("profile_id")
);

CREATE TABLE IF NOT EXISTS "mdm_apple_declaration_activation_references" (
  "declaration_uuid" varchar(37) NOT NULL DEFAULT '',
  "reference" varchar(37) NOT NULL DEFAULT '',
  PRIMARY KEY ("declaration_uuid","reference")
);

CREATE TABLE IF NOT EXISTS "mdm_apple_declarations" (
  "declaration_uuid" varchar(37) NOT NULL DEFAULT '',
  "team_id" int  NOT NULL DEFAULT '0',
  "identifier" varchar(255) NOT NULL,
  "name" varchar(255) NOT NULL,
  "raw_json" text NOT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "uploaded_at" timestamp NULL DEFAULT NULL,
  "GENERATED ALWAYS AS IDENTITY" bigint GENERATED ALWAYS AS IDENTITY,
  "secrets_updated_at" timestamp DEFAULT NULL,
  "token" bytea,
  "scope" text NOT NULL DEFAULT 'System',
  PRIMARY KEY ("declaration_uuid"),
  CONSTRAINT "idx_mdm_apple_declaration_team_identifier" UNIQUE ("team_id","identifier"),
  CONSTRAINT "idx_mdm_apple_declaration_team_name" UNIQUE ("team_id","name"),
  CONSTRAINT UNIQUE ()
);

CREATE TABLE IF NOT EXISTS "mdm_apple_declarative_requests" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "enrollment_id" varchar(255) NOT NULL,
  "message_type" varchar(255) NOT NULL,
  "raw_json" text,
  PRIMARY KEY ("id")
);

CREATE TABLE IF NOT EXISTS "mdm_apple_default_setup_assistants" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "team_id" int  DEFAULT NULL,
  "global_or_team_id" int  NOT NULL DEFAULT '0',
  "profile_uuid" varchar(255) NOT NULL DEFAULT '',
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  "abm_token_id" int  DEFAULT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_mdm_default_setup_assistant_global_or_team_id_abm_token_id" UNIQUE ("global_or_team_id","abm_token_id")
);

CREATE TABLE IF NOT EXISTS "mdm_apple_enrollment_profiles" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "token" varchar(36) DEFAULT NULL,
  "type" varchar(10) NOT NULL DEFAULT 'automatic',
  "dep_profile" jsonb DEFAULT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_type" UNIQUE ("type"),
  CONSTRAINT "idx_token" UNIQUE ("token")
);

CREATE TABLE IF NOT EXISTS "mdm_apple_installers" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "name" varchar(255) NOT NULL DEFAULT '',
  "size" bigint NOT NULL,
  "manifest" text NOT NULL,
  "installer" bytea,
  "url_token" varchar(36) DEFAULT NULL,
  PRIMARY KEY ("id")
);

CREATE TABLE IF NOT EXISTS "mdm_apple_setup_assistant_profiles" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "setup_assistant_id" int  NOT NULL,
  "abm_token_id" int  NOT NULL,
  "profile_uuid" varchar(255) NOT NULL DEFAULT '',
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_mdm_apple_setup_assistant_profiles_asst_id_tok_id" UNIQUE ("setup_assistant_id","abm_token_id")
);

CREATE TABLE IF NOT EXISTS "mdm_apple_setup_assistants" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "team_id" int  DEFAULT NULL,
  "global_or_team_id" int  NOT NULL DEFAULT '0',
  "name" text NOT NULL,
  "profile" jsonb NOT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_mdm_setup_assistant_global_or_team_id" UNIQUE ("global_or_team_id")
);

CREATE TABLE IF NOT EXISTS "mdm_config_assets" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "name" varchar(256) NOT NULL DEFAULT '',
  "value" bytea NOT NULL,
  "deleted_at" timestamp NULL DEFAULT NULL,
  "deletion_uuid" varchar(127) NOT NULL DEFAULT '',
  "md5_checksum" bytea NOT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_mdm_config_assets_name_deletion_uuid" UNIQUE ("name","deletion_uuid")
);

CREATE TABLE IF NOT EXISTS "mdm_configuration_profile_labels" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "apple_profile_uuid" varchar(37) DEFAULT NULL,
  "windows_profile_uuid" varchar(37) DEFAULT NULL,
  "label_name" varchar(255) NOT NULL,
  "label_id" int  DEFAULT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  "exclude" boolean NOT NULL DEFAULT FALSE,
  "require_all" boolean NOT NULL DEFAULT FALSE,
  "android_profile_uuid" varchar(37) DEFAULT NULL,
  CONSTRAINT "ck_mdm_configuration_profile_labels_profile_uuid" CHECK ((((if(("apple_profile_uuid" is null),0,1) + if(("windows_profile_uuid" is null),0,1)) + if(("android_profile_uuid" is null),0,1)) = 1)),
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_mdm_configuration_profile_labels_apple_label_name" UNIQUE ("apple_profile_uuid","label_name"),
  CONSTRAINT "idx_mdm_configuration_profile_labels_windows_label_name" UNIQUE ("windows_profile_uuid","label_name"),
  CONSTRAINT "idx_mdm_configuration_profile_labels_android_label_name" UNIQUE ("android_profile_uuid","label_name")
);

CREATE TABLE IF NOT EXISTS "mdm_configuration_profile_variables" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "apple_profile_uuid" varchar(37) DEFAULT NULL,
  "windows_profile_uuid" varchar(37) DEFAULT NULL,
  "fleet_variable_id" int  NOT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT "ck_mdm_configuration_profile_variables_apple_or_windows" CHECK ((("apple_profile_uuid" is null) <> ("windows_profile_uuid" is null))),
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_mdm_configuration_profile_variables_apple_variable" UNIQUE ("apple_profile_uuid","fleet_variable_id"),
  CONSTRAINT "idx_mdm_configuration_profile_variables_windows_label_name" UNIQUE ("windows_profile_uuid","fleet_variable_id")
);

CREATE TABLE IF NOT EXISTS "mdm_declaration_labels" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "apple_declaration_uuid" varchar(37) NOT NULL DEFAULT '',
  "label_name" varchar(255) NOT NULL,
  "label_id" int  DEFAULT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "uploaded_at" timestamp NULL DEFAULT NULL,
  "exclude" boolean NOT NULL DEFAULT FALSE,
  "require_all" boolean NOT NULL DEFAULT FALSE,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_mdm_declaration_labels_label_name" UNIQUE ("apple_declaration_uuid","label_name")
);

CREATE TABLE IF NOT EXISTS "mdm_delivery_status" (
  "status" varchar(20) NOT NULL,
  PRIMARY KEY ("status")
);

CREATE TABLE IF NOT EXISTS "mdm_idp_accounts" (
  "uuid" varchar(255) NOT NULL,
  "username" varchar(255) NOT NULL,
  "fullname" varchar(256) NOT NULL DEFAULT '',
  "email" varchar(255) NOT NULL DEFAULT '',
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("uuid"),
  CONSTRAINT "unique_idp_email" UNIQUE ("email")
);

CREATE TABLE IF NOT EXISTS "mdm_operation_types" (
  "operation_type" varchar(20) NOT NULL,
  PRIMARY KEY ("operation_type")
);

CREATE TABLE IF NOT EXISTS "mdm_windows_configuration_profiles" (
  "team_id" int  NOT NULL DEFAULT '0',
  "name" varchar(255) NOT NULL,
  "syncml" bytea NOT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "uploaded_at" timestamp NULL DEFAULT NULL,
  "profile_uuid" varchar(37) NOT NULL DEFAULT '',
  "GENERATED ALWAYS AS IDENTITY" bigint GENERATED ALWAYS AS IDENTITY,
  "checksum" bytea,
  "secrets_updated_at" timestamp DEFAULT NULL,
  PRIMARY KEY ("profile_uuid"),
  CONSTRAINT "idx_mdm_windows_configuration_profiles_team_id_name" UNIQUE ("team_id","name"),
  CONSTRAINT UNIQUE ()
);

CREATE TABLE IF NOT EXISTS "mdm_windows_enrollments" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "mdm_device_id" varchar(255) NOT NULL,
  "mdm_hardware_id" varchar(255) NOT NULL,
  "device_state" varchar(255) NOT NULL,
  "device_type" varchar(255) NOT NULL,
  "device_name" varchar(255) NOT NULL,
  "enroll_type" varchar(255) NOT NULL,
  "enroll_user_id" varchar(255) NOT NULL,
  "enroll_proto_version" varchar(255) NOT NULL,
  "enroll_client_version" varchar(255) NOT NULL,
  "not_in_oobe" boolean NOT NULL DEFAULT FALSE,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  "host_uuid" varchar(255) NOT NULL DEFAULT '',
  "credentials_hash" bytea DEFAULT NULL,
  "credentials_acknowledged" boolean NOT NULL DEFAULT FALSE,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_type" UNIQUE ("mdm_hardware_id")
);

CREATE TABLE IF NOT EXISTS "microsoft_compliance_partner_host_statuses" (
  "host_id" int  NOT NULL,
  "device_id" varchar(64) NOT NULL,
  "user_principal_name" varchar(255) NOT NULL,
  "managed" boolean DEFAULT NULL,
  "compliant" boolean DEFAULT NULL,
  "created_at" timestamp DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("host_id")
);

CREATE TABLE IF NOT EXISTS "microsoft_compliance_partner_integrations" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "tenant_id" varchar(64) NOT NULL,
  "proxy_server_secret" varchar(64) NOT NULL,
  "setup_done" boolean NOT NULL DEFAULT FALSE,
  "created_at" timestamp DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_microsoft_compliance_partner_tenant_id" UNIQUE ("tenant_id")
);

CREATE TABLE IF NOT EXISTS "migration_status_tables" (
  "id" bigint GENERATED ALWAYS AS IDENTITY,
  "version_id" bigint NOT NULL,
  "is_applied" boolean NOT NULL,
  "tstamp" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY ("id")
);

CREATE TABLE IF NOT EXISTS "mobile_device_management_solutions" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "name" varchar(100) NOT NULL,
  "server_url" varchar(255) NOT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_mobile_device_management_solutions_name" UNIQUE ("name","server_url")
);

CREATE TABLE IF NOT EXISTS "munki_issues" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "name" varchar(255) NOT NULL,
  "issue_type" varchar(10) NOT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_munki_issues_name" UNIQUE ("name","issue_type")
);

CREATE TABLE IF NOT EXISTS "nano_cert_auth_associations" (
  "id" varchar(255) NOT NULL,
  "sha256" char(64) NOT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  "cert_not_valid_after" timestamp NULL DEFAULT NULL,
  "renew_command_uuid" varchar(127) DEFAULT NULL,
  CONSTRAINT "nano_cert_auth_associations_chk_1" CHECK (("id" <> '')),
  CONSTRAINT "nano_cert_auth_associations_chk_2" CHECK (("sha256" <> '')),
  PRIMARY KEY ("id","sha256")
);

CREATE TABLE IF NOT EXISTS "nano_command_results" (
  "id" varchar(255) NOT NULL,
  "command_uuid" varchar(127) NOT NULL,
  "status" varchar(31) NOT NULL,
  "result" text NOT NULL,
  "not_now_at" timestamp NULL DEFAULT NULL,
  "not_now_tally" int NOT NULL DEFAULT '0',
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  CONSTRAINT "nano_command_results_chk_1" CHECK (("status" <> '')),
  CONSTRAINT "nano_command_results_chk_2" CHECK ((substr("result",1,5) = '<?xml')),
  PRIMARY KEY ("id","command_uuid")
);

CREATE TABLE IF NOT EXISTS "nano_commands" (
  "command_uuid" varchar(127) NOT NULL,
  "request_type" varchar(63) NOT NULL,
  "command" text NOT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  "subtype" text NOT NULL DEFAULT 'None',
  CONSTRAINT "nano_commands_chk_1" CHECK (("command_uuid" <> '')),
  CONSTRAINT "nano_commands_chk_2" CHECK (("request_type" <> '')),
  PRIMARY KEY ("command_uuid")
);

CREATE TABLE IF NOT EXISTS "nano_dep_names" (
  "name" varchar(255) NOT NULL,
  "consumer_key" text,
  "consumer_secret" text,
  "access_token" text,
  "access_secret" text,
  "access_token_expiry" timestamp NULL DEFAULT NULL,
  "config_base_url" varchar(255) DEFAULT NULL,
  "tokenpki_cert_pem" text,
  "tokenpki_key_pem" text,
  "syncer_cursor" varchar(1024) DEFAULT NULL,
  "syncer_cursor_at" timestamp NULL DEFAULT NULL,
  "assigner_profile_uuid" text,
  "assigner_profile_uuid_at" timestamp NULL DEFAULT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  CONSTRAINT "nano_dep_names_chk_1" CHECK ((("tokenpki_cert_pem" is null) or (substr("tokenpki_cert_pem",1,27) = '-----BEGIN CERTIFICATE-----'))),
  CONSTRAINT "nano_dep_names_chk_2" CHECK ((("tokenpki_key_pem" is null) or (substr("tokenpki_key_pem",1,5) = '-----'))),
  PRIMARY KEY ("name")
);

CREATE TABLE IF NOT EXISTS "nano_devices" (
  "id" varchar(255) NOT NULL,
  "identity_cert" text,
  "serial_number" varchar(127) DEFAULT NULL,
  "unlock_token" bytea,
  "unlock_token_at" timestamp NULL DEFAULT NULL,
  "authenticate" text NOT NULL,
  "authenticate_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "token_update" text,
  "token_update_at" timestamp NULL DEFAULT NULL,
  "bootstrap_token_b64" text,
  "bootstrap_token_at" timestamp NULL DEFAULT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  "platform" varchar(255) NOT NULL DEFAULT '',
  "enroll_team_id" int  DEFAULT NULL,
  CONSTRAINT "nano_devices_chk_1" CHECK ((("identity_cert" is null) or (substr("identity_cert",1,27) = '-----BEGIN CERTIFICATE-----'))),
  CONSTRAINT "nano_devices_chk_2" CHECK ((("serial_number" is null) or ("serial_number" <> ''))),
  CONSTRAINT "nano_devices_chk_3" CHECK ((("unlock_token" is null) or (length("unlock_token") > 0))),
  CONSTRAINT "nano_devices_chk_4" CHECK (("authenticate" <> '')),
  CONSTRAINT "nano_devices_chk_5" CHECK ((("token_update" is null) or ("token_update" <> ''))),
  CONSTRAINT "nano_devices_chk_6" CHECK ((("bootstrap_token_b64" is null) or ("bootstrap_token_b64" <> ''))),
  PRIMARY KEY ("id")
);

CREATE TABLE IF NOT EXISTS "nano_enrollment_queue" (
  "id" varchar(255) NOT NULL,
  "command_uuid" varchar(127) NOT NULL,
  "active" boolean NOT NULL DEFAULT TRUE,
  "priority" smallint NOT NULL DEFAULT '0',
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id","command_uuid")
);

CREATE TABLE IF NOT EXISTS "nano_enrollments" (
  "id" varchar(255) NOT NULL,
  "device_id" varchar(255) NOT NULL,
  "user_id" varchar(255) DEFAULT NULL,
  "type" varchar(31) NOT NULL,
  "topic" varchar(255) NOT NULL,
  "push_magic" varchar(127) NOT NULL,
  "token_hex" varchar(255) NOT NULL,
  "enabled" boolean NOT NULL DEFAULT TRUE,
  "token_update_tally" int NOT NULL DEFAULT '1',
  "last_seen_at" timestamp NOT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  "enrolled_from_migration" smallint  NOT NULL DEFAULT '0',
  CONSTRAINT "nano_enrollments_chk_1" CHECK (("id" <> '')),
  CONSTRAINT "nano_enrollments_chk_2" CHECK (("type" <> '')),
  CONSTRAINT "nano_enrollments_chk_3" CHECK (("topic" <> '')),
  CONSTRAINT "nano_enrollments_chk_4" CHECK (("push_magic" <> '')),
  CONSTRAINT "nano_enrollments_chk_5" CHECK (("token_hex" <> '')),
  PRIMARY KEY ("id"),
  CONSTRAINT "user_id" UNIQUE ("user_id")
);

CREATE TABLE IF NOT EXISTS "nano_push_certs" (
  "topic" varchar(255) NOT NULL,
  "cert_pem" text NOT NULL,
  "key_pem" text NOT NULL,
  "stale_token" int NOT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  CONSTRAINT "nano_push_certs_chk_1" CHECK (("topic" <> '')),
  CONSTRAINT "nano_push_certs_chk_2" CHECK ((substr("cert_pem",1,27) = '-----BEGIN CERTIFICATE-----')),
  CONSTRAINT "nano_push_certs_chk_3" CHECK ((substr("key_pem",1,5) = '-----')),
  PRIMARY KEY ("topic")
);

CREATE TABLE IF NOT EXISTS "nano_users" (
  "id" varchar(255) NOT NULL,
  "device_id" varchar(255) NOT NULL,
  "user_short_name" varchar(255) DEFAULT NULL,
  "user_long_name" varchar(255) DEFAULT NULL,
  "token_update" text,
  "token_update_at" timestamp NULL DEFAULT NULL,
  "user_authenticate" text,
  "user_authenticate_at" timestamp NULL DEFAULT NULL,
  "user_authenticate_digest" text,
  "user_authenticate_digest_at" timestamp NULL DEFAULT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  CONSTRAINT "nano_users_chk_1" CHECK ((("user_short_name" is null) or ("user_short_name" <> ''))),
  CONSTRAINT "nano_users_chk_2" CHECK ((("user_long_name" is null) or ("user_long_name" <> ''))),
  CONSTRAINT "nano_users_chk_3" CHECK ((("token_update" is null) or ("token_update" <> ''))),
  CONSTRAINT "nano_users_chk_4" CHECK ((("user_authenticate" is null) or ("user_authenticate" <> ''))),
  CONSTRAINT "nano_users_chk_5" CHECK ((("user_authenticate_digest" is null) or ("user_authenticate_digest" <> ''))),
  PRIMARY KEY ("id","device_id"),
  CONSTRAINT "idx_unique_id" UNIQUE ("id")
);

CREATE TABLE IF NOT EXISTS "network_interfaces" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "host_id" int  NOT NULL,
  "mac" varchar(255) NOT NULL DEFAULT '',
  "ip_address" varchar(255) NOT NULL DEFAULT '',
  "broadcast" varchar(255) NOT NULL DEFAULT '',
  "ibytes" bigint NOT NULL DEFAULT '0',
  "interface" varchar(255) NOT NULL DEFAULT '',
  "ipackets" bigint NOT NULL DEFAULT '0',
  "last_change" bigint NOT NULL DEFAULT '0',
  "mask" varchar(255) NOT NULL DEFAULT '',
  "metric" int NOT NULL DEFAULT '0',
  "mtu" int NOT NULL DEFAULT '0',
  "obytes" bigint NOT NULL DEFAULT '0',
  "ierrors" bigint NOT NULL DEFAULT '0',
  "oerrors" bigint NOT NULL DEFAULT '0',
  "opackets" bigint NOT NULL DEFAULT '0',
  "point_to_point" varchar(255) NOT NULL DEFAULT '',
  "type" int NOT NULL DEFAULT '0',
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_network_interfaces_unique_ip_host_intf" UNIQUE ("ip_address","host_id","interface")
);

CREATE TABLE IF NOT EXISTS "operating_system_version_vulnerabilities" (
  "id" bigint GENERATED ALWAYS AS IDENTITY,
  "os_version_id" int  NOT NULL,
  "cve" varchar(255) NOT NULL,
  "team_id" int  DEFAULT NULL,
  "source" smallint DEFAULT '0',
  "resolved_in_version" varchar(255) DEFAULT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_os_version_vulnerabilities_unq_os_version_team_cve" UNIQUE ((ifnull(cast("team_id" as signed),-(1))),"os_version_id","cve")
);

CREATE TABLE IF NOT EXISTS "operating_system_vulnerabilities" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "operating_system_id" int  NOT NULL,
  "cve" varchar(255) NOT NULL,
  "source" smallint DEFAULT '0',
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "resolved_in_version" varchar(255) DEFAULT NULL,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_os_vulnerabilities_unq_os_id_cve" UNIQUE ("operating_system_id","cve")
);

CREATE TABLE IF NOT EXISTS "operating_systems" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "name" varchar(255) NOT NULL,
  "version" varchar(150) NOT NULL,
  "arch" varchar(150) NOT NULL,
  "kernel_version" varchar(150) NOT NULL,
  "platform" varchar(50) NOT NULL,
  "display_version" varchar(10) NOT NULL DEFAULT '',
  "os_version_id" int  DEFAULT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_unique_os" UNIQUE ("name","version","arch","kernel_version","platform","display_version")
);

CREATE TABLE IF NOT EXISTS "osquery_options" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "override_type" int NOT NULL,
  "override_identifier" varchar(255) NOT NULL DEFAULT '',
  "options" jsonb NOT NULL,
  PRIMARY KEY ("id")
);

CREATE TABLE IF NOT EXISTS "pack_targets" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "pack_id" int  DEFAULT NULL,
  "type" int DEFAULT NULL,
  "target_id" int  NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "constraint_pack_target_unique" UNIQUE ("pack_id","target_id","type")
);

CREATE TABLE IF NOT EXISTS "packs" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  "disabled" boolean NOT NULL DEFAULT FALSE,
  "name" varchar(255) NOT NULL,
  "description" varchar(255) DEFAULT NULL,
  "platform" varchar(255) DEFAULT NULL,
  "pack_type" varchar(255) DEFAULT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_pack_unique_name" UNIQUE ("name")
);

CREATE TABLE IF NOT EXISTS "password_reset_requests" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "expires_at" timestamp NOT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  "user_id" int  NOT NULL,
  "token" varchar(1024) NOT NULL,
  PRIMARY KEY ("id")
);

CREATE TABLE IF NOT EXISTS "policies" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  "team_id" int  DEFAULT NULL,
  "resolution" text,
  "name" varchar(255) NOT NULL,
  "query" text NOT NULL,
  "description" text NOT NULL,
  "author_id" int  DEFAULT NULL,
  "platforms" varchar(255) NOT NULL DEFAULT '',
  "critical" boolean NOT NULL DEFAULT FALSE,
  "checksum" bytea NOT NULL,
  "calendar_events_enabled" smallint  NOT NULL DEFAULT '0',
  "software_installer_id" int  DEFAULT NULL,
  "script_id" int  DEFAULT NULL,
  "vpp_apps_teams_id" int  DEFAULT NULL,
  "conditional_access_enabled" smallint  NOT NULL DEFAULT '0',
  "conditional_access_bypass_enabled" boolean NOT NULL DEFAULT TRUE,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_policies_checksum" UNIQUE ("checksum")
);

CREATE TABLE IF NOT EXISTS "policy_automation_iterations" (
  "policy_id" int  NOT NULL,
  "iteration" int NOT NULL,
  PRIMARY KEY ("policy_id")
);

CREATE TABLE IF NOT EXISTS "policy_labels" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "policy_id" int  NOT NULL,
  "label_id" int  NOT NULL,
  "exclude" boolean NOT NULL DEFAULT FALSE,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_policy_labels_policy_label" UNIQUE ("policy_id","label_id")
);

CREATE TABLE IF NOT EXISTS "policy_membership" (
  "policy_id" int  NOT NULL,
  "host_id" int  NOT NULL,
  "passes" boolean DEFAULT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  "automation_iteration" int DEFAULT NULL,
  PRIMARY KEY ("policy_id","host_id")
);

CREATE TABLE IF NOT EXISTS "policy_stats" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "policy_id" int  NOT NULL,
  "inherited_team_id" int  DEFAULT NULL,
  "passing_host_count" integer  NOT NULL DEFAULT '0',
  "failing_host_count" integer  NOT NULL DEFAULT '0',
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  "inherited_team_id_char" text,
  PRIMARY KEY ("id"),
  CONSTRAINT "policy_id" UNIQUE ("policy_id","inherited_team_id_char")
);

CREATE TABLE IF NOT EXISTS "queries" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  "saved" boolean NOT NULL DEFAULT FALSE,
  "name" varchar(255) NOT NULL,
  "description" text NOT NULL,
  "query" text NOT NULL,
  "author_id" int  DEFAULT NULL,
  "observer_can_run" boolean NOT NULL DEFAULT FALSE,
  "team_id" int  DEFAULT NULL,
  "team_id_char" char(10) NOT NULL DEFAULT '',
  "platform" varchar(255) NOT NULL DEFAULT '',
  "min_osquery_version" varchar(255) NOT NULL DEFAULT '',
  "schedule_interval" int  NOT NULL DEFAULT '0',
  "automations_enabled" smallint  NOT NULL DEFAULT '0',
  "logging_type" varchar(255) NOT NULL DEFAULT 'snapshot',
  "discard_data" boolean NOT NULL DEFAULT TRUE,
  "is_scheduled" text,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_team_id_name_unq" UNIQUE ("team_id_char","name"),
  CONSTRAINT "idx_name_team_id_unq" UNIQUE ("name","team_id_char")
);

CREATE TABLE IF NOT EXISTS "query_labels" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "query_id" int  NOT NULL,
  "label_id" int  NOT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_query_labels_query_label" UNIQUE ("query_id","label_id")
);

CREATE TABLE IF NOT EXISTS "query_results" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "query_id" int  NOT NULL,
  "host_id" int  NOT NULL,
  "osquery_version" varchar(50) DEFAULT NULL,
  "error" text,
  "last_fetched" timestamp NOT NULL,
  "data" jsonb DEFAULT NULL,
  PRIMARY KEY ("id")
);

CREATE TABLE IF NOT EXISTS "scep_certificates" (
  "serial" bigint NOT NULL,
  "name" varchar(1024) DEFAULT NULL,
  "not_valid_before" timestamp NOT NULL,
  "not_valid_after" timestamp NOT NULL,
  "certificate_pem" text NOT NULL,
  "revoked" boolean NOT NULL DEFAULT FALSE,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  CONSTRAINT "scep_certificates_chk_1" CHECK ((substr("certificate_pem",1,27) = '-----BEGIN CERTIFICATE-----')),
  CONSTRAINT "scep_certificates_chk_2" CHECK ((("name" is null) or ("name" <> ''))),
  PRIMARY KEY ("serial")
);

CREATE TABLE IF NOT EXISTS "scep_serials" (
  "serial" bigint GENERATED ALWAYS AS IDENTITY,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY ("serial")
);

CREATE TABLE IF NOT EXISTS "scheduled_queries" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  "pack_id" int  DEFAULT NULL,
  "query_id" int  DEFAULT NULL,
  "interval" int  DEFAULT NULL,
  "snapshot" boolean DEFAULT NULL,
  "removed" boolean DEFAULT NULL,
  "platform" varchar(255) DEFAULT '',
  "version" varchar(255) DEFAULT '',
  "shard" int  DEFAULT NULL,
  "query_name" varchar(255) NOT NULL,
  "name" varchar(255) NOT NULL,
  "description" varchar(1023) DEFAULT '',
  "denylist" boolean DEFAULT NULL,
  "team_id_char" char(10) NOT NULL DEFAULT '',
  PRIMARY KEY ("id"),
  CONSTRAINT "unique_names_in_packs" UNIQUE ("name","pack_id")
);

CREATE TABLE IF NOT EXISTS "scheduled_query_stats" (
  "host_id" int  NOT NULL,
  "scheduled_query_id" int  NOT NULL,
  "average_memory" bigint  NOT NULL,
  "denylisted" boolean DEFAULT NULL,
  "executions" bigint  NOT NULL,
  "schedule_interval" int DEFAULT NULL,
  "last_executed" timestamp NULL DEFAULT NULL,
  "output_size" bigint  NOT NULL,
  "system_time" bigint  NOT NULL,
  "user_time" bigint  NOT NULL,
  "wall_time" bigint  NOT NULL,
  "query_type" smallint NOT NULL DEFAULT '0',
  PRIMARY KEY ("host_id","scheduled_query_id","query_type")
);

CREATE TABLE IF NOT EXISTS "scim_groups" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "external_id" varchar(255) DEFAULT NULL,
  "display_name" varchar(255) NOT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_scim_groups_display_name" UNIQUE ("display_name")
);

CREATE TABLE IF NOT EXISTS "scim_last_request" (
  "id" smallint  NOT NULL DEFAULT '1',
  "status" varchar(31) NOT NULL,
  "details" varchar(255) NOT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id")
);

CREATE TABLE IF NOT EXISTS "scim_user_emails" (
  "id" bigint GENERATED ALWAYS AS IDENTITY,
  "scim_user_id" int  NOT NULL,
  "email" varchar(255) NOT NULL,
  "primary" boolean DEFAULT NULL,
  "type" varchar(31) DEFAULT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id")
);

CREATE TABLE IF NOT EXISTS "scim_user_group" (
  "scim_user_id" int  NOT NULL,
  "group_id" int  NOT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY ("scim_user_id","group_id")
);

CREATE TABLE IF NOT EXISTS "scim_users" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "external_id" varchar(255) DEFAULT NULL,
  "user_name" varchar(255) NOT NULL,
  "given_name" varchar(255) DEFAULT NULL,
  "family_name" varchar(255) DEFAULT NULL,
  "active" boolean DEFAULT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  "department" varchar(255) DEFAULT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_scim_users_user_name" UNIQUE ("user_name")
);

CREATE TABLE IF NOT EXISTS "script_contents" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "md5_checksum" bytea NOT NULL,
  "contents" text NOT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_script_contents_md5_checksum" UNIQUE ("md5_checksum")
);

CREATE TABLE IF NOT EXISTS "script_upcoming_activities" (
  "upcoming_activity_id" bigint  NOT NULL,
  "script_id" int  DEFAULT NULL,
  "script_content_id" int  DEFAULT NULL,
  "policy_id" int  DEFAULT NULL,
  "setup_experience_script_id" int  DEFAULT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("upcoming_activity_id")
);

CREATE TABLE IF NOT EXISTS "scripts" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "team_id" int  DEFAULT NULL,
  "global_or_team_id" int  NOT NULL DEFAULT '0',
  "name" varchar(255) NOT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  "script_content_id" int  DEFAULT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_scripts_global_or_team_id_name" UNIQUE ("global_or_team_id","name"),
  CONSTRAINT "idx_scripts_team_name" UNIQUE ("team_id","name")
);

CREATE TABLE IF NOT EXISTS "secret_variables" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "name" varchar(255) NOT NULL,
  "value" bytea NOT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_secret_variables_name" UNIQUE ("name")
);

CREATE TABLE IF NOT EXISTS "sessions" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "accessed_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  "user_id" int  NOT NULL,
  "key" varchar(255) NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_session_unique_key" UNIQUE ("key")
);

CREATE TABLE IF NOT EXISTS "setup_experience_scripts" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "team_id" int  DEFAULT NULL,
  "global_or_team_id" int  NOT NULL DEFAULT '0',
  "name" varchar(255) NOT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  "script_content_id" int  DEFAULT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_setup_experience_scripts_global_or_team_id" UNIQUE ("global_or_team_id")
);

CREATE TABLE IF NOT EXISTS "setup_experience_status_results" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "host_uuid" varchar(255) NOT NULL,
  "name" varchar(255) NOT NULL,
  "status" text NOT NULL,
  "software_installer_id" int  DEFAULT NULL,
  "host_software_installs_execution_id" varchar(255) DEFAULT NULL,
  "vpp_app_team_id" int  DEFAULT NULL,
  "nano_command_uuid" varchar(255) DEFAULT NULL,
  "setup_experience_script_id" int  DEFAULT NULL,
  "script_execution_id" varchar(255) DEFAULT NULL,
  "error" varchar(255) DEFAULT NULL,
  PRIMARY KEY ("id")
);

CREATE TABLE IF NOT EXISTS "software" (
  "id" bigint GENERATED ALWAYS AS IDENTITY,
  "name" varchar(255) NOT NULL,
  "version" varchar(255) NOT NULL DEFAULT '',
  "source" varchar(64) NOT NULL,
  "bundle_identifier" varchar(255) DEFAULT '',
  "release" varchar(64) NOT NULL DEFAULT '',
  "vendor_old" varchar(32) NOT NULL DEFAULT '',
  "arch" varchar(16) NOT NULL DEFAULT '',
  "vendor" varchar(114) NOT NULL DEFAULT '',
  "extension_for" varchar(255) NOT NULL DEFAULT '',
  "extension_id" varchar(255) NOT NULL DEFAULT '',
  "title_id" int  DEFAULT NULL,
  "checksum" bytea NOT NULL,
  "name_source" text NOT NULL DEFAULT 'basic',
  "application_id" varchar(255) DEFAULT NULL,
  "upgrade_code" char(38) DEFAULT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_software_checksum" UNIQUE ("checksum")
);

CREATE TABLE IF NOT EXISTS "software_categories" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "name" varchar(63) NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_software_categories_name" UNIQUE ("name")
);

CREATE TABLE IF NOT EXISTS "software_cpe" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "software_id" bigint  DEFAULT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  "cpe" varchar(255) NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "unq_software_id" UNIQUE ("software_id")
);

CREATE TABLE IF NOT EXISTS "software_cve" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "cve" varchar(255) NOT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  "source" int DEFAULT '0',
  "software_id" bigint  DEFAULT NULL,
  "resolved_in_version" varchar(255) DEFAULT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "unq_software_id_cve" UNIQUE ("software_id","cve")
);

CREATE TABLE IF NOT EXISTS "software_host_counts" (
  "software_id" bigint  NOT NULL,
  "hosts_count" int  NOT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  "team_id" int  NOT NULL DEFAULT '0',
  "global_stats" smallint  NOT NULL DEFAULT '0',
  PRIMARY KEY ("software_id","team_id","global_stats")
);

CREATE TABLE IF NOT EXISTS "software_install_upcoming_activities" (
  "upcoming_activity_id" bigint  NOT NULL,
  "software_installer_id" int  DEFAULT NULL,
  "policy_id" int  DEFAULT NULL,
  "software_title_id" int  DEFAULT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("upcoming_activity_id")
);

CREATE TABLE IF NOT EXISTS "software_installer_labels" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "software_installer_id" int  NOT NULL,
  "label_id" int  NOT NULL,
  "exclude" boolean NOT NULL DEFAULT FALSE,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_software_installer_labels_software_installer_id_label_id" UNIQUE ("software_installer_id","label_id")
);

CREATE TABLE IF NOT EXISTS "software_installer_software_categories" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "software_category_id" int  NOT NULL,
  "software_installer_id" int  NOT NULL,
  "created_at" timestamp DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_unique_software_installer_id_software_category_id" UNIQUE ("software_installer_id","software_category_id")
);

CREATE TABLE IF NOT EXISTS "software_installers" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "team_id" int  DEFAULT NULL,
  "global_or_team_id" int  NOT NULL DEFAULT '0',
  "title_id" int  DEFAULT NULL,
  "filename" varchar(255) NOT NULL,
  "version" varchar(255) NOT NULL,
  "platform" varchar(255) NOT NULL,
  "pre_install_query" text,
  "install_script_content_id" int  NOT NULL,
  "post_install_script_content_id" int  DEFAULT NULL,
  "storage_id" varchar(64) NOT NULL,
  "uploaded_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "self_service" boolean NOT NULL DEFAULT FALSE,
  "user_id" int  DEFAULT NULL,
  "user_name" varchar(255) NOT NULL DEFAULT '',
  "user_email" varchar(255) NOT NULL DEFAULT '',
  "url" varchar(4095) NOT NULL DEFAULT '',
  "package_ids" text NOT NULL,
  "extension" varchar(32) NOT NULL DEFAULT '',
  "uninstall_script_content_id" int  NOT NULL,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  "fleet_maintained_app_id" int  DEFAULT NULL,
  "install_during_setup" boolean NOT NULL DEFAULT FALSE,
  "upgrade_code" varchar(48) NOT NULL DEFAULT '',
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_software_installers_team_id_title_id" UNIQUE ("global_or_team_id","title_id")
);

CREATE TABLE IF NOT EXISTS "software_title_display_names" (
  "id" int NOT NULL GENERATED ALWAYS AS IDENTITY,
  "team_id" int  NOT NULL,
  "software_title_id" int  NOT NULL,
  "display_name" varchar(255) NOT NULL DEFAULT '',
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_unique_team_id_title_id" UNIQUE ("team_id","software_title_id")
);

CREATE TABLE IF NOT EXISTS "software_title_icons" (
  "id" int NOT NULL GENERATED ALWAYS AS IDENTITY,
  "team_id" int  NOT NULL,
  "software_title_id" int  NOT NULL,
  "storage_id" varchar(64) NOT NULL,
  "filename" varchar(255) NOT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_unique_team_id_title_id_storage_id" UNIQUE ("team_id","software_title_id")
);

CREATE TABLE IF NOT EXISTS "software_titles" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "name" varchar(255) NOT NULL,
  "source" varchar(64) NOT NULL,
  "extension_for" varchar(255) NOT NULL DEFAULT '',
  "bundle_identifier" varchar(255) DEFAULT NULL,
  "additional_identifier" text,
  "is_kernel" boolean NOT NULL DEFAULT FALSE,
  "application_id" varchar(255) DEFAULT NULL,
  "unique_identifier" text,
  "upgrade_code" char(38) DEFAULT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_software_titles_bundle_identifier" UNIQUE ("bundle_identifier","additional_identifier"),
  CONSTRAINT "idx_unique_sw_titles" UNIQUE ("unique_identifier","source","extension_for")
);

CREATE TABLE IF NOT EXISTS "software_titles_host_counts" (
  "software_title_id" int  NOT NULL,
  "hosts_count" int  NOT NULL,
  "team_id" int  NOT NULL DEFAULT '0',
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  "global_stats" smallint  NOT NULL DEFAULT '0',
  PRIMARY KEY ("software_title_id","team_id","global_stats")
);

CREATE TABLE IF NOT EXISTS "software_update_schedules" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "team_id" int  NOT NULL,
  "title_id" int  NOT NULL,
  "enabled" boolean NOT NULL DEFAULT FALSE,
  "start_time" char(5) NOT NULL,
  "end_time" char(5) NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_team_title" UNIQUE ("team_id","title_id")
);

CREATE TABLE IF NOT EXISTS "statistics" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  "anonymous_identifier" varchar(255) NOT NULL,
  PRIMARY KEY ("id")
);

CREATE TABLE IF NOT EXISTS "teams" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "name" varchar(255) NOT NULL,
  "description" varchar(1023) NOT NULL DEFAULT '',
  "config" jsonb DEFAULT NULL,
  "name_bin" text,
  "filename" varchar(255) DEFAULT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_teams_filename" UNIQUE ("filename"),
  CONSTRAINT "idx_name_bin" UNIQUE ("name_bin")
);

CREATE TABLE IF NOT EXISTS "upcoming_activities" (
  "id" bigint GENERATED ALWAYS AS IDENTITY,
  "host_id" int  NOT NULL,
  "priority" int NOT NULL DEFAULT '0',
  "user_id" int  DEFAULT NULL,
  "fleet_initiated" boolean NOT NULL DEFAULT FALSE,
  "activity_type" text NOT NULL,
  "execution_id" varchar(255) NOT NULL,
  "payload" jsonb NOT NULL,
  "activated_at" timestamp DEFAULT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_upcoming_activities_execution_id" UNIQUE ("execution_id")
);

CREATE TABLE IF NOT EXISTS "user_teams" (
  "user_id" int  NOT NULL,
  "team_id" int  NOT NULL,
  "role" varchar(64) NOT NULL,
  PRIMARY KEY ("user_id","team_id")
);

CREATE TABLE IF NOT EXISTS "users" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  "password" bytea NOT NULL,
  "salt" varchar(255) NOT NULL,
  "name" varchar(255) NOT NULL DEFAULT '',
  "email" varchar(255) NOT NULL,
  "admin_forced_password_reset" boolean NOT NULL DEFAULT FALSE,
  "gravatar_url" varchar(255) NOT NULL DEFAULT '',
  "position" varchar(255) NOT NULL DEFAULT '',
  "sso_enabled" smallint NOT NULL DEFAULT '0',
  "global_role" varchar(64) DEFAULT NULL,
  "api_only" boolean NOT NULL DEFAULT FALSE,
  "mfa_enabled" boolean NOT NULL DEFAULT FALSE,
  "settings" jsonb NOT NULL DEFAULT '{}'::jsonb,
  "invite_id" int  DEFAULT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_user_unique_email" UNIQUE ("email"),
  CONSTRAINT "invite_id" UNIQUE ("invite_id")
);

CREATE TABLE IF NOT EXISTS "users_deleted" (
  "id" int  NOT NULL,
  "name" varchar(255) NOT NULL DEFAULT '',
  "email" varchar(255) NOT NULL,
  "created_at" timestamp DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id")
);

CREATE TABLE IF NOT EXISTS "verification_tokens" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "user_id" int  NOT NULL,
  "token" varchar(255) NOT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY ("id"),
  CONSTRAINT "token" UNIQUE ("token")
);

CREATE TABLE IF NOT EXISTS "vpp_app_team_labels" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "vpp_app_team_id" int  NOT NULL,
  "label_id" int  NOT NULL,
  "exclude" boolean NOT NULL DEFAULT FALSE,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_vpp_app_team_labels_vpp_app_team_id_label_id" UNIQUE ("vpp_app_team_id","label_id")
);

CREATE TABLE IF NOT EXISTS "vpp_app_team_software_categories" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "software_category_id" int  NOT NULL,
  "vpp_app_team_id" int  NOT NULL,
  "created_at" timestamp DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_unique_vpp_app_team_id_software_category_id" UNIQUE ("vpp_app_team_id","software_category_id")
);

CREATE TABLE IF NOT EXISTS "vpp_app_upcoming_activities" (
  "upcoming_activity_id" bigint  NOT NULL,
  "adam_id" varchar(255) NOT NULL,
  "platform" varchar(10) NOT NULL,
  "vpp_token_id" int  DEFAULT NULL,
  "policy_id" int  DEFAULT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("upcoming_activity_id")
);

CREATE TABLE IF NOT EXISTS "vpp_apps" (
  "adam_id" varchar(255) NOT NULL,
  "title_id" int  DEFAULT NULL,
  "bundle_identifier" varchar(255) NOT NULL DEFAULT '',
  "icon_url" varchar(255) NOT NULL DEFAULT '',
  "name" varchar(255) NOT NULL DEFAULT '',
  "latest_version" varchar(255) NOT NULL DEFAULT '',
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  "platform" varchar(10) NOT NULL,
  PRIMARY KEY ("adam_id","platform")
);

CREATE TABLE IF NOT EXISTS "vpp_apps_teams" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "adam_id" varchar(255) NOT NULL,
  "team_id" int  DEFAULT NULL,
  "global_or_team_id" int NOT NULL DEFAULT '0',
  "platform" varchar(10) NOT NULL,
  "self_service" boolean NOT NULL DEFAULT FALSE,
  "vpp_token_id" int  DEFAULT NULL,
  "install_during_setup" boolean NOT NULL DEFAULT FALSE,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_global_or_team_id_adam_id" UNIQUE ("global_or_team_id","adam_id","platform")
);

CREATE TABLE IF NOT EXISTS "vpp_token_teams" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "vpp_token_id" int  NOT NULL,
  "team_id" int  DEFAULT NULL,
  "null_team_type" text DEFAULT 'none',
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_vpp_token_teams_team_id" UNIQUE ("team_id")
);

CREATE TABLE IF NOT EXISTS "vpp_tokens" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "organization_name" varchar(255) NOT NULL,
  "location" varchar(255) NOT NULL,
  "renew_at" timestamp NOT NULL,
  "token" bytea NOT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_vpp_tokens_location" UNIQUE ("location")
);

CREATE TABLE IF NOT EXISTS "vulnerability_host_counts" (
  "cve" varchar(20) NOT NULL,
  "team_id" int  NOT NULL DEFAULT '0',
  "host_count" int  NOT NULL DEFAULT '0',
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  "global_stats" boolean NOT NULL DEFAULT FALSE,
  CONSTRAINT "cve_team_id_global_stats" UNIQUE ("cve","team_id","global_stats")
);

CREATE TABLE IF NOT EXISTS "windows_mdm_command_queue" (
  "enrollment_id" int  NOT NULL,
  "command_uuid" varchar(127) NOT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("enrollment_id","command_uuid")
);

CREATE TABLE IF NOT EXISTS "windows_mdm_command_results" (
  "enrollment_id" int  NOT NULL,
  "command_uuid" varchar(127) NOT NULL,
  "raw_result" text NOT NULL,
  "response_id" int  NOT NULL,
  "status_code" varchar(31) NOT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("enrollment_id","command_uuid")
);

CREATE TABLE IF NOT EXISTS "windows_mdm_commands" (
  "command_uuid" varchar(127) NOT NULL,
  "raw_command" text NOT NULL,
  "target_loc_uri" varchar(255) NOT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("command_uuid")
);

CREATE TABLE IF NOT EXISTS "windows_mdm_responses" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "enrollment_id" int  NOT NULL,
  "raw_response" text NOT NULL,
  "created_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id")
);

CREATE TABLE IF NOT EXISTS "windows_updates" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "host_id" int  NOT NULL,
  "date_epoch" int  NOT NULL,
  "kb_id" int  NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_unique_windows_updates" UNIQUE ("host_id","kb_id")
);

CREATE TABLE IF NOT EXISTS "wstep_cert_auth_associations" (
  "id" varchar(255) NOT NULL,
  "sha256" char(64) NOT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("id","sha256")
);

CREATE TABLE IF NOT EXISTS "wstep_certificates" (
  "serial" bigint  NOT NULL,
  "name" varchar(1024) NOT NULL,
  "not_valid_before" timestamp NOT NULL,
  "not_valid_after" timestamp NOT NULL,
  "certificate_pem" text NOT NULL,
  "revoked" boolean NOT NULL DEFAULT FALSE,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NULL DEFAULT CURRENT_TIMESTAMP ,
  PRIMARY KEY ("serial")
);

CREATE TABLE IF NOT EXISTS "wstep_serials" (
  "serial" bigint GENERATED ALWAYS AS IDENTITY,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY ("serial")
);

CREATE TABLE IF NOT EXISTS "yara_rules" (
  "id" int  NOT NULL GENERATED ALWAYS AS IDENTITY,
  "name" varchar(255) NOT NULL,
  "contents" text NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "idx_yara_rules_name" UNIQUE ("name")
);


CREATE TABLE IF NOT EXISTS "migration_status_tables" (
    id serial NOT NULL PRIMARY KEY, version_id bigint NOT NULL,
    is_applied boolean NOT NULL, tstamp timestamp DEFAULT now());
CREATE TABLE IF NOT EXISTS "migration_status_data" (
    id serial NOT NULL PRIMARY KEY, version_id bigint NOT NULL,
    is_applied boolean NOT NULL, tstamp timestamp DEFAULT now());

INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20161118193812, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20161118211713, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20161118212436, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20161118212515, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20161118212528, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20161118212538, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20161118212549, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20161118212557, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20161118212604, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20161118212613, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20161118212621, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20161118212630, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20161118212641, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20161118212649, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20161118212656, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20161118212758, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20161128234849, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20161230162221, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20170104113816, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20170105151732, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20170108191242, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20170109094020, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20170109130438, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20170110202752, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20170111133013, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20170117025759, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20170118191001, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20170119234632, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20170124230432, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20170127014618, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20170131232841, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20170223094154, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20170306075207, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20170309100733, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20170331111922, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20170502143928, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20170504130602, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20170509132100, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20170519105647, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20170519105648, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20170831234300, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20170831234301, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20170831234303, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20171116163618, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20171219164727, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20180620164811, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20180620175054, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20180620175055, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20191010101639, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20191010155147, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20191220130734, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20200311140000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20200405120000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20200407120000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20200420120000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20200504120000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20200512120000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20200707120000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20201011162341, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20201021104586, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20201102112520, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20201208121729, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20201215091637, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210119174155, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210326182902, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210421112652, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210506095025, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210513115729, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210526113559, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210601000001, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210601000002, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210601000003, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210601000004, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210601000005, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210601000006, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210601000007, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210601000008, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210606151329, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210616163757, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210617174723, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210622160235, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210623100031, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210623133615, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210708143152, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210709124443, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210712155608, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210714102108, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210719153709, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210721171531, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210723135713, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210802135933, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210806112844, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210810095603, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210811150223, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210818151827, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210818151828, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210818182258, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210819131107, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210819143446, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210903132338, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210915144307, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210920155130, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210927143115, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20210927143116, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20211013133706, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20211013133707, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20211102135149, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20211109121546, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20211110163320, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20211116184029, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20211116184030, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20211202092042, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20211202181033, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20211207161856, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20211216131203, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20211221110132, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220107155700, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220125105650, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220201084510, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220208144830, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220208144831, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220208144831, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220215152203, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220215152203, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220223113157, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220223113157, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220307104655, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220309133956, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220309133956, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220316155700, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220323152301, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220323152301, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220330100659, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220330100659, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220404091216, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220404091216, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220419140750, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220419140750, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220428140039, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220428140039, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220503134048, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220503134048, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220524102918, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220524102918, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220526123327, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220526123327, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220526123328, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220526123329, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220608113128, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220608113128, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220627104817, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220627104817, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220704101843, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220704101843, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220708095046, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220708095046, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220713091130, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220713091130, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220802135510, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220802135510, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220818101352, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220818101352, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220822161445, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220822161445, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220831100036, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220831100151, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220831100151, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220908181826, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220908181826, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220914154915, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220915165115, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220915165116, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220915165116, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220928100158, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20220928100158, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20221014084130, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20221014084130, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20221027085019, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20221101103952, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20221101103952, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20221104144401, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20221109100749, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20221109100749, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20221115104546, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20221115104546, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20221130114928, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20221205112142, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20221205112142, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20221216115820, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20221220195934, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20221220195934, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20221220195935, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20221223174807, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20221223174807, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20221227163855, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20221227163855, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20221227163856, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230202224725, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230202224725, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230206163608, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230206163608, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230214131519, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230214131519, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230303135738, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230303135738, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230313135301, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230313135301, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230313141819, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230313141819, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230315104937, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230315104937, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230317173844, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230317173844, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230320133602, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230320133602, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230330100011, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230330100011, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230330134823, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230330134823, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230405232025, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230405232025, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230408084104, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230408084104, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230411102858, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230411102858, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230421155932, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230421155932, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230425082126, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230425082126, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230425105727, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230425105727, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230501154913, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230501154913, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230503101418, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230503101418, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230515144206, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230515144206, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230517140952, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230517152807, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230517152807, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230518114155, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230518114155, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230520153236, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230520153236, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230525151159, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230530122103, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230530122103, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230602111827, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230602111827, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230608103123, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230608103123, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230629140529, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230629140530, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230629140530, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230711144622, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230711144622, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230721135421, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230721161508, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230721161508, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230726115701, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230807100822, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230814150442, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230814150442, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230823122728, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230823122728, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230906152143, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230906152143, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230911163618, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230911163618, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230912101759, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230912101759, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230915101341, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230915101341, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230918132351, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20230918132351, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231004144339, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231004144339, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231009094541, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231009094542, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231009094542, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231009094543, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231009094543, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231009094544, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231009094544, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231016091915, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231016091915, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231024174135, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231024174135, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231025120016, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231025120016, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231025160156, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231025160156, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231031165350, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231106144110, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231106144110, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231107130934, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231107130934, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231109115838, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231109115838, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231121054530, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231121054530, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231122101320, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231122101320, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231130132828, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231130132828, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231130132931, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231130132931, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231204155427, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231204155427, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231206142340, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231206142340, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231207102320, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231207102320, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231207102321, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231207102321, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231207133731, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231207133731, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231212094238, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231212094238, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231212095734, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231212161121, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231212161121, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231215122713, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231215122713, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231219143041, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231219143041, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231224070653, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20231224070653, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240110134315, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240110134315, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240119091637, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240119091637, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240126020642, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240126020642, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240126020643, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240126020643, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240129162819, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240129162819, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240130115133, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240130115133, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240131083822, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240131083822, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240205095928, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240205095928, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240205121956, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240209110212, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240209110212, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240212111533, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240221112844, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240221112844, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240222073518, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240222073518, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240222135115, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240222135115, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240226082255, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240226082255, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240228082706, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240228082706, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240301173035, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240301173035, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240302111134, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240302111134, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240312103753, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240313143416, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240314085226, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240314085226, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240314151747, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240314151747, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240320145650, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240320145650, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240327115530, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240327115617, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240408085837, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240408085837, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240415104633, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240415104633, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240430111727, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240430111727, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240515200020, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240521143023, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240521143023, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240521143024, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240521143024, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240601174138, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240601174138, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240607133721, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240607133721, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240612150059, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240612150059, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240613162201, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240613172616, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240613172616, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240618142419, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240618142419, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240625093543, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240625093543, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240626195531, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240626195531, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240702123921, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240703154849, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240703154849, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240707134035, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240707134035, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240707134036, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240707134036, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240709124958, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240709124958, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240709132642, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240709132642, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240709183940, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240709183940, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240710155623, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240710155623, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240723102712, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240725152735, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240725152735, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240725182118, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240725182118, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240726100517, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240726100517, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240730171504, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240730171504, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240730174056, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240730174056, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240730215453, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240730215453, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240730374423, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240730374423, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240801115359, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240801115359, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240802101043, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240802113716, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240802113716, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240814135330, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240814135330, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240815000000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240815000000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240815000001, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240816103247, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240816103247, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240820091218, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240826111228, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240826160025, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240826160025, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240829165448, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240829165448, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240829165605, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240829165605, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240829165715, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240829165930, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240829165930, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240829170023, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240829170023, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240829170033, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240829170033, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240829170044, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240905105135, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240905140514, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240905200000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240905200000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240905200001, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20240905200001, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241002104104, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241002104105, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241002104105, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241002104106, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241002104106, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241002210000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241002210000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241003145349, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241003145349, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241004005000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241004005000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241008083925, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241008083925, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241009090010, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241017163402, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241017163402, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241021224359, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241021224359, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241022140321, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241025111236, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241025112748, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241025141855, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241110152839, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241110152840, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241110152841, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241110152841, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241116233322, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241122171434, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241125150614, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241125150614, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241203125346, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241203125346, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241203130032, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241203130032, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241205122800, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241209164540, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241210140021, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241219180042, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241219180042, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241220100000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241220114903, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241220114903, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241220114904, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241224000000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241224000000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241230000000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20241231112624, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250102121439, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250121094045, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250121094045, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250121094500, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250121094600, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250121094600, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250121094700, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250121094700, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250124194347, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250124194347, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250127162751, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250127162751, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250213104005, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250214205657, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250217093329, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250217093329, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250219090511, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250219100000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250219100000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250219142401, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250224184002, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250225085436, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250226000000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250226153445, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250304162702, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250306144233, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250313163430, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250317130944, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250318165922, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250318165922, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250320132525, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250320200000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250320200000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250326161930, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250326161930, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250326161931, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250326161931, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250331042354, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250331154206, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250331154206, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250401155831, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250408133233, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250410104321, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250410104321, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250421085116, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250422095806, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250424153059, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250430103833, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250430112622, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250430112622, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250501162727, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250501162727, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250502154517, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250502222222, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250502222222, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250507170845, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250507170845, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250513162912, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250513162912, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250519161614, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250519161614, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250519170000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250520153848, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250528115932, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250529102706, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250603105558, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250609102714, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250609102714, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250609112613, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250613103810, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250616193950, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250616193950, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250624140757, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250624140757, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250626130239, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250629131032, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250701155654, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250701155654, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250707095725, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250716152435, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250716152435, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250718091828, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250718091828, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250728122229, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250731122715, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250731151000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250731151000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250803000000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250805083116, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250805083116, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250807140441, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250808000000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250811155036, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250813205039, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250813205039, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250814123333, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250815130115, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250815130115, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250816115553, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250817154557, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250825113751, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250827113140, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250828120836, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250902112642, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250902112642, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250904091745, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250904091745, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250905090000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250905090000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250922083056, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250922083056, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250923120000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250926123048, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20250926123048, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251015103505, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251015103505, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251015103600, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251015103700, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251015103700, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251015103800, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251015103800, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251015103900, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251015103900, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251028140000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251028140000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251028140100, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251028140100, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251028140110, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251028140110, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251028140200, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251028140200, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251028140300, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251028140300, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251028140400, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251028140400, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251031154558, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251031154558, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251103160848, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251103160848, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251104112849, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251106000000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251106000000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251107164629, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251107164629, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251107170854, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251107170854, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251110172137, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251110172137, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251111153133, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251111153133, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251117020000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251117020100, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251117020100, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251117020200, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251117020200, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251121100000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251121124239, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251121124239, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251124090450, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251124090450, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251124135808, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251124140138, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251124140138, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251124162948, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251124162948, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251127113559, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251202162232, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251203170808, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251203170808, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251207050413, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251207050413, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251208215800, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251209221730, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251209221730, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251209221850, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251215163721, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251217000000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251217000000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251217120000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251217120000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251229000000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251229000010, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251229000020, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20251229000020, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260106000000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260106000000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260108200708, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260108214732, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260109231821, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260109231821, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260113012054, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260113012054, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260124200020, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260124200020, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260126150840, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260126150840, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260126210724, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260202151756, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260205184907, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260210151544, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260210155109, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260210155109, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260210181120, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260210181120, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260211200153, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260211200153, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260217141240, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260217141240, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260217200906, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260218175704, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260314120000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260316120000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260316120001, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260316120001, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260316120002, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260316120002, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260316120003, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260316120004, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260316120005, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260316120006, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260316120006, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260316120007, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260316120007, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260316120008, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260316120009, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260316120010, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260316120010, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260316120011, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260317120000, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260318184559, true);
INSERT INTO migration_status_tables (version_id, is_applied) VALUES (20260318184559, true);
INSERT INTO migration_status_data (version_id, is_applied) VALUES (20161229171615, true);
INSERT INTO migration_status_data (version_id, is_applied) VALUES (20170223171234, true);
INSERT INTO migration_status_data (version_id, is_applied) VALUES (20170301093653, true);
INSERT INTO migration_status_data (version_id, is_applied) VALUES (20170314151620, true);
INSERT INTO migration_status_data (version_id, is_applied) VALUES (20181119180000, true);
INSERT INTO migration_status_data (version_id, is_applied) VALUES (20210330130314, true);
INSERT INTO migration_status_data (version_id, is_applied) VALUES (20210806135609, true);
INSERT INTO migration_status_data (version_id, is_applied) VALUES (20210819120215, true);
INSERT INTO migration_status_data (version_id, is_applied) VALUES (20230525175650, true);
-- Manual fixes for 12 tables the converter can't handle
DROP TABLE IF EXISTS "abm_tokens" CASCADE;
CREATE TABLE "abm_tokens" (
  "id" integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  "organization_name" varchar(255) NOT NULL,
  "token_encrypted" bytea NOT NULL,
  "macos_default_team_id" integer, "ios_default_team_id" integer, "ipados_default_team_id" integer,
  "terms_expired" boolean NOT NULL DEFAULT FALSE,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP, "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT "idx_abm_tokens_organization_name" UNIQUE ("organization_name"));

DROP TABLE IF EXISTS "carve_metadata" CASCADE;
CREATE TABLE "carve_metadata" ("id" integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  "host_id" integer NOT NULL, "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "name" varchar(255) DEFAULT '', "block_count" integer NOT NULL DEFAULT 0,
  "block_size" integer NOT NULL DEFAULT 0, "carve_size" bigint NOT NULL DEFAULT 0,
  "carve_id" varchar(255) NOT NULL, "request_id" varchar(255) NOT NULL,
  "session_id" varchar(255) NOT NULL, "expired" boolean DEFAULT FALSE,
  "max_block" integer NOT NULL DEFAULT -1, "error" text);

DROP TABLE IF EXISTS "host_mdm_apple_bootstrap_packages" CASCADE;
CREATE TABLE "host_mdm_apple_bootstrap_packages" ("host_uuid" varchar(255) NOT NULL PRIMARY KEY,
  "command_uuid" varchar(255), "skipped" boolean DEFAULT FALSE);

DROP TABLE IF EXISTS "host_mdm_windows_profiles" CASCADE;
CREATE TABLE "host_mdm_windows_profiles" ("host_uuid" varchar(255) NOT NULL, "profile_uuid" varchar(37) NOT NULL,
  "profile_name" varchar(255) NOT NULL DEFAULT '', "status" text, "operation_type" text,
  "detail" text NOT NULL DEFAULT '', "command_uuid" varchar(255) NOT NULL DEFAULT '',
  "retries" smallint NOT NULL DEFAULT 0, PRIMARY KEY ("host_uuid","profile_uuid"));

DROP TABLE IF EXISTS "mdm_android_configuration_profiles" CASCADE;
CREATE TABLE "mdm_android_configuration_profiles" ("name" varchar(255) NOT NULL, "team_id" integer,
  "raw_json" text NOT NULL, "profile_uuid" varchar(37) NOT NULL DEFAULT '' PRIMARY KEY,
  "uploaded_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP, "secrets_updated_at" timestamp, "token" bytea);

DROP TABLE IF EXISTS "mdm_apple_declarations" CASCADE;
CREATE TABLE "mdm_apple_declarations" ("declaration_uuid" varchar(37) NOT NULL PRIMARY KEY,
  "team_id" integer, "identifier" varchar(256) NOT NULL DEFAULT '', "name" varchar(256) NOT NULL DEFAULT '',
  "raw_json" text NOT NULL, "checksum" bytea NOT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP, "uploaded_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP);

DROP TABLE IF EXISTS "mdm_apple_enrollment_profiles" CASCADE;
CREATE TABLE "mdm_apple_enrollment_profiles" ("id" integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  "token" bytea, "type" text NOT NULL DEFAULT 'automatic', "dep_profile" jsonb,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP, "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP);

DROP TABLE IF EXISTS "mdm_configuration_profile_labels" CASCADE;
CREATE TABLE "mdm_configuration_profile_labels" ("id" integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  "apple_profile_uuid" varchar(37), "windows_profile_uuid" varchar(37), "declaration_uuid" varchar(37),
  "label_name" varchar(255) NOT NULL DEFAULT '', "label_id" integer,
  "exclude" boolean NOT NULL DEFAULT FALSE, "require_all" boolean NOT NULL DEFAULT FALSE);

DROP TABLE IF EXISTS "mdm_windows_configuration_profiles" CASCADE;
CREATE TABLE "mdm_windows_configuration_profiles" ("profile_uuid" varchar(37) NOT NULL DEFAULT '' PRIMARY KEY,
  "team_id" integer, "name" varchar(255) NOT NULL DEFAULT '', "syncml" text NOT NULL,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP, "uploaded_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "secrets_updated_at" timestamp, "checksum" bytea);

DROP TABLE IF EXISTS "operating_system_version_vulnerabilities" CASCADE;
CREATE TABLE "operating_system_version_vulnerabilities" ("id" bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  "os_version_id" integer NOT NULL, "cve" varchar(255) NOT NULL, "source" smallint DEFAULT 0,
  "resolved_in_version" varchar(255), "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP, "team_id" integer NOT NULL DEFAULT 0);

DROP TABLE IF EXISTS "policy_stats" CASCADE;
CREATE TABLE "policy_stats" ("id" bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  "policy_id" integer NOT NULL, "inherited_team_id" integer,
  "passing_host_count" integer NOT NULL DEFAULT 0, "failing_host_count" integer NOT NULL DEFAULT 0,
  "created_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP, "updated_at" timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP);
