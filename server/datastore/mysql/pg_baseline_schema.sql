-- Fleet PostgreSQL Baseline Schema
-- Generated from production database via pg_dump --no-owner --no-privileges.
-- To regenerate:
--   kubectl exec -n fleet fleet-db-1 -- pg_dump -U postgres -d fleet \
--     --schema-only --no-owner --no-privileges
-- Then strip:
--   - leading `\restrict <token>` and trailing `\unrestrict <token>` psql meta-commands
--     (pg_dump 17+ emits these; db.Exec fails on the backslash)
--   - the SET/SELECT pg_catalog preamble (especially set_config('search_path',''))
--     since the embedded loader runs seed inserts that expect search_path=public
--
-- Bump the marker below to the highest applied migration on the source DB at
-- regen time. It is parsed by migratePGBaseline to (a) seed
-- migration_status_tables on a fresh apply so MigrationStatus reports the
-- right state, and (b) detect drift when the running code carries newer
-- migrations than this baseline knows about.
--
-- Get the value with:
--   kubectl exec -n fleet fleet-db-1 -- psql -U postgres -d fleet -tAc \
--     "SELECT MAX(version_id) FROM migration_status_tables WHERE is_applied"
--
-- After bumping, verify locally before pushing:
--   go test -count=1 -run TestVersionsAbove_EmbeddedBaselineCoversAllCode \
--     ./server/datastore/mysql/
-- Then run the schema-drift validator:
--   make check-pg-compat
--
-- pg-baseline-up-to-migration: 20260729120000
--
--
-- PostgreSQL database dump
--


-- Dumped from database version 16.14 (Debian 16.14-1.pgdg13+1)
-- Dumped by pg_dump version 16.14 (Debian 16.14-1.pgdg13+1)


--
-- Name: calendar_events_set_uuid(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.calendar_events_set_uuid() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
		DECLARE h text;
		BEGIN
			h := upper(encode(NEW.uuid_bin, 'hex'));
			NEW.uuid := substr(h,1,8) || '-' || substr(h,9,4) || '-' || substr(h,13,4) || '-' || substr(h,17,4) || '-' || substr(h,21,12);
			RETURN NEW;
		END $$;


--
-- Name: fleet_set_updated_at(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.fleet_set_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW IS DISTINCT FROM OLD
       AND NEW.updated_at IS NOT DISTINCT FROM OLD.updated_at THEN
        NEW.updated_at = CURRENT_TIMESTAMP;
    END IF;
    RETURN NEW;
END;
$$;


--
-- Name: fleet_software_titles_set_unique_id(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.fleet_software_titles_set_unique_id() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.unique_identifier = COALESCE(
        NULLIF(NEW.bundle_identifier, ''),
        NULLIF(NEW.application_id, ''),
        NULLIF(NEW.upgrade_code, ''),
        NEW.name
    );
    RETURN NEW;
END;
$$;


--
-- Name: fleet_touch_column(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.fleet_touch_column() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW IS DISTINCT FROM OLD
       AND (to_jsonb(NEW)->TG_ARGV[0]) IS NOT DISTINCT FROM (to_jsonb(OLD)->TG_ARGV[0]) THEN
        NEW := jsonb_populate_record(NEW, jsonb_build_object(TG_ARGV[0], CURRENT_TIMESTAMP));
    END IF;
    RETURN NEW;
END $$;


--
-- Name: host_mdm_set_enrollment_status(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.host_mdm_set_enrollment_status() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
		BEGIN
			NEW.enrollment_status :=
				CASE
					WHEN NEW.is_server = true THEN NULL
					WHEN NEW.enrolled = true AND NEW.installed_from_dep = false AND NEW.is_personal_enrollment = true THEN 'On (manual - personal)'
					WHEN NEW.enrolled = true AND NEW.installed_from_dep = false AND NEW.is_personal_enrollment = false THEN 'On (manual)'
					WHEN NEW.enrolled = true AND NEW.installed_from_dep = true AND NEW.is_personal_enrollment = false THEN 'On (automatic)'
					WHEN NEW.enrolled = false AND NEW.installed_from_dep = true THEN 'Pending'
					WHEN NEW.enrolled = false AND NEW.installed_from_dep = false THEN 'Off'
					ELSE NULL
				END;
			RETURN NEW;
		END $$;


--
-- Name: host_software_installs_set_statuses(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.host_software_installs_set_statuses() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
		DECLARE
			exec_status text;
		BEGIN
			exec_status :=
				CASE
					WHEN NEW.canceled = true AND NEW.uninstall = false THEN 'canceled_install'
					WHEN NEW.canceled = true AND NEW.uninstall = true THEN 'canceled_uninstall'
					WHEN NEW.install_script_exit_code IS NOT NULL AND NEW.install_script_exit_code <> 0 THEN 'failed_install'
					WHEN NEW.post_install_script_exit_code IS NOT NULL AND NEW.post_install_script_exit_code = 0 THEN 'installed'
					WHEN NEW.post_install_script_exit_code IS NOT NULL AND NEW.post_install_script_exit_code <> 0 THEN 'failed_install'
					WHEN NEW.install_script_exit_code IS NOT NULL AND NEW.install_script_exit_code = 0 THEN 'installed'
					WHEN NEW.pre_install_query_output IS NOT NULL AND NEW.pre_install_query_output = '' THEN 'failed_install'
					WHEN NEW.host_id IS NOT NULL AND NEW.uninstall = false THEN 'pending_install'
					WHEN NEW.uninstall_script_exit_code IS NOT NULL AND NEW.uninstall_script_exit_code <> 0 THEN 'failed_uninstall'
					WHEN NEW.uninstall_script_exit_code IS NOT NULL AND NEW.uninstall_script_exit_code = 0 THEN NULL
					WHEN NEW.host_id IS NOT NULL AND NEW.uninstall = true THEN 'pending_uninstall'
					ELSE NULL
				END;
			NEW.execution_status := exec_status;
			NEW.status := CASE WHEN NEW.removed = true THEN NULL ELSE exec_status END;
			RETURN NEW;
		END $$;


--
-- Name: mdm_apple_declarations_set_token(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.mdm_apple_declarations_set_token() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
		BEGIN
			NEW.token := decode(md5(NEW.raw_json::text || COALESCE(extract(epoch from NEW.secrets_updated_at)::text, '')), 'hex');
			RETURN NEW;
		END $$;


--
-- Name: mdm_windows_configuration_profiles_set_checksum(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.mdm_windows_configuration_profiles_set_checksum() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
		BEGIN
			NEW.checksum := decode(md5(NEW.syncml), 'hex');
			RETURN NEW;
		END $$;


--
-- Name: software_titles_set_additional_identifier(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.software_titles_set_additional_identifier() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
		BEGIN
			NEW.additional_identifier :=
				CASE
					WHEN NEW.source = 'ios_apps' THEN 1
					WHEN NEW.source = 'ipados_apps' THEN 2
					WHEN NEW.bundle_identifier IS NOT NULL THEN 0
					ELSE NULL
				END;
			RETURN NEW;
		END $$;




--
-- Name: abm_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.abm_tokens (
    id integer NOT NULL,
    organization_name character varying(255) NOT NULL,
    apple_id character varying(255) NOT NULL,
    terms_expired boolean DEFAULT false NOT NULL,
    renew_at timestamp without time zone NOT NULL,
    token bytea NOT NULL,
    macos_default_team_id integer,
    ios_default_team_id integer,
    ipados_default_team_id integer,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    byod_default_team_id integer,
    enrollment_url_token bytea NOT NULL,
    token_invalid smallint DEFAULT '0'::smallint NOT NULL,
    CONSTRAINT abm_tokens_enroll_url_length CHECK ((length(enrollment_url_token) > 32))
);


--
-- Name: abm_tokens_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.abm_tokens ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.abm_tokens_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: acme_accounts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.acme_accounts (
    id integer NOT NULL,
    acme_enrollment_id integer NOT NULL,
    json_web_key jsonb NOT NULL,
    json_web_key_thumbprint character varying(45) NOT NULL,
    revoked boolean DEFAULT false NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: acme_accounts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.acme_accounts ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.acme_accounts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: acme_authorizations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.acme_authorizations (
    id integer NOT NULL,
    identifier_type character varying(255) NOT NULL,
    identifier_value character varying(255) NOT NULL,
    acme_order_id integer NOT NULL,
    status character varying(16) DEFAULT 'pending'::character varying NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT acme_authorizations_status_check CHECK (((status)::text = ANY (ARRAY[('pending'::character varying)::text, ('valid'::character varying)::text, ('invalid'::character varying)::text, ('deactivated'::character varying)::text, ('expired'::character varying)::text, ('revoked'::character varying)::text])))
);


--
-- Name: acme_authorizations_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.acme_authorizations ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.acme_authorizations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: acme_challenges; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.acme_challenges (
    id integer NOT NULL,
    challenge_type character varying(64) NOT NULL,
    token character varying(64) NOT NULL,
    acme_authorization_id integer NOT NULL,
    status character varying(16) DEFAULT 'pending'::character varying NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT acme_challenges_status_check CHECK (((status)::text = ANY (ARRAY[('pending'::character varying)::text, ('valid'::character varying)::text, ('invalid'::character varying)::text, ('processing'::character varying)::text])))
);


--
-- Name: acme_challenges_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.acme_challenges ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.acme_challenges_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: acme_enrollments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.acme_enrollments (
    id integer NOT NULL,
    path_identifier character varying(64) NOT NULL,
    host_identifier character varying(255) NOT NULL,
    not_valid_after timestamp without time zone,
    revoked boolean DEFAULT false NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: acme_enrollments_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.acme_enrollments ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.acme_enrollments_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: acme_orders; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.acme_orders (
    id integer NOT NULL,
    acme_account_id integer NOT NULL,
    finalized smallint DEFAULT 0 NOT NULL,
    certificate_signing_request text NOT NULL,
    identifiers jsonb NOT NULL,
    status character varying(16) DEFAULT 'pending'::character varying NOT NULL,
    issued_certificate_serial bigint,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT acme_orders_status_check CHECK (((status)::text = ANY (ARRAY[('pending'::character varying)::text, ('ready'::character varying)::text, ('processing'::character varying)::text, ('valid'::character varying)::text, ('invalid'::character varying)::text])))
);


--
-- Name: acme_orders_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.acme_orders ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.acme_orders_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: activity_host_past; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.activity_host_past (
    host_id integer NOT NULL,
    activity_id integer NOT NULL
);


--
-- Name: activity_past; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.activity_past (
    id integer NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    user_id integer,
    user_name character varying(255) DEFAULT NULL::character varying,
    activity_type character varying(255) NOT NULL,
    details jsonb,
    streamed boolean DEFAULT false NOT NULL,
    user_email character varying(255) DEFAULT ''::character varying NOT NULL,
    fleet_initiated boolean DEFAULT false NOT NULL,
    host_only boolean DEFAULT false NOT NULL
);


--
-- Name: activity_past_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.activity_past ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.activity_past_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: aggregated_stats; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.aggregated_stats (
    id bigint NOT NULL,
    type character varying(255) NOT NULL,
    json_value jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    global_stats boolean DEFAULT false NOT NULL
);


--
-- Name: android_app_configurations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.android_app_configurations (
    id integer NOT NULL,
    application_id character varying(255) NOT NULL,
    team_id integer,
    global_or_team_id integer DEFAULT 0 NOT NULL,
    configuration jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: android_app_configurations_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.android_app_configurations ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.android_app_configurations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: android_devices; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.android_devices (
    id integer NOT NULL,
    host_id integer NOT NULL,
    device_id character varying(32) NOT NULL,
    enterprise_specific_id character varying(64) DEFAULT NULL::character varying,
    last_policy_sync_time timestamp without time zone,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    applied_policy_id character varying(100) DEFAULT NULL::character varying,
    applied_policy_version integer,
    team_id integer
);


--
-- Name: android_devices_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.android_devices ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.android_devices_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: android_enterprises; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.android_enterprises (
    id integer NOT NULL,
    signup_name character varying(63) DEFAULT ''::character varying NOT NULL,
    enterprise_id character varying(63) DEFAULT ''::character varying NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    signup_token character varying(64) DEFAULT ''::character varying NOT NULL,
    pubsub_topic_id character varying(64) DEFAULT ''::character varying NOT NULL,
    user_id integer DEFAULT 0 NOT NULL
);


--
-- Name: android_enterprises_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.android_enterprises ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.android_enterprises_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: android_policy_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.android_policy_requests (
    request_uuid character varying(36) NOT NULL,
    request_name character varying(255) NOT NULL,
    policy_id character varying(100) NOT NULL,
    payload jsonb NOT NULL,
    status_code integer NOT NULL,
    error_details text,
    applied_policy_version integer,
    policy_version integer,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: app_config_json; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.app_config_json (
    id integer DEFAULT 1 NOT NULL,
    json_value jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: batch_activities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.batch_activities (
    id integer NOT NULL,
    script_id integer NOT NULL,
    execution_id character varying(255) NOT NULL,
    user_id integer,
    job_id integer,
    status character varying(255) DEFAULT NULL::character varying,
    activity_type character varying(255) DEFAULT NULL::character varying,
    num_targeted integer,
    num_pending integer,
    num_ran integer,
    num_errored integer,
    num_incompatible integer,
    num_canceled integer,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    started_at timestamp without time zone,
    finished_at timestamp without time zone,
    canceled boolean DEFAULT false
);


--
-- Name: batch_activities_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.batch_activities ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.batch_activities_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: batch_activity_host_results; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.batch_activity_host_results (
    id integer NOT NULL,
    batch_execution_id character varying(255) NOT NULL,
    host_id integer NOT NULL,
    host_execution_id character varying(255) DEFAULT NULL::character varying,
    error character varying(255) DEFAULT NULL::character varying,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: batch_activity_host_results_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.batch_activity_host_results ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.batch_activity_host_results_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: ca_config_assets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ca_config_assets (
    id integer NOT NULL,
    type text NOT NULL,
    name character varying(255) NOT NULL,
    value bytea NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: ca_config_assets_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.ca_config_assets ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.ca_config_assets_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: calendar_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.calendar_events (
    id integer NOT NULL,
    email character varying(255) NOT NULL,
    start_time timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    end_time timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    event jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    timezone character varying(64) DEFAULT NULL::character varying,
    uuid_bin bytea NOT NULL,
    uuid text
);


--
-- Name: calendar_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.calendar_events ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.calendar_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: carve_blocks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.carve_blocks (
    metadata_id integer NOT NULL,
    block_id integer NOT NULL,
    data bytea
);


--
-- Name: carve_metadata; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.carve_metadata (
    id integer NOT NULL,
    host_id integer NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    name character varying(255) DEFAULT NULL::character varying,
    block_count integer NOT NULL,
    block_size integer NOT NULL,
    carve_size bigint NOT NULL,
    carve_id character varying(64) NOT NULL,
    request_id character varying(64) NOT NULL,
    session_id character varying(255) NOT NULL,
    expired smallint DEFAULT 0,
    max_block integer DEFAULT '-1'::integer,
    error text
);


--
-- Name: carve_metadata_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.carve_metadata ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.carve_metadata_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: certificate_authorities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.certificate_authorities (
    id integer NOT NULL,
    type text NOT NULL,
    name character varying(255) NOT NULL,
    url text NOT NULL,
    api_token_encrypted bytea,
    profile_id character varying(255) DEFAULT NULL::character varying,
    certificate_common_name character varying(255) DEFAULT NULL::character varying,
    certificate_user_principal_names jsonb,
    certificate_seat_id character varying(255) DEFAULT NULL::character varying,
    admin_url text,
    username character varying(255) DEFAULT NULL::character varying,
    password_encrypted bytea,
    challenge_url text,
    challenge_encrypted bytea,
    client_id character varying(255) DEFAULT NULL::character varying,
    client_secret_encrypted bytea,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: certificate_authorities_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.certificate_authorities ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.certificate_authorities_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: certificate_templates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.certificate_templates (
    id integer NOT NULL,
    team_id integer NOT NULL,
    certificate_authority_id integer NOT NULL,
    name character varying(255) NOT NULL,
    subject_name text NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    subject_alternative_name text
);


--
-- Name: certificate_templates_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.certificate_templates ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.certificate_templates_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: challenges; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.challenges (
    challenge character(32) NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: conditional_access_scep_certificates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.conditional_access_scep_certificates (
    serial bigint NOT NULL,
    host_id integer NOT NULL,
    name character varying(64) NOT NULL,
    not_valid_before timestamp without time zone NOT NULL,
    not_valid_after timestamp without time zone NOT NULL,
    certificate_pem text NOT NULL,
    revoked boolean DEFAULT false NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT conditional_access_scep_certificates_chk_1 CHECK ((substr(certificate_pem, 1, 27) = '-----BEGIN CERTIFICATE-----'::text))
);


--
-- Name: conditional_access_scep_serials; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.conditional_access_scep_serials (
    serial bigint NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: conditional_access_scep_serials_serial_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.conditional_access_scep_serials ALTER COLUMN serial ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.conditional_access_scep_serials_serial_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: cron_stats; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cron_stats (
    id integer NOT NULL,
    name character varying(255) NOT NULL,
    instance character varying(255) NOT NULL,
    stats_type character varying(255) NOT NULL,
    status character varying(255) NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    errors jsonb
);


--
-- Name: cron_stats_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.cron_stats ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.cron_stats_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: custom_host_vitals; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.custom_host_vitals (
    id integer NOT NULL,
    name character varying(255) NOT NULL,
    created_at timestamp(6) without time zone DEFAULT now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT now() NOT NULL
);


--
-- Name: custom_host_vitals_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.custom_host_vitals ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.custom_host_vitals_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: cve_meta; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cve_meta (
    cve character varying(20) NOT NULL,
    cvss_score double precision,
    epss_probability double precision,
    cisa_known_exploit boolean,
    published timestamp without time zone,
    description text
);


--
-- Name: default_team_config_json; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.default_team_config_json (
    id integer DEFAULT 1 NOT NULL,
    json_value jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT default_team_config_id CHECK ((id = 1))
);


--
-- Name: distributed_query_campaign_targets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.distributed_query_campaign_targets (
    id integer NOT NULL,
    type integer,
    distributed_query_campaign_id integer,
    target_id integer
);


--
-- Name: distributed_query_campaign_targets_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.distributed_query_campaign_targets ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.distributed_query_campaign_targets_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: distributed_query_campaigns; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.distributed_query_campaigns (
    id integer NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    query_id integer,
    status integer,
    user_id integer
);


--
-- Name: distributed_query_campaigns_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.distributed_query_campaigns ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.distributed_query_campaigns_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: email_changes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.email_changes (
    id integer NOT NULL,
    user_id integer NOT NULL,
    token character varying(128) NOT NULL,
    new_email character varying(255) NOT NULL
);


--
-- Name: email_changes_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.email_changes ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.email_changes_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: enroll_secrets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.enroll_secrets (
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    secret character varying(255) NOT NULL,
    team_id integer
);


--
-- Name: eulas; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.eulas (
    id integer NOT NULL,
    token character varying(36) DEFAULT NULL::character varying,
    name character varying(255) DEFAULT NULL::character varying,
    bytes bytea,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    sha256 bytea
);


--
-- Name: fleet_maintained_apps; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.fleet_maintained_apps (
    id integer NOT NULL,
    name character varying(255) NOT NULL,
    slug character varying(255) NOT NULL,
    platform character varying(255) NOT NULL,
    unique_identifier character varying(255) NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: fleet_maintained_apps_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.fleet_maintained_apps ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.fleet_maintained_apps_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: fleet_variables; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.fleet_variables (
    id integer NOT NULL,
    name character varying(255) DEFAULT ''::character varying NOT NULL,
    is_prefix boolean DEFAULT false NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: fleet_variables_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.fleet_variables ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.fleet_variables_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: host_additional; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_additional (
    host_id integer NOT NULL,
    additional jsonb
);


--
-- Name: host_batteries; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_batteries (
    id integer NOT NULL,
    host_id integer NOT NULL,
    serial_number character varying(255) NOT NULL,
    cycle_count integer NOT NULL,
    health character varying(40) NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: host_batteries_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.host_batteries ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.host_batteries_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: host_calendar_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_calendar_events (
    id integer NOT NULL,
    host_id integer NOT NULL,
    calendar_event_id integer NOT NULL,
    webhook_status smallint NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: host_calendar_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.host_calendar_events ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.host_calendar_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: host_certificate_sources; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_certificate_sources (
    id bigint NOT NULL,
    host_certificate_id bigint NOT NULL,
    source text NOT NULL,
    username character varying(255) NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: host_certificate_sources_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.host_certificate_sources ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.host_certificate_sources_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: host_certificate_templates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_certificate_templates (
    id integer NOT NULL,
    host_uuid character varying(255) NOT NULL,
    certificate_template_id integer NOT NULL,
    fleet_challenge character(32) DEFAULT NULL::bpchar,
    status character varying(20) DEFAULT 'pending'::character varying NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    detail text,
    operation_type character varying(20) DEFAULT 'install'::character varying NOT NULL,
    name character varying(255) NOT NULL,
    uuid uuid,
    not_valid_before timestamp without time zone,
    not_valid_after timestamp without time zone,
    serial character varying(40) DEFAULT NULL::character varying,
    retry_count integer DEFAULT 0 NOT NULL
);


--
-- Name: host_certificate_templates_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.host_certificate_templates ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.host_certificate_templates_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: host_certificates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_certificates (
    id bigint NOT NULL,
    host_id integer NOT NULL,
    not_valid_after timestamp without time zone NOT NULL,
    not_valid_before timestamp without time zone NOT NULL,
    certificate_authority boolean NOT NULL,
    common_name character varying(255) NOT NULL,
    key_algorithm character varying(255) NOT NULL,
    key_strength integer NOT NULL,
    key_usage character varying(255) NOT NULL,
    serial character varying(255) NOT NULL,
    signing_algorithm character varying(255) NOT NULL,
    subject_country character varying(32) NOT NULL,
    subject_org character varying(255) NOT NULL,
    subject_org_unit character varying(255) NOT NULL,
    subject_common_name character varying(255) NOT NULL,
    issuer_country character varying(32) NOT NULL,
    issuer_org character varying(255) NOT NULL,
    issuer_org_unit character varying(255) NOT NULL,
    issuer_common_name character varying(255) NOT NULL,
    sha1_sum bytea NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    deleted_at timestamp without time zone,
    origin character varying(255) DEFAULT 'osquery'::character varying NOT NULL,
    CONSTRAINT host_certificates_origin_check CHECK (((origin)::text = ANY (ARRAY[('osquery'::character varying)::text, ('mdm'::character varying)::text])))
);


--
-- Name: host_certificates_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.host_certificates ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.host_certificates_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: host_conditional_access; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_conditional_access (
    id integer NOT NULL,
    host_id integer NOT NULL,
    bypassed_at timestamp without time zone,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: host_conditional_access_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.host_conditional_access ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.host_conditional_access_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: host_custom_host_vitals; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_custom_host_vitals (
    id integer NOT NULL,
    host_id integer NOT NULL,
    custom_host_vital_id integer NOT NULL,
    value text NOT NULL,
    created_at timestamp(6) without time zone DEFAULT now() NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT now() NOT NULL
);


--
-- Name: host_custom_host_vitals_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.host_custom_host_vitals ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.host_custom_host_vitals_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: host_dep_assignments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_dep_assignments (
    host_id integer NOT NULL,
    added_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    deleted_at timestamp without time zone,
    profile_uuid character varying(37) DEFAULT NULL::character varying,
    assign_profile_response character varying(15) DEFAULT NULL::character varying,
    response_updated_at timestamp without time zone,
    retry_job_id integer DEFAULT 0 NOT NULL,
    abm_token_id integer,
    mdm_migration_deadline timestamp without time zone,
    mdm_migration_completed timestamp without time zone,
    hardware_serial character varying(255) DEFAULT ''::character varying NOT NULL
);


--
-- Name: host_device_auth; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_device_auth (
    host_id integer NOT NULL,
    token character varying(255) NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    previous_token character varying(255) DEFAULT NULL::character varying
);


--
-- Name: host_disk_encryption_keys; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_disk_encryption_keys (
    host_id integer NOT NULL,
    base64_encrypted text NOT NULL,
    base64_encrypted_salt character varying(255) DEFAULT ''::character varying NOT NULL,
    key_slot smallint,
    decryptable boolean,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    reset_requested boolean DEFAULT false NOT NULL,
    client_error character varying(255) DEFAULT ''::character varying NOT NULL
);


--
-- Name: host_disk_encryption_keys_archive; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_disk_encryption_keys_archive (
    id bigint NOT NULL,
    host_id integer NOT NULL,
    hardware_serial character varying(255) DEFAULT ''::character varying NOT NULL,
    base64_encrypted text NOT NULL,
    base64_encrypted_salt character varying(255) DEFAULT ''::character varying NOT NULL,
    key_slot smallint,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: host_disk_encryption_keys_archive_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.host_disk_encryption_keys_archive ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.host_disk_encryption_keys_archive_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: host_disks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_disks (
    host_id integer NOT NULL,
    gigs_disk_space_available numeric(10,2) DEFAULT 0.00 NOT NULL,
    percent_disk_space_available numeric(10,2) DEFAULT 0.00 NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    encrypted boolean,
    gigs_total_disk_space numeric(10,2) DEFAULT 0.00 NOT NULL,
    tpm_pin_set boolean DEFAULT false,
    gigs_all_disk_space numeric(10,2) DEFAULT NULL::numeric,
    bitlocker_protection_status smallint
);


--
-- Name: host_display_names; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_display_names (
    host_id integer NOT NULL,
    display_name character varying(255) NOT NULL
);


--
-- Name: host_emails; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_emails (
    id integer NOT NULL,
    host_id integer NOT NULL,
    email character varying(255) NOT NULL,
    source character varying(255) NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: host_emails_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.host_emails ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.host_emails_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: host_identity_scep_certificates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_identity_scep_certificates (
    serial bigint NOT NULL,
    host_id integer,
    name character varying(255) NOT NULL,
    not_valid_before timestamp without time zone NOT NULL,
    not_valid_after timestamp without time zone NOT NULL,
    certificate_pem text NOT NULL,
    public_key_raw bytea NOT NULL,
    revoked boolean DEFAULT false NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT host_identity_scep_certificates_chk_1 CHECK ((substr(certificate_pem, 1, 27) = '-----BEGIN CERTIFICATE-----'::text))
);


--
-- Name: host_identity_scep_serials; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_identity_scep_serials (
    serial bigint NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: host_identity_scep_serials_serial_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.host_identity_scep_serials ALTER COLUMN serial ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.host_identity_scep_serials_serial_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: host_in_house_software_installs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_in_house_software_installs (
    id integer NOT NULL,
    host_id integer NOT NULL,
    in_house_app_id integer NOT NULL,
    command_uuid character varying(127) NOT NULL,
    user_id integer,
    platform character varying(10) NOT NULL,
    removed boolean DEFAULT false NOT NULL,
    canceled boolean DEFAULT false NOT NULL,
    verification_command_uuid character varying(127) DEFAULT NULL::character varying,
    verification_at timestamp without time zone,
    verification_failed_at timestamp without time zone,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    self_service boolean DEFAULT false NOT NULL
);


--
-- Name: host_in_house_software_installs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.host_in_house_software_installs ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.host_in_house_software_installs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: host_issues; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_issues (
    host_id integer NOT NULL,
    failing_policies_count integer DEFAULT 0 NOT NULL,
    critical_vulnerabilities_count integer DEFAULT 0 NOT NULL,
    total_issues_count integer DEFAULT 0 NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: host_last_known_locations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_last_known_locations (
    host_id integer NOT NULL,
    latitude numeric(10,8) DEFAULT NULL::numeric,
    longitude numeric(11,8) DEFAULT NULL::numeric,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: host_managed_local_account_passwords; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_managed_local_account_passwords (
    host_uuid character varying(255) NOT NULL,
    encrypted_password bytea NOT NULL,
    command_uuid character varying(127) NOT NULL,
    status character varying(20) DEFAULT NULL::character varying,
    created_at timestamp(6) without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    account_uuid character varying(36) DEFAULT NULL::character varying,
    auto_rotate_at timestamp(6) without time zone DEFAULT NULL::timestamp without time zone,
    pending_encrypted_password bytea,
    pending_command_uuid character varying(127) DEFAULT NULL::character varying,
    initiated_by_fleet smallint DEFAULT 0 NOT NULL
);


--
-- Name: host_mdm; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_mdm (
    host_id integer NOT NULL,
    enrolled boolean DEFAULT false NOT NULL,
    server_url character varying(255) DEFAULT ''::character varying NOT NULL,
    installed_from_dep boolean DEFAULT false NOT NULL,
    mdm_id integer,
    is_server boolean,
    fleet_enroll_ref character varying(36) DEFAULT ''::character varying NOT NULL,
    enrollment_status text,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    is_personal_enrollment boolean DEFAULT false NOT NULL,
    managed_apple_id character varying(255) DEFAULT NULL::character varying
);


--
-- Name: host_mdm_actions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_mdm_actions (
    host_id integer NOT NULL,
    lock_ref character varying(36) DEFAULT NULL::character varying,
    wipe_ref character varying(36) DEFAULT NULL::character varying,
    unlock_pin character varying(6) DEFAULT NULL::character varying,
    unlock_ref character varying(36) DEFAULT NULL::character varying,
    fleet_platform character varying(255) DEFAULT ''::character varying NOT NULL,
    clear_passcode_ref character varying(36) DEFAULT NULL::character varying
);


--
-- Name: host_mdm_android_profiles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_mdm_android_profiles (
    host_uuid character varying(255) NOT NULL,
    status character varying(20) DEFAULT NULL::character varying,
    operation_type character varying(20) DEFAULT NULL::character varying,
    detail text,
    profile_uuid character varying(37) DEFAULT ''::character varying NOT NULL,
    profile_name character varying(255) DEFAULT ''::character varying NOT NULL,
    policy_request_uuid character varying(36) DEFAULT NULL::character varying,
    device_request_uuid character varying(36) DEFAULT NULL::character varying,
    request_fail_count smallint DEFAULT '0'::smallint NOT NULL,
    included_in_policy_version integer,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    can_reverify boolean DEFAULT false NOT NULL,
    checksum bytea DEFAULT '\x00000000000000000000000000000000'::bytea NOT NULL
);


--
-- Name: host_mdm_apple_awaiting_configuration; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_mdm_apple_awaiting_configuration (
    host_uuid character varying(255) NOT NULL,
    awaiting_configuration boolean DEFAULT false NOT NULL
);


--
-- Name: host_mdm_apple_bootstrap_packages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_mdm_apple_bootstrap_packages (
    host_uuid character varying(127) NOT NULL,
    command_uuid character varying(127) DEFAULT NULL::character varying,
    skipped boolean DEFAULT false NOT NULL,
    CONSTRAINT ck_skipped_or_commanduuid CHECK (((skipped = false) = (command_uuid IS NOT NULL)))
);


--
-- Name: host_mdm_apple_declarations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_mdm_apple_declarations (
    host_uuid character varying(255) NOT NULL,
    status character varying(20) DEFAULT NULL::character varying,
    operation_type character varying(20) DEFAULT NULL::character varying,
    detail text,
    token bytea NOT NULL,
    declaration_uuid character varying(37) DEFAULT ''::character varying NOT NULL,
    declaration_identifier character varying(255) NOT NULL,
    declaration_name character varying(255) DEFAULT ''::character varying NOT NULL,
    secrets_updated_at timestamp without time zone,
    resync boolean DEFAULT false NOT NULL,
    scope text DEFAULT 'System'::text NOT NULL,
    variables_updated_at timestamp without time zone,
    assets_updated_at timestamp(6) without time zone DEFAULT NULL::timestamp without time zone
);


--
-- Name: host_mdm_apple_device_names; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_mdm_apple_device_names (
    host_uuid character varying(255) NOT NULL,
    status character varying(20) DEFAULT NULL::character varying,
    command_uuid character varying(127) DEFAULT NULL::character varying,
    expected_device_name character varying(255) DEFAULT NULL::character varying,
    detail text,
    created_at timestamp(6) without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: host_mdm_apple_enrollment_permissions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_mdm_apple_enrollment_permissions (
    host_uuid character varying(255) NOT NULL,
    access_rights integer DEFAULT 8191 NOT NULL,
    delivered_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: host_mdm_apple_profiles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_mdm_apple_profiles (
    profile_identifier character varying(255) NOT NULL,
    host_uuid character varying(255) NOT NULL,
    status character varying(20) DEFAULT NULL::character varying,
    operation_type character varying(20) DEFAULT NULL::character varying,
    detail text,
    command_uuid character varying(127) NOT NULL,
    profile_name character varying(255) DEFAULT ''::character varying NOT NULL,
    checksum bytea NOT NULL,
    retries smallint DEFAULT '0'::smallint NOT NULL,
    profile_uuid character varying(37) DEFAULT ''::character varying NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    secrets_updated_at timestamp without time zone,
    ignore_error boolean DEFAULT false NOT NULL,
    variables_updated_at timestamp without time zone,
    scope text DEFAULT 'System'::text NOT NULL,
    has_acme_payload smallint DEFAULT 0 NOT NULL
);


--
-- Name: host_mdm_commands; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_mdm_commands (
    host_id integer NOT NULL,
    command_type character varying(31) NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: host_mdm_idp_accounts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_mdm_idp_accounts (
    id integer NOT NULL,
    host_uuid character varying(255) NOT NULL,
    account_uuid character varying(36) DEFAULT ''::character varying NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: host_mdm_idp_accounts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.host_mdm_idp_accounts ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.host_mdm_idp_accounts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: host_mdm_managed_certificates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_mdm_managed_certificates (
    host_uuid character varying(255) NOT NULL,
    profile_uuid character varying(37) NOT NULL,
    type text,
    ca_name character varying(255) DEFAULT 'NDES'::character varying NOT NULL,
    challenge_retrieved_at timestamp without time zone,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    not_valid_after timestamp without time zone,
    serial character varying(40) DEFAULT NULL::character varying,
    not_valid_before timestamp without time zone
);


--
-- Name: host_mdm_windows_profiles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_mdm_windows_profiles (
    host_uuid character varying(255) NOT NULL,
    status character varying(20) DEFAULT NULL::character varying,
    operation_type character varying(20) DEFAULT NULL::character varying,
    detail text,
    command_uuid character varying(127) NOT NULL,
    profile_name character varying(255) DEFAULT ''::character varying NOT NULL,
    retries smallint DEFAULT 0 NOT NULL,
    profile_uuid character varying(37) DEFAULT ''::character varying NOT NULL,
    checksum bytea DEFAULT '\x00000000000000000000000000000000'::bytea NOT NULL,
    secrets_updated_at timestamp without time zone,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: host_mdm_windows_profiles_status; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_mdm_windows_profiles_status (
    host_uuid character varying(255) NOT NULL,
    status character varying(20) DEFAULT ''::character varying NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: host_munki_info; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_munki_info (
    host_id integer NOT NULL,
    version character varying(255) DEFAULT ''::character varying NOT NULL,
    deleted_at timestamp without time zone
);


--
-- Name: host_munki_issues; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_munki_issues (
    host_id integer NOT NULL,
    munki_issue_id integer NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: host_operating_system; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_operating_system (
    host_id integer NOT NULL,
    os_id integer NOT NULL
);


--
-- Name: host_orbit_info; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_orbit_info (
    host_id integer NOT NULL,
    version character varying(50) NOT NULL,
    desktop_version character varying(50) DEFAULT NULL::character varying,
    scripts_enabled boolean
);


--
-- Name: host_recovery_key_passwords; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_recovery_key_passwords (
    host_uuid character varying(255) NOT NULL,
    encrypted_password bytea NOT NULL,
    status character varying(20) DEFAULT NULL::character varying,
    operation_type character varying(20) NOT NULL,
    error_message text,
    deleted boolean DEFAULT false NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    pending_encrypted_password bytea,
    pending_error_message text,
    auto_rotate_at timestamp(6) without time zone DEFAULT NULL::timestamp without time zone
);


--
-- Name: host_scd_data; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_scd_data (
    id bigint NOT NULL,
    dataset character varying(50) NOT NULL,
    entity_id character varying(100) DEFAULT ''::character varying NOT NULL,
    host_bitmap bytea NOT NULL,
    valid_from timestamp without time zone NOT NULL,
    valid_to timestamp without time zone DEFAULT '9999-12-31 00:00:00'::timestamp without time zone NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    encoding_type smallint DEFAULT 0 NOT NULL
);


--
-- Name: host_scd_data_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.host_scd_data_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: host_scd_data_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.host_scd_data_id_seq OWNED BY public.host_scd_data.id;


--
-- Name: host_scim_user; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_scim_user (
    host_id integer NOT NULL,
    scim_user_id integer NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: host_script_results; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_script_results (
    id integer NOT NULL,
    host_id integer NOT NULL,
    execution_id character varying(255) NOT NULL,
    output text NOT NULL,
    runtime integer DEFAULT 0 NOT NULL,
    exit_code integer,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    script_id integer,
    user_id integer,
    sync_request boolean DEFAULT false NOT NULL,
    script_content_id integer,
    host_deleted_at timestamp without time zone,
    timeout integer,
    policy_id integer,
    setup_experience_script_id integer,
    is_internal boolean DEFAULT false,
    canceled boolean DEFAULT false NOT NULL,
    attempt_number integer
);


--
-- Name: host_script_results_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.host_script_results ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.host_script_results_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: host_seen_times; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_seen_times (
    host_id integer NOT NULL,
    seen_time timestamp without time zone
);


--
-- Name: host_software; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_software (
    host_id integer NOT NULL,
    software_id bigint NOT NULL,
    last_opened_at timestamp without time zone
);


--
-- Name: host_software_installed_paths; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_software_installed_paths (
    id bigint NOT NULL,
    host_id integer NOT NULL,
    software_id bigint NOT NULL,
    installed_path text NOT NULL,
    team_identifier character varying(10) DEFAULT ''::character varying NOT NULL,
    cdhash_sha256 character(64) DEFAULT NULL::bpchar,
    executable_sha256 character(64) DEFAULT NULL::bpchar,
    executable_path text
);


--
-- Name: host_software_installed_paths_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.host_software_installed_paths ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.host_software_installed_paths_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: host_software_installs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_software_installs (
    id integer NOT NULL,
    execution_id character varying(255) NOT NULL,
    host_id integer NOT NULL,
    software_installer_id integer,
    pre_install_query_output text,
    install_script_output text,
    install_script_exit_code integer,
    post_install_script_output text,
    post_install_script_exit_code integer,
    user_id integer,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    self_service boolean DEFAULT false NOT NULL,
    host_deleted_at timestamp without time zone,
    removed boolean DEFAULT false NOT NULL,
    uninstall_script_output text,
    uninstall_script_exit_code integer,
    uninstall boolean DEFAULT false NOT NULL,
    status text,
    policy_id integer,
    installer_filename character varying(255) DEFAULT '[deleted installer]'::character varying NOT NULL,
    version character varying(255) DEFAULT 'unknown'::character varying NOT NULL,
    software_title_id integer,
    software_title_name character varying(255) DEFAULT '[deleted title]'::character varying NOT NULL,
    execution_status text,
    canceled boolean DEFAULT false NOT NULL,
    attempt_number integer
);


--
-- Name: host_software_installs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.host_software_installs ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.host_software_installs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: host_updates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_updates (
    host_id integer NOT NULL,
    software_updated_at timestamp without time zone
);


--
-- Name: host_users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_users (
    host_id integer NOT NULL,
    uid integer NOT NULL,
    username character varying(255) NOT NULL,
    groupname character varying(255) DEFAULT NULL::character varying,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    removed_at timestamp without time zone,
    user_type character varying(255) DEFAULT NULL::character varying,
    shell character varying(255) DEFAULT ''::character varying
);


--
-- Name: host_vpp_software_installs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_vpp_software_installs (
    id integer NOT NULL,
    host_id integer NOT NULL,
    adam_id character varying(255) NOT NULL,
    command_uuid character varying(127) NOT NULL,
    user_id integer,
    self_service boolean DEFAULT false NOT NULL,
    associated_event_id character varying(36) DEFAULT NULL::character varying,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    platform character varying(10) NOT NULL,
    removed boolean DEFAULT false NOT NULL,
    vpp_token_id integer,
    policy_id integer,
    canceled boolean DEFAULT false NOT NULL,
    verification_command_uuid character varying(127) DEFAULT NULL::character varying,
    verification_at timestamp without time zone,
    verification_failed_at timestamp without time zone,
    retry_count integer DEFAULT 0 NOT NULL
);


--
-- Name: host_vpp_software_installs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.host_vpp_software_installs ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.host_vpp_software_installs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: hosts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.hosts (
    id integer NOT NULL,
    osquery_host_id character varying(255) DEFAULT NULL::character varying,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    detail_updated_at timestamp without time zone,
    node_key character varying(255) DEFAULT NULL::character varying,
    hostname character varying(255) DEFAULT ''::character varying NOT NULL,
    uuid character varying(255) DEFAULT ''::character varying NOT NULL,
    platform character varying(255) DEFAULT ''::character varying NOT NULL,
    osquery_version character varying(255) DEFAULT ''::character varying NOT NULL,
    os_version character varying(255) DEFAULT ''::character varying NOT NULL,
    build character varying(255) DEFAULT ''::character varying NOT NULL,
    platform_like character varying(255) DEFAULT ''::character varying NOT NULL,
    code_name character varying(255) DEFAULT ''::character varying NOT NULL,
    uptime bigint DEFAULT '0'::bigint NOT NULL,
    memory bigint DEFAULT '0'::bigint NOT NULL,
    cpu_type character varying(255) DEFAULT ''::character varying NOT NULL,
    cpu_subtype character varying(255) DEFAULT ''::character varying NOT NULL,
    cpu_brand character varying(255) DEFAULT ''::character varying NOT NULL,
    cpu_physical_cores integer DEFAULT 0 NOT NULL,
    cpu_logical_cores integer DEFAULT 0 NOT NULL,
    hardware_vendor character varying(255) DEFAULT ''::character varying NOT NULL,
    hardware_model character varying(255) DEFAULT ''::character varying NOT NULL,
    hardware_version character varying(255) DEFAULT ''::character varying NOT NULL,
    hardware_serial character varying(255) DEFAULT ''::character varying NOT NULL,
    computer_name character varying(255) DEFAULT ''::character varying NOT NULL,
    primary_ip_id integer,
    distributed_interval integer DEFAULT 0,
    logger_tls_period integer DEFAULT 0,
    config_tls_refresh integer DEFAULT 0,
    primary_ip character varying(45) DEFAULT ''::character varying NOT NULL,
    primary_mac character varying(17) DEFAULT ''::character varying NOT NULL,
    label_updated_at timestamp without time zone DEFAULT '2000-01-01 00:00:00'::timestamp without time zone NOT NULL,
    last_enrolled_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    refetch_requested boolean DEFAULT false NOT NULL,
    team_id integer,
    policy_updated_at timestamp without time zone DEFAULT '2000-01-01 00:00:00'::timestamp without time zone NOT NULL,
    public_ip character varying(45) DEFAULT ''::character varying NOT NULL,
    orbit_node_key character varying(255) DEFAULT NULL::character varying,
    refetch_critical_queries_until timestamp without time zone,
    last_restarted_at timestamp without time zone DEFAULT '0001-01-01 00:00:00'::timestamp without time zone,
    timezone character varying(255) DEFAULT NULL::character varying,
    orbit_debug_until timestamp(6) without time zone DEFAULT NULL::timestamp without time zone
);


--
-- Name: hosts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.hosts ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.hosts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: identity_certificates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.identity_certificates (
    serial bigint NOT NULL,
    name character varying(1024) DEFAULT NULL::character varying,
    not_valid_before timestamp without time zone NOT NULL,
    not_valid_after timestamp without time zone NOT NULL,
    certificate_pem text NOT NULL,
    revoked boolean DEFAULT false NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT scep_certificates_chk_1 CHECK ((substr(certificate_pem, 1, 27) = '-----BEGIN CERTIFICATE-----'::text)),
    CONSTRAINT scep_certificates_chk_2 CHECK (((name IS NULL) OR ((name)::text <> ''::text)))
);


--
-- Name: identity_serials; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.identity_serials (
    serial bigint NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: in_house_app_configurations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.in_house_app_configurations (
    id integer NOT NULL,
    in_house_app_id integer NOT NULL,
    configuration text NOT NULL,
    created_at timestamp(6) without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: in_house_app_configurations_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.in_house_app_configurations ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.in_house_app_configurations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: in_house_app_install_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.in_house_app_install_tokens (
    token character varying(36) NOT NULL,
    software_title_id integer NOT NULL,
    team_id integer NOT NULL,
    host_id integer NOT NULL,
    expires_at timestamp(6) without time zone NOT NULL,
    created_at timestamp(6) without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: in_house_app_labels; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.in_house_app_labels (
    id integer NOT NULL,
    in_house_app_id integer NOT NULL,
    label_id integer NOT NULL,
    exclude boolean DEFAULT false NOT NULL,
    require_all boolean DEFAULT false NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: in_house_app_labels_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.in_house_app_labels ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.in_house_app_labels_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: in_house_app_software_categories; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.in_house_app_software_categories (
    id integer NOT NULL,
    software_category_id integer NOT NULL,
    in_house_app_id integer NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: in_house_app_software_categories_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.in_house_app_software_categories ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.in_house_app_software_categories_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: in_house_app_upcoming_activities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.in_house_app_upcoming_activities (
    upcoming_activity_id bigint NOT NULL,
    in_house_app_id integer NOT NULL,
    software_title_id integer,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: in_house_apps; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.in_house_apps (
    id integer NOT NULL,
    title_id integer,
    team_id integer,
    global_or_team_id integer DEFAULT 0 NOT NULL,
    filename character varying(255) DEFAULT ''::character varying NOT NULL,
    version character varying(255) DEFAULT ''::character varying NOT NULL,
    storage_id character varying(64) NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    platform character varying(10) NOT NULL,
    bundle_identifier character varying(255) DEFAULT ''::character varying NOT NULL,
    self_service boolean DEFAULT false NOT NULL,
    url character varying(4095) DEFAULT ''::character varying NOT NULL
);


--
-- Name: in_house_apps_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.in_house_apps ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.in_house_apps_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: invite_teams; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.invite_teams (
    invite_id integer NOT NULL,
    team_id integer NOT NULL,
    role character varying(64) NOT NULL
);


--
-- Name: invites; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.invites (
    id integer NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    invited_by integer NOT NULL,
    email character varying(255) NOT NULL,
    name character varying(255) DEFAULT NULL::character varying,
    "position" character varying(255) DEFAULT NULL::character varying,
    token character varying(255) NOT NULL,
    sso_enabled boolean DEFAULT false NOT NULL,
    global_role character varying(64) DEFAULT NULL::character varying,
    mfa_enabled boolean DEFAULT false NOT NULL
);


--
-- Name: invites_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.invites ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.invites_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: jobs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.jobs (
    id integer NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    name character varying(255) NOT NULL,
    args jsonb,
    state character varying(255) NOT NULL,
    retries integer DEFAULT 0 NOT NULL,
    error text,
    not_before timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: jobs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.jobs ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.jobs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: kernel_host_counts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.kernel_host_counts (
    id integer NOT NULL,
    software_title_id integer,
    software_id integer,
    os_version_id integer,
    hosts_count integer NOT NULL,
    team_id integer NOT NULL
);


--
-- Name: kernel_host_counts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.kernel_host_counts ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.kernel_host_counts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: label_membership; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.label_membership (
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    label_id integer NOT NULL,
    host_id integer NOT NULL
);


--
-- Name: labels; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.labels (
    id integer NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    name character varying(255) NOT NULL,
    description character varying(255) DEFAULT ''::character varying NOT NULL,
    query text NOT NULL,
    platform character varying(255) DEFAULT ''::character varying NOT NULL,
    label_type integer DEFAULT 1 NOT NULL,
    label_membership_type integer DEFAULT 0 NOT NULL,
    author_id integer,
    criteria jsonb,
    team_id integer
);


--
-- Name: labels_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.labels ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.labels_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: legacy_host_filevault_profiles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.legacy_host_filevault_profiles (
    id integer NOT NULL,
    host_uuid character varying(36) NOT NULL,
    status character varying(20) NOT NULL,
    operation_type character varying(20) NOT NULL,
    profile_uuid character varying(37) NOT NULL,
    detail text,
    command_uuid character varying(127) NOT NULL,
    scope text DEFAULT 'System'::text NOT NULL,
    created_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone NOT NULL
);


--
-- Name: legacy_host_filevault_profiles_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.legacy_host_filevault_profiles ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.legacy_host_filevault_profiles_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: legacy_host_mdm_enroll_refs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.legacy_host_mdm_enroll_refs (
    id integer NOT NULL,
    host_uuid character varying(255) NOT NULL,
    enroll_ref character varying(36) NOT NULL
);


--
-- Name: legacy_host_mdm_enroll_refs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.legacy_host_mdm_enroll_refs ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.legacy_host_mdm_enroll_refs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: legacy_host_mdm_idp_accounts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.legacy_host_mdm_idp_accounts (
    id integer NOT NULL,
    host_uuid character varying(255) NOT NULL,
    email character varying(255) NOT NULL,
    account_uuid character varying(36) DEFAULT NULL::character varying,
    host_id integer,
    email_id integer,
    email_created_at timestamp without time zone,
    email_updated_at timestamp without time zone
);


--
-- Name: legacy_host_mdm_idp_accounts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.legacy_host_mdm_idp_accounts ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.legacy_host_mdm_idp_accounts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: locks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.locks (
    id integer NOT NULL,
    name character varying(255) DEFAULT NULL::character varying,
    owner character varying(255) DEFAULT NULL::character varying,
    expires_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: locks_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.locks ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.locks_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: mdm_adue_enrollment_challenges; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mdm_adue_enrollment_challenges (
    id integer NOT NULL,
    challenge bytea NOT NULL,
    idp_account_uuid character varying(255) NOT NULL,
    abm_token_id integer,
    expires_at timestamp(6) without time zone NOT NULL,
    used_at timestamp(6) without time zone,
    created_at timestamp(6) without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: mdm_adue_enrollment_challenges_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.mdm_adue_enrollment_challenges ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.mdm_adue_enrollment_challenges_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: mdm_android_commands; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mdm_android_commands (
    command_uuid character varying(36) NOT NULL,
    host_uuid character varying(255) NOT NULL,
    operation_name character varying(255) NOT NULL,
    command_type character varying(32) NOT NULL,
    status character varying(16) DEFAULT 'pending'::character varying NOT NULL,
    error_code character varying(64) DEFAULT NULL::character varying,
    error_message character varying(1024) DEFAULT NULL::character varying,
    created_at timestamp(6) without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT mdm_android_commands_status_check CHECK (((status)::text = ANY (ARRAY[('pending'::character varying)::text, ('acknowledged'::character varying)::text, ('error'::character varying)::text])))
);


--
-- Name: mdm_android_configuration_profiles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mdm_android_configuration_profiles (
    profile_uuid character varying(37) DEFAULT ''::character varying NOT NULL,
    team_id integer DEFAULT 0 NOT NULL,
    name character varying(255) NOT NULL,
    raw_json jsonb NOT NULL,
    auto_increment bigint NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    uploaded_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    checksum bytea GENERATED ALWAYS AS (decode(md5((raw_json)::text), 'hex'::text)) STORED
);


--
-- Name: mdm_android_configuration_profiles_auto_increment_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.mdm_android_configuration_profiles ALTER COLUMN auto_increment ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.mdm_android_configuration_profiles_auto_increment_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: mdm_apple_bootstrap_packages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mdm_apple_bootstrap_packages (
    team_id integer NOT NULL,
    name character varying(255) DEFAULT NULL::character varying,
    sha256 bytea NOT NULL,
    bytes bytea,
    token character varying(36) DEFAULT NULL::character varying,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: mdm_apple_configuration_profiles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mdm_apple_configuration_profiles (
    profile_id integer NOT NULL,
    team_id integer DEFAULT 0 NOT NULL,
    identifier character varying(255) NOT NULL,
    name character varying(255) NOT NULL,
    mobileconfig bytea NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    uploaded_at timestamp without time zone,
    checksum bytea NOT NULL,
    profile_uuid character varying(37) DEFAULT ''::character varying NOT NULL,
    secrets_updated_at timestamp without time zone,
    scope text DEFAULT 'System'::text NOT NULL
);


--
-- Name: mdm_apple_configuration_profiles_profile_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.mdm_apple_configuration_profiles ALTER COLUMN profile_id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.mdm_apple_configuration_profiles_profile_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: mdm_apple_declaration_activation_references; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mdm_apple_declaration_activation_references (
    declaration_uuid character varying(37) DEFAULT ''::character varying NOT NULL,
    reference character varying(37) DEFAULT ''::character varying NOT NULL
);


--
-- Name: mdm_apple_declaration_asset_references; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mdm_apple_declaration_asset_references (
    declaration_uuid character varying(37) NOT NULL,
    asset_uuid character varying(37) NOT NULL
);


--
-- Name: mdm_apple_declaration_assets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mdm_apple_declaration_assets (
    asset_uuid character varying(37) NOT NULL,
    team_id integer NOT NULL,
    identifier character varying(255) NOT NULL,
    name character varying(255) NOT NULL,
    raw_json text NOT NULL,
    secrets_updated_at timestamp(6) without time zone DEFAULT NULL::timestamp without time zone,
    token bytea GENERATED ALWAYS AS (decode(md5((raw_json || COALESCE((EXTRACT(epoch FROM secrets_updated_at))::text, ''::text))), 'hex'::text)) STORED,
    created_at timestamp(6) without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    uploaded_at timestamp(6) without time zone DEFAULT NULL::timestamp without time zone
);


--
-- Name: mdm_apple_declarations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mdm_apple_declarations (
    declaration_uuid character varying(37) DEFAULT ''::character varying NOT NULL,
    team_id integer DEFAULT 0 NOT NULL,
    identifier character varying(255) NOT NULL,
    name character varying(255) NOT NULL,
    raw_json text NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    uploaded_at timestamp without time zone,
    auto_increment bigint NOT NULL,
    secrets_updated_at timestamp without time zone,
    token bytea,
    scope text DEFAULT 'System'::text NOT NULL
);


--
-- Name: mdm_apple_declarations_auto_increment_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.mdm_apple_declarations ALTER COLUMN auto_increment ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.mdm_apple_declarations_auto_increment_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: mdm_apple_declarative_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mdm_apple_declarative_requests (
    id integer NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    enrollment_id character varying(255) NOT NULL,
    message_type character varying(255) NOT NULL,
    raw_json text
);


--
-- Name: mdm_apple_declarative_requests_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.mdm_apple_declarative_requests ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.mdm_apple_declarative_requests_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: mdm_apple_default_setup_assistants; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mdm_apple_default_setup_assistants (
    id integer NOT NULL,
    team_id integer,
    global_or_team_id integer DEFAULT 0 NOT NULL,
    profile_uuid character varying(255) DEFAULT ''::character varying NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    abm_token_id integer
);


--
-- Name: mdm_apple_default_setup_assistants_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.mdm_apple_default_setup_assistants ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.mdm_apple_default_setup_assistants_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: mdm_apple_enrollment_profiles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mdm_apple_enrollment_profiles (
    id integer NOT NULL,
    token character varying(36) DEFAULT NULL::character varying,
    type character varying(10) DEFAULT 'automatic'::character varying NOT NULL,
    dep_profile jsonb,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: mdm_apple_enrollment_profiles_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.mdm_apple_enrollment_profiles ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.mdm_apple_enrollment_profiles_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: mdm_apple_installers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mdm_apple_installers (
    id integer NOT NULL,
    name character varying(255) DEFAULT ''::character varying NOT NULL,
    size bigint NOT NULL,
    manifest text NOT NULL,
    installer bytea,
    url_token character varying(36) DEFAULT NULL::character varying
);


--
-- Name: mdm_apple_installers_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.mdm_apple_installers ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.mdm_apple_installers_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: mdm_apple_psso_devices; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mdm_apple_psso_devices (
    host_uuid character varying(255) NOT NULL,
    created_at timestamp(6) without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: mdm_apple_psso_keys; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mdm_apple_psso_keys (
    kid character varying(255) NOT NULL,
    host_uuid character varying(255) NOT NULL,
    key_type character varying(255) NOT NULL,
    pem text NOT NULL,
    created_at timestamp(6) without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT mdm_apple_psso_keys_key_type_check CHECK (((key_type)::text = ANY (ARRAY[('signing'::character varying)::text, ('encryption'::character varying)::text])))
);


--
-- Name: mdm_apple_setup_assistant_profiles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mdm_apple_setup_assistant_profiles (
    id integer NOT NULL,
    setup_assistant_id integer NOT NULL,
    abm_token_id integer NOT NULL,
    profile_uuid character varying(255) DEFAULT ''::character varying NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: mdm_apple_setup_assistant_profiles_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.mdm_apple_setup_assistant_profiles ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.mdm_apple_setup_assistant_profiles_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: mdm_apple_setup_assistants; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mdm_apple_setup_assistants (
    id integer NOT NULL,
    team_id integer,
    global_or_team_id integer DEFAULT 0 NOT NULL,
    name text NOT NULL,
    profile jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: mdm_apple_setup_assistants_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.mdm_apple_setup_assistants ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.mdm_apple_setup_assistants_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: mdm_config_assets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mdm_config_assets (
    id integer NOT NULL,
    name character varying(256) DEFAULT ''::character varying NOT NULL,
    value bytea NOT NULL,
    deleted_at timestamp without time zone,
    deletion_uuid character varying(127) DEFAULT ''::character varying NOT NULL,
    md5_checksum bytea NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: mdm_config_assets_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.mdm_config_assets ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.mdm_config_assets_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: mdm_configuration_profile_labels; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mdm_configuration_profile_labels (
    id integer NOT NULL,
    apple_profile_uuid character varying(37) DEFAULT NULL::character varying,
    windows_profile_uuid character varying(37) DEFAULT NULL::character varying,
    label_name character varying(255) NOT NULL,
    label_id integer,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    exclude boolean DEFAULT false NOT NULL,
    require_all boolean DEFAULT false NOT NULL,
    android_profile_uuid character varying(37) DEFAULT NULL::character varying
);


--
-- Name: mdm_configuration_profile_labels_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.mdm_configuration_profile_labels ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.mdm_configuration_profile_labels_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: mdm_configuration_profile_update_settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mdm_configuration_profile_update_settings (
    id integer NOT NULL,
    windows_profile_uuid character varying(37) DEFAULT NULL::character varying,
    apple_declaration_uuid character varying(37) DEFAULT NULL::character varying,
    created_at timestamp(6) without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT ck_mdm_config_profile_update_settings_exactly_one CHECK (((
CASE
    WHEN (apple_declaration_uuid IS NULL) THEN 0
    ELSE 1
END +
CASE
    WHEN (windows_profile_uuid IS NULL) THEN 0
    ELSE 1
END) = 1))
);


--
-- Name: mdm_configuration_profile_update_settings_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.mdm_configuration_profile_update_settings ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.mdm_configuration_profile_update_settings_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: mdm_configuration_profile_variables; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mdm_configuration_profile_variables (
    id integer NOT NULL,
    apple_profile_uuid character varying(37) DEFAULT NULL::character varying,
    windows_profile_uuid character varying(37) DEFAULT NULL::character varying,
    fleet_variable_id integer NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    apple_declaration_uuid character varying(37) DEFAULT NULL::character varying,
    android_profile_uuid character varying(37) DEFAULT NULL::character varying,
    certificate_template_id integer,
    android_app_configuration_id integer,
    CONSTRAINT ck_mdm_configuration_profile_variables_apple_or_windows CHECK (((apple_profile_uuid IS NULL) <> (windows_profile_uuid IS NULL))),
    CONSTRAINT ck_mdm_configuration_profile_variables_exactly_one CHECK (((((((
CASE
    WHEN (apple_profile_uuid IS NULL) THEN 0
    ELSE 1
END +
CASE
    WHEN (windows_profile_uuid IS NULL) THEN 0
    ELSE 1
END) +
CASE
    WHEN (apple_declaration_uuid IS NULL) THEN 0
    ELSE 1
END) +
CASE
    WHEN (android_profile_uuid IS NULL) THEN 0
    ELSE 1
END) +
CASE
    WHEN (certificate_template_id IS NULL) THEN 0
    ELSE 1
END) +
CASE
    WHEN (android_app_configuration_id IS NULL) THEN 0
    ELSE 1
END) = 1))
);


--
-- Name: mdm_configuration_profile_variables_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.mdm_configuration_profile_variables ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.mdm_configuration_profile_variables_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: mdm_declaration_labels; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mdm_declaration_labels (
    id integer NOT NULL,
    apple_declaration_uuid character varying(37) DEFAULT ''::character varying NOT NULL,
    label_name character varying(255) NOT NULL,
    label_id integer,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    uploaded_at timestamp without time zone,
    exclude boolean DEFAULT false NOT NULL,
    require_all boolean DEFAULT false NOT NULL
);


--
-- Name: mdm_declaration_labels_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.mdm_declaration_labels ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.mdm_declaration_labels_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: mdm_delivery_status; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mdm_delivery_status (
    status character varying(20) NOT NULL
);


--
-- Name: mdm_idp_accounts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mdm_idp_accounts (
    uuid character varying(255) NOT NULL,
    username character varying(255) NOT NULL,
    fullname character varying(256) DEFAULT ''::character varying NOT NULL,
    email character varying(255) DEFAULT ''::character varying NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: mdm_operation_types; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mdm_operation_types (
    operation_type character varying(20) NOT NULL
);


--
-- Name: mdm_windows_configuration_profiles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mdm_windows_configuration_profiles (
    profile_uuid character varying(37) DEFAULT ''::character varying NOT NULL,
    team_id integer DEFAULT 0 NOT NULL,
    name character varying(255) NOT NULL,
    syncml bytea NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    uploaded_at timestamp without time zone,
    auto_increment bigint NOT NULL,
    checksum bytea,
    secrets_updated_at timestamp without time zone
);


--
-- Name: mdm_windows_configuration_profiles_auto_increment_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.mdm_windows_configuration_profiles ALTER COLUMN auto_increment ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.mdm_windows_configuration_profiles_auto_increment_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: mdm_windows_configuration_profiles_prior_content; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mdm_windows_configuration_profiles_prior_content (
    profile_uuid character varying(37) DEFAULT ''::character varying NOT NULL,
    checksum bytea NOT NULL,
    syncml bytea NOT NULL,
    created_at timestamp(6) without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: mdm_windows_enrollments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mdm_windows_enrollments (
    id integer NOT NULL,
    mdm_device_id character varying(255) NOT NULL,
    mdm_hardware_id character varying(255) NOT NULL,
    device_state character varying(255) NOT NULL,
    device_type character varying(255) NOT NULL,
    device_name character varying(255) NOT NULL,
    enroll_type character varying(255) NOT NULL,
    enroll_user_id character varying(255) NOT NULL,
    enroll_proto_version character varying(255) NOT NULL,
    enroll_client_version character varying(255) NOT NULL,
    not_in_oobe boolean DEFAULT false NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    host_uuid character varying(255) DEFAULT ''::character varying NOT NULL,
    credentials_hash bytea,
    credentials_acknowledged boolean DEFAULT false NOT NULL,
    awaiting_configuration smallint DEFAULT 0 NOT NULL,
    awaiting_configuration_at timestamp without time zone,
    poll_schedule_relaxed smallint DEFAULT 0 NOT NULL,
    has_pending_commands smallint DEFAULT 0 NOT NULL,
    fleetd_sync_capable smallint DEFAULT 0 NOT NULL
);


--
-- Name: mdm_windows_enrollments_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.mdm_windows_enrollments ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.mdm_windows_enrollments_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: microsoft_compliance_partner_host_statuses; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.microsoft_compliance_partner_host_statuses (
    host_id integer NOT NULL,
    device_id character varying(64) NOT NULL,
    user_principal_name character varying(255) NOT NULL,
    managed boolean,
    compliant boolean,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: microsoft_compliance_partner_integrations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.microsoft_compliance_partner_integrations (
    id integer NOT NULL,
    tenant_id character varying(64) NOT NULL,
    proxy_server_secret character varying(64) NOT NULL,
    setup_done boolean DEFAULT false NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: microsoft_compliance_partner_integrations_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.microsoft_compliance_partner_integrations ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.microsoft_compliance_partner_integrations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: migration_status_data; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.migration_status_data (
    id integer NOT NULL,
    version_id bigint NOT NULL,
    is_applied boolean NOT NULL,
    tstamp timestamp without time zone DEFAULT now()
);


--
-- Name: migration_status_data_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.migration_status_data_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: migration_status_data_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.migration_status_data_id_seq OWNED BY public.migration_status_data.id;


--
-- Name: migration_status_tables; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.migration_status_tables (
    id bigint NOT NULL,
    version_id bigint NOT NULL,
    is_applied boolean NOT NULL,
    tstamp timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: migration_status_tables_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.migration_status_tables ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.migration_status_tables_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: mobile_device_management_solutions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mobile_device_management_solutions (
    id integer NOT NULL,
    name character varying(100) NOT NULL,
    server_url character varying(255) NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: mobile_device_management_solutions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.mobile_device_management_solutions ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.mobile_device_management_solutions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: munki_issues; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.munki_issues (
    id integer NOT NULL,
    name character varying(255) NOT NULL,
    issue_type character varying(10) NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: munki_issues_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.munki_issues ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.munki_issues_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: nano_cert_auth_associations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.nano_cert_auth_associations (
    id character varying(255) NOT NULL,
    sha256 character(64) NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    cert_not_valid_after timestamp without time zone,
    renew_command_uuid character varying(127) DEFAULT NULL::character varying,
    CONSTRAINT nano_cert_auth_associations_chk_1 CHECK (((id)::text <> ''::text)),
    CONSTRAINT nano_cert_auth_associations_chk_2 CHECK ((sha256 <> ''::bpchar))
);


--
-- Name: nano_command_results; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.nano_command_results (
    id character varying(255) NOT NULL,
    command_uuid character varying(127) NOT NULL,
    status character varying(31) NOT NULL,
    result text NOT NULL,
    not_now_at timestamp without time zone,
    not_now_tally integer DEFAULT 0 NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT nano_command_results_chk_1 CHECK (((status)::text <> ''::text))
);


--
-- Name: nano_commands; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.nano_commands (
    command_uuid character varying(127) NOT NULL,
    request_type character varying(63) NOT NULL,
    command text NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    subtype text DEFAULT 'None'::text NOT NULL,
    name character varying(255) DEFAULT NULL::character varying,
    CONSTRAINT nano_commands_chk_1 CHECK (((command_uuid)::text <> ''::text)),
    CONSTRAINT nano_commands_chk_2 CHECK (((request_type)::text <> ''::text))
);


--
-- Name: nano_dep_names; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.nano_dep_names (
    name character varying(255) NOT NULL,
    consumer_key text,
    consumer_secret text,
    access_token text,
    access_secret text,
    access_token_expiry timestamp without time zone,
    config_base_url character varying(255) DEFAULT NULL::character varying,
    tokenpki_cert_pem text,
    tokenpki_key_pem text,
    syncer_cursor character varying(1024) DEFAULT NULL::character varying,
    syncer_cursor_at timestamp without time zone,
    assigner_profile_uuid text,
    assigner_profile_uuid_at timestamp without time zone,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT nano_dep_names_chk_1 CHECK (((tokenpki_cert_pem IS NULL) OR (substr(tokenpki_cert_pem, 1, 27) = '-----BEGIN CERTIFICATE-----'::text))),
    CONSTRAINT nano_dep_names_chk_2 CHECK (((tokenpki_key_pem IS NULL) OR (substr(tokenpki_key_pem, 1, 5) = '-----'::text)))
);


--
-- Name: nano_devices; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.nano_devices (
    id character varying(255) NOT NULL,
    identity_cert text,
    serial_number character varying(127) DEFAULT NULL::character varying,
    unlock_token bytea,
    unlock_token_at timestamp without time zone,
    authenticate text NOT NULL,
    authenticate_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    token_update text,
    token_update_at timestamp without time zone,
    bootstrap_token_b64 text,
    bootstrap_token_at timestamp without time zone,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    platform character varying(255) DEFAULT ''::character varying NOT NULL,
    enroll_team_id integer,
    CONSTRAINT nano_devices_chk_1 CHECK (((identity_cert IS NULL) OR (substr(identity_cert, 1, 27) = '-----BEGIN CERTIFICATE-----'::text))),
    CONSTRAINT nano_devices_chk_2 CHECK (((serial_number IS NULL) OR ((serial_number)::text <> ''::text))),
    CONSTRAINT nano_devices_chk_3 CHECK (((unlock_token IS NULL) OR (length(unlock_token) > 0))),
    CONSTRAINT nano_devices_chk_4 CHECK ((authenticate <> ''::text)),
    CONSTRAINT nano_devices_chk_5 CHECK (((token_update IS NULL) OR (token_update <> ''::text))),
    CONSTRAINT nano_devices_chk_6 CHECK (((bootstrap_token_b64 IS NULL) OR (bootstrap_token_b64 <> ''::text)))
);


--
-- Name: nano_enrollment_queue; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.nano_enrollment_queue (
    id character varying(255) NOT NULL,
    command_uuid character varying(127) NOT NULL,
    active boolean DEFAULT true NOT NULL,
    priority smallint DEFAULT '0'::smallint NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: nano_enrollments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.nano_enrollments (
    id character varying(255) NOT NULL,
    device_id character varying(255) NOT NULL,
    user_id character varying(255) DEFAULT NULL::character varying,
    type character varying(31) NOT NULL,
    topic character varying(255) NOT NULL,
    push_magic character varying(127) NOT NULL,
    token_hex character varying(255) NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    token_update_tally integer DEFAULT 1 NOT NULL,
    last_seen_at timestamp without time zone NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    enrolled_from_migration smallint DEFAULT '0'::smallint NOT NULL,
    hardware_attested boolean DEFAULT false NOT NULL,
    CONSTRAINT nano_enrollments_chk_1 CHECK (((id)::text <> ''::text)),
    CONSTRAINT nano_enrollments_chk_2 CHECK (((type)::text <> ''::text)),
    CONSTRAINT nano_enrollments_chk_3 CHECK (((topic)::text <> ''::text)),
    CONSTRAINT nano_enrollments_chk_4 CHECK (((push_magic)::text <> ''::text)),
    CONSTRAINT nano_enrollments_chk_5 CHECK (((token_hex)::text <> ''::text))
);


--
-- Name: nano_push_certs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.nano_push_certs (
    topic character varying(255) NOT NULL,
    cert_pem text NOT NULL,
    key_pem text NOT NULL,
    stale_token integer NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT nano_push_certs_chk_1 CHECK (((topic)::text <> ''::text)),
    CONSTRAINT nano_push_certs_chk_2 CHECK ((substr(cert_pem, 1, 27) = '-----BEGIN CERTIFICATE-----'::text)),
    CONSTRAINT nano_push_certs_chk_3 CHECK ((substr(key_pem, 1, 5) = '-----'::text))
);


--
-- Name: nano_users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.nano_users (
    id character varying(255) NOT NULL,
    device_id character varying(255) NOT NULL,
    user_short_name character varying(255) DEFAULT NULL::character varying,
    user_long_name character varying(255) DEFAULT NULL::character varying,
    token_update text,
    token_update_at timestamp without time zone,
    user_authenticate text,
    user_authenticate_at timestamp without time zone,
    user_authenticate_digest text,
    user_authenticate_digest_at timestamp without time zone,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT nano_users_chk_1 CHECK (((user_short_name IS NULL) OR ((user_short_name)::text <> ''::text))),
    CONSTRAINT nano_users_chk_2 CHECK (((user_long_name IS NULL) OR ((user_long_name)::text <> ''::text))),
    CONSTRAINT nano_users_chk_3 CHECK (((token_update IS NULL) OR (token_update <> ''::text))),
    CONSTRAINT nano_users_chk_4 CHECK (((user_authenticate IS NULL) OR (user_authenticate <> ''::text))),
    CONSTRAINT nano_users_chk_5 CHECK (((user_authenticate_digest IS NULL) OR (user_authenticate_digest <> ''::text)))
);


--
-- Name: nano_view_queue; Type: VIEW; Schema: public; Owner: -
--

CREATE VIEW public.nano_view_queue AS
 SELECT q.id,
    q.created_at,
    q.active,
    q.priority,
    c.command_uuid,
    c.request_type,
    c.command,
    c.name,
    r.updated_at AS result_updated_at,
    r.status,
    r.result
   FROM ((public.nano_enrollment_queue q
     JOIN public.nano_commands c ON (((q.command_uuid)::text = (c.command_uuid)::text)))
     LEFT JOIN public.nano_command_results r ON ((((r.command_uuid)::text = (q.command_uuid)::text) AND ((r.id)::text = (q.id)::text))))
  ORDER BY q.priority DESC, q.created_at;


--
-- Name: network_interfaces; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.network_interfaces (
    id integer NOT NULL,
    host_id integer NOT NULL,
    mac character varying(255) DEFAULT ''::character varying NOT NULL,
    ip_address character varying(255) DEFAULT ''::character varying NOT NULL,
    broadcast character varying(255) DEFAULT ''::character varying NOT NULL,
    ibytes bigint DEFAULT '0'::bigint NOT NULL,
    interface character varying(255) DEFAULT ''::character varying NOT NULL,
    ipackets bigint DEFAULT '0'::bigint NOT NULL,
    last_change bigint DEFAULT '0'::bigint NOT NULL,
    mask character varying(255) DEFAULT ''::character varying NOT NULL,
    metric integer DEFAULT 0 NOT NULL,
    mtu integer DEFAULT 0 NOT NULL,
    obytes bigint DEFAULT '0'::bigint NOT NULL,
    ierrors bigint DEFAULT '0'::bigint NOT NULL,
    oerrors bigint DEFAULT '0'::bigint NOT NULL,
    opackets bigint DEFAULT '0'::bigint NOT NULL,
    point_to_point character varying(255) DEFAULT ''::character varying NOT NULL,
    type integer DEFAULT 0 NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: network_interfaces_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.network_interfaces ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.network_interfaces_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: operating_system_version_vulnerabilities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.operating_system_version_vulnerabilities (
    id bigint NOT NULL,
    os_version_id integer NOT NULL,
    cve character varying(255) NOT NULL,
    team_id integer,
    source smallint DEFAULT 0,
    resolved_in_version character varying(255) DEFAULT NULL::character varying,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: operating_system_version_vulnerabilities_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.operating_system_version_vulnerabilities ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.operating_system_version_vulnerabilities_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: operating_system_vulnerabilities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.operating_system_vulnerabilities (
    id integer NOT NULL,
    operating_system_id integer NOT NULL,
    cve character varying(255) NOT NULL,
    source smallint DEFAULT '0'::smallint,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    resolved_in_version character varying(255) DEFAULT NULL::character varying,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: operating_system_vulnerabilities_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.operating_system_vulnerabilities ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.operating_system_vulnerabilities_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: operating_systems; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.operating_systems (
    id integer NOT NULL,
    name character varying(255) NOT NULL,
    version character varying(150) NOT NULL,
    arch character varying(150) NOT NULL,
    kernel_version character varying(150) NOT NULL,
    platform character varying(50) NOT NULL,
    display_version character varying(10) DEFAULT ''::character varying NOT NULL,
    os_version_id integer,
    installation_type character varying(20) DEFAULT ''::character varying NOT NULL
);


--
-- Name: operating_systems_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.operating_systems ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.operating_systems_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: org_logo; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.org_logo (
    mode character varying(10) NOT NULL,
    data bytea NOT NULL,
    uploaded_at timestamp(6) without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: osquery_options; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.osquery_options (
    id integer NOT NULL,
    override_type integer NOT NULL,
    override_identifier character varying(255) DEFAULT ''::character varying NOT NULL,
    options jsonb NOT NULL
);


--
-- Name: osquery_options_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.osquery_options ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.osquery_options_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: pack_targets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pack_targets (
    id integer NOT NULL,
    pack_id integer,
    type integer,
    target_id integer NOT NULL
);


--
-- Name: pack_targets_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.pack_targets ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.pack_targets_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: packs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.packs (
    id integer NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    disabled boolean DEFAULT false NOT NULL,
    name character varying(255) NOT NULL,
    description character varying(255) DEFAULT ''::character varying NOT NULL,
    platform character varying(255) DEFAULT ''::character varying NOT NULL,
    pack_type character varying(255) DEFAULT NULL::character varying
);


--
-- Name: packs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.packs ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.packs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: password_reset_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.password_reset_requests (
    id integer NOT NULL,
    expires_at timestamp without time zone NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    user_id integer NOT NULL,
    token character varying(1024) NOT NULL
);


--
-- Name: password_reset_requests_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.password_reset_requests ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.password_reset_requests_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: policies; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.policies (
    id integer NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    team_id integer,
    resolution text,
    name character varying(255) NOT NULL,
    query text NOT NULL,
    description text NOT NULL,
    author_id integer,
    platforms character varying(255) DEFAULT ''::character varying NOT NULL,
    critical boolean DEFAULT false NOT NULL,
    checksum bytea NOT NULL,
    calendar_events_enabled boolean DEFAULT false NOT NULL,
    software_installer_id integer,
    script_id integer,
    vpp_apps_teams_id integer,
    conditional_access_enabled boolean DEFAULT false NOT NULL,
    type character varying(255) DEFAULT 'dynamic'::character varying NOT NULL,
    patch_software_title_id integer,
    needs_full_membership_cleanup boolean DEFAULT false NOT NULL,
    continuous_automations_enabled smallint DEFAULT 0 NOT NULL
);


--
-- Name: policies_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.policies ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.policies_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: policy_automation_iterations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.policy_automation_iterations (
    policy_id integer NOT NULL,
    iteration integer NOT NULL
);


--
-- Name: policy_labels; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.policy_labels (
    id integer NOT NULL,
    policy_id integer NOT NULL,
    label_id integer NOT NULL,
    exclude boolean DEFAULT false NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    require_all boolean DEFAULT false NOT NULL
);


--
-- Name: policy_labels_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.policy_labels ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.policy_labels_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: policy_membership; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.policy_membership (
    policy_id integer NOT NULL,
    host_id integer NOT NULL,
    passes boolean,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    automation_iteration integer
);


--
-- Name: policy_stats; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.policy_stats (
    id integer NOT NULL,
    policy_id integer NOT NULL,
    inherited_team_id integer,
    passing_host_count integer DEFAULT 0 NOT NULL,
    failing_host_count integer DEFAULT 0 NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    inherited_team_id_char text GENERATED ALWAYS AS (
CASE
    WHEN (inherited_team_id IS NULL) THEN 'global'::text
    ELSE (inherited_team_id)::text
END) STORED
);


--
-- Name: policy_stats_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.policy_stats ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.policy_stats_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: queries; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.queries (
    id integer NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    saved boolean DEFAULT false NOT NULL,
    name character varying(255) NOT NULL,
    description text NOT NULL,
    query text NOT NULL,
    author_id integer,
    observer_can_run boolean DEFAULT false NOT NULL,
    team_id integer,
    team_id_char character(10) DEFAULT ''::bpchar NOT NULL,
    platform character varying(255) DEFAULT ''::character varying NOT NULL,
    min_osquery_version character varying(255) DEFAULT ''::character varying NOT NULL,
    schedule_interval integer DEFAULT 0 NOT NULL,
    automations_enabled boolean DEFAULT false NOT NULL,
    logging_type character varying(255) DEFAULT 'snapshot'::character varying NOT NULL,
    discard_data boolean DEFAULT true NOT NULL,
    is_scheduled boolean GENERATED ALWAYS AS ((schedule_interval > 0)) STORED
);


--
-- Name: queries_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.queries ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.queries_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: query_labels; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.query_labels (
    id integer NOT NULL,
    query_id integer NOT NULL,
    label_id integer NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    require_all boolean DEFAULT false NOT NULL
);


--
-- Name: query_labels_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.query_labels ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.query_labels_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: query_results; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.query_results (
    id integer NOT NULL,
    query_id integer NOT NULL,
    host_id integer NOT NULL,
    osquery_version character varying(50) DEFAULT NULL::character varying,
    error text,
    last_fetched timestamp without time zone NOT NULL,
    data jsonb,
    has_data boolean GENERATED ALWAYS AS ((data IS NOT NULL)) STORED
);


--
-- Name: query_results_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.query_results ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.query_results_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: scep_serials_serial_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.identity_serials ALTER COLUMN serial ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.scep_serials_serial_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: scheduled_queries; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.scheduled_queries (
    id integer NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    pack_id integer,
    query_id integer,
    "interval" integer,
    snapshot boolean,
    removed boolean,
    platform character varying(255) DEFAULT ''::character varying,
    version character varying(255) DEFAULT ''::character varying,
    shard integer,
    query_name character varying(255) NOT NULL,
    name character varying(255) NOT NULL,
    description character varying(1023) DEFAULT ''::character varying,
    denylist boolean,
    team_id_char character(10) DEFAULT ''::bpchar NOT NULL
);


--
-- Name: scheduled_queries_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.scheduled_queries ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.scheduled_queries_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: scheduled_query_stats; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.scheduled_query_stats (
    host_id integer NOT NULL,
    scheduled_query_id integer NOT NULL,
    average_memory bigint DEFAULT 0 NOT NULL,
    denylisted boolean,
    executions bigint DEFAULT 0 NOT NULL,
    schedule_interval integer,
    last_executed timestamp without time zone,
    output_size bigint DEFAULT 0 NOT NULL,
    system_time bigint DEFAULT 0 NOT NULL,
    user_time bigint DEFAULT 0 NOT NULL,
    wall_time bigint DEFAULT 0 NOT NULL,
    query_type smallint DEFAULT '0'::smallint NOT NULL
);


--
-- Name: scim_groups; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.scim_groups (
    id integer NOT NULL,
    external_id character varying(255) DEFAULT NULL::character varying,
    display_name character varying(255) NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: scim_groups_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.scim_groups ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.scim_groups_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: scim_last_request; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.scim_last_request (
    id smallint DEFAULT '1'::smallint NOT NULL,
    status character varying(31) NOT NULL,
    details character varying(255) NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: scim_user_emails; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.scim_user_emails (
    id bigint NOT NULL,
    scim_user_id integer NOT NULL,
    email character varying(255) NOT NULL,
    "primary" boolean,
    type character varying(31) DEFAULT NULL::character varying,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: scim_user_emails_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.scim_user_emails ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.scim_user_emails_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: scim_user_group; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.scim_user_group (
    scim_user_id integer NOT NULL,
    group_id integer NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: scim_users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.scim_users (
    id integer NOT NULL,
    external_id character varying(255) DEFAULT NULL::character varying,
    user_name character varying(255) NOT NULL,
    given_name character varying(255) DEFAULT NULL::character varying,
    family_name character varying(255) DEFAULT NULL::character varying,
    active boolean,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    department character varying(255) DEFAULT NULL::character varying
);


--
-- Name: scim_users_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.scim_users ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.scim_users_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: script_contents; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.script_contents (
    id integer NOT NULL,
    md5_checksum bytea NOT NULL,
    contents text NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: script_contents_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.script_contents ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.script_contents_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: script_upcoming_activities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.script_upcoming_activities (
    upcoming_activity_id bigint NOT NULL,
    script_id integer,
    script_content_id integer,
    policy_id integer,
    setup_experience_script_id integer,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: scripts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.scripts (
    id integer NOT NULL,
    team_id integer,
    global_or_team_id integer DEFAULT 0 NOT NULL,
    name character varying(255) NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    script_content_id integer
);


--
-- Name: scripts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.scripts ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.scripts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: secret_variables; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.secret_variables (
    id integer NOT NULL,
    name character varying(255) NOT NULL,
    value bytea NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: secret_variables_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.secret_variables ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.secret_variables_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sessions (
    id integer NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    accessed_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    user_id integer NOT NULL,
    key character varying(255) NOT NULL
);


--
-- Name: sessions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.sessions ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.sessions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: setup_experience_scripts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.setup_experience_scripts (
    id integer NOT NULL,
    team_id integer,
    global_or_team_id integer DEFAULT 0 NOT NULL,
    name character varying(255) NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    script_content_id integer
);


--
-- Name: setup_experience_scripts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.setup_experience_scripts ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.setup_experience_scripts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: setup_experience_software_installers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.setup_experience_software_installers (
    software_installer_id integer NOT NULL,
    platform character varying(32) NOT NULL,
    global_or_team_id integer NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: setup_experience_status_results; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.setup_experience_status_results (
    id integer NOT NULL,
    host_uuid character varying(255) NOT NULL,
    name character varying(255) NOT NULL,
    status text NOT NULL,
    software_installer_id integer,
    host_software_installs_execution_id character varying(255) DEFAULT NULL::character varying,
    vpp_app_team_id integer,
    nano_command_uuid character varying(255) DEFAULT NULL::character varying,
    setup_experience_script_id integer,
    script_execution_id character varying(255) DEFAULT NULL::character varying,
    error character varying(255) DEFAULT NULL::character varying,
    policy_gated smallint DEFAULT 0 NOT NULL
);


--
-- Name: setup_experience_status_results_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.setup_experience_status_results ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.setup_experience_status_results_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: software; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.software (
    id bigint NOT NULL,
    name character varying(255) NOT NULL,
    version character varying(255) DEFAULT ''::character varying NOT NULL,
    source character varying(64) NOT NULL,
    bundle_identifier character varying(255) DEFAULT ''::character varying,
    release character varying(64) DEFAULT ''::character varying NOT NULL,
    vendor_old character varying(32) DEFAULT ''::character varying NOT NULL,
    arch character varying(16) DEFAULT ''::character varying NOT NULL,
    vendor character varying(114) DEFAULT ''::character varying NOT NULL,
    extension_for character varying(255) DEFAULT ''::character varying NOT NULL,
    extension_id character varying(255) DEFAULT ''::character varying NOT NULL,
    title_id integer,
    checksum bytea NOT NULL,
    name_source text DEFAULT 'basic'::text NOT NULL,
    application_id character varying(255) DEFAULT NULL::character varying,
    upgrade_code character(38) DEFAULT NULL::bpchar
);


--
-- Name: software_categories; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.software_categories (
    id integer NOT NULL,
    name character varying(255) NOT NULL,
    team_id integer DEFAULT 0 NOT NULL,
    created_at timestamp(6) without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: software_categories_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.software_categories ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.software_categories_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: software_cpe; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.software_cpe (
    id integer NOT NULL,
    software_id bigint,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    cpe character varying(255) NOT NULL
);


--
-- Name: software_cpe_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.software_cpe ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.software_cpe_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: software_cve; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.software_cve (
    id integer NOT NULL,
    cve character varying(255) NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    source integer DEFAULT 0,
    software_id bigint,
    resolved_in_version character varying(255) DEFAULT NULL::character varying
);


--
-- Name: software_cve_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.software_cve ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.software_cve_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: software_host_counts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.software_host_counts (
    software_id bigint NOT NULL,
    hosts_count integer NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    team_id integer DEFAULT 0 NOT NULL,
    global_stats boolean DEFAULT false NOT NULL
);


--
-- Name: software_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.software ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.software_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: software_install_upcoming_activities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.software_install_upcoming_activities (
    upcoming_activity_id bigint NOT NULL,
    software_installer_id integer,
    policy_id integer,
    software_title_id integer,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: software_installer_labels; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.software_installer_labels (
    id integer NOT NULL,
    software_installer_id integer NOT NULL,
    label_id integer NOT NULL,
    exclude boolean DEFAULT false NOT NULL,
    require_all boolean DEFAULT false NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: software_installer_labels_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.software_installer_labels ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.software_installer_labels_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: software_installer_software_categories; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.software_installer_software_categories (
    id integer NOT NULL,
    software_category_id integer NOT NULL,
    software_installer_id integer NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: software_installer_software_categories_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.software_installer_software_categories ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.software_installer_software_categories_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: software_installers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.software_installers (
    id integer NOT NULL,
    team_id integer,
    global_or_team_id integer DEFAULT 0 NOT NULL,
    title_id integer,
    filename character varying(255) NOT NULL,
    version character varying(255) NOT NULL,
    platform character varying(255) NOT NULL,
    pre_install_query text,
    install_script_content_id integer NOT NULL,
    post_install_script_content_id integer,
    storage_id character varying(64) NOT NULL,
    uploaded_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    self_service boolean DEFAULT false NOT NULL,
    user_id integer,
    user_name character varying(255) DEFAULT ''::character varying NOT NULL,
    user_email character varying(255) DEFAULT ''::character varying NOT NULL,
    url character varying(4095) DEFAULT ''::character varying NOT NULL,
    package_ids text NOT NULL,
    extension character varying(32) DEFAULT ''::character varying NOT NULL,
    uninstall_script_content_id integer NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    fleet_maintained_app_id integer,
    install_during_setup boolean DEFAULT false NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    upgrade_code character varying(48) DEFAULT ''::character varying NOT NULL,
    patch_query text DEFAULT ''::text NOT NULL,
    http_etag character varying(512) DEFAULT NULL::character varying,
    dedup_token character varying(255) GENERATED ALWAYS AS (
CASE
    WHEN (fleet_maintained_app_id IS NULL) THEN storage_id
    ELSE version
END) STORED
);


--
-- Name: software_installers_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.software_installers ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.software_installers_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: software_title_display_names; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.software_title_display_names (
    id integer NOT NULL,
    team_id integer NOT NULL,
    software_title_id integer NOT NULL,
    display_name character varying(255) DEFAULT ''::character varying NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: software_title_display_names_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.software_title_display_names ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.software_title_display_names_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: software_title_icons; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.software_title_icons (
    id integer NOT NULL,
    team_id integer NOT NULL,
    software_title_id integer NOT NULL,
    storage_id character varying(64) NOT NULL,
    filename character varying(255) NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: software_title_icons_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.software_title_icons ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.software_title_icons_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: software_title_team_pins; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.software_title_team_pins (
    team_id integer NOT NULL,
    title_id integer NOT NULL,
    pinned_version character varying(255) NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: software_titles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.software_titles (
    id integer NOT NULL,
    name character varying(255) NOT NULL,
    source character varying(64) NOT NULL,
    extension_for character varying(255) DEFAULT ''::character varying NOT NULL,
    bundle_identifier character varying(255) DEFAULT NULL::character varying,
    additional_identifier text,
    is_kernel boolean DEFAULT false NOT NULL,
    application_id character varying(255) DEFAULT NULL::character varying,
    unique_identifier text,
    upgrade_code character(38) DEFAULT NULL::bpchar
);


--
-- Name: software_titles_host_counts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.software_titles_host_counts (
    software_title_id integer NOT NULL,
    hosts_count integer NOT NULL,
    team_id integer DEFAULT 0 NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    global_stats boolean DEFAULT false NOT NULL
);


--
-- Name: software_titles_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.software_titles ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.software_titles_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: software_update_schedules; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.software_update_schedules (
    id integer NOT NULL,
    team_id integer NOT NULL,
    title_id integer NOT NULL,
    enabled boolean DEFAULT false NOT NULL,
    start_time character(5) NOT NULL,
    end_time character(5) NOT NULL
);


--
-- Name: software_update_schedules_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.software_update_schedules ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.software_update_schedules_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: statistics; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.statistics (
    id integer NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    anonymous_identifier character varying(255) NOT NULL
);


--
-- Name: statistics_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.statistics ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.statistics_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: teams; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.teams (
    id integer NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    name character varying(255) NOT NULL,
    description character varying(1023) DEFAULT ''::character varying NOT NULL,
    config jsonb,
    name_bin text,
    filename character varying(255) DEFAULT NULL::character varying
);


--
-- Name: teams_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.teams ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.teams_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: trace_sampler_settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.trace_sampler_settings (
    id smallint NOT NULL,
    high_volume_ratio double precision DEFAULT 0.001 NOT NULL,
    standard_ratio double precision DEFAULT 0.02 NOT NULL,
    force_full smallint DEFAULT 0 NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT ck_trace_sampler_settings_high_range CHECK (((high_volume_ratio >= (0)::double precision) AND (high_volume_ratio <= (1)::double precision))),
    CONSTRAINT ck_trace_sampler_settings_singleton CHECK ((id = 1)),
    CONSTRAINT ck_trace_sampler_settings_std_range CHECK (((standard_ratio >= (0)::double precision) AND (standard_ratio <= (1)::double precision)))
);


--
-- Name: upcoming_activities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.upcoming_activities (
    id bigint NOT NULL,
    host_id integer NOT NULL,
    priority integer DEFAULT 0 NOT NULL,
    user_id integer,
    fleet_initiated boolean DEFAULT false NOT NULL,
    activity_type text NOT NULL,
    execution_id character varying(255) NOT NULL,
    payload jsonb NOT NULL,
    activated_at timestamp without time zone,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: upcoming_activities_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.upcoming_activities ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.upcoming_activities_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: user_api_endpoints; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_api_endpoints (
    user_id integer NOT NULL,
    path character varying(255) NOT NULL,
    method character varying(10) NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: user_teams; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_teams (
    user_id integer NOT NULL,
    team_id integer NOT NULL,
    role character varying(64) NOT NULL
);


--
-- Name: users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.users (
    id integer NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    password bytea NOT NULL,
    salt character varying(255) NOT NULL,
    name character varying(255) DEFAULT ''::character varying NOT NULL,
    email character varying(255) NOT NULL,
    admin_forced_password_reset boolean DEFAULT false NOT NULL,
    gravatar_url character varying(255) DEFAULT ''::character varying NOT NULL,
    "position" character varying(255) DEFAULT ''::character varying NOT NULL,
    sso_enabled boolean DEFAULT false NOT NULL,
    global_role character varying(64) DEFAULT NULL::character varying,
    api_only boolean DEFAULT false NOT NULL,
    mfa_enabled boolean DEFAULT false NOT NULL,
    settings jsonb DEFAULT '{}'::jsonb NOT NULL,
    invite_id integer
);


--
-- Name: users_deleted; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.users_deleted (
    id integer NOT NULL,
    name character varying(255) DEFAULT ''::character varying NOT NULL,
    email character varying(255) NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: users_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.users ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.users_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: verification_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.verification_tokens (
    id integer NOT NULL,
    user_id integer NOT NULL,
    token character varying(255) NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: verification_tokens_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.verification_tokens ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.verification_tokens_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: vpp_app_configurations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.vpp_app_configurations (
    id integer NOT NULL,
    application_id character varying(255) NOT NULL,
    team_id integer NOT NULL,
    platform character varying(10) NOT NULL,
    configuration text NOT NULL,
    created_at timestamp(6) without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp(6) without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: vpp_app_configurations_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.vpp_app_configurations ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.vpp_app_configurations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: vpp_app_team_labels; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.vpp_app_team_labels (
    id integer NOT NULL,
    vpp_app_team_id integer NOT NULL,
    label_id integer NOT NULL,
    exclude boolean DEFAULT false NOT NULL,
    require_all boolean DEFAULT false NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: vpp_app_team_labels_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.vpp_app_team_labels ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.vpp_app_team_labels_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: vpp_app_team_software_categories; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.vpp_app_team_software_categories (
    id integer NOT NULL,
    software_category_id integer NOT NULL,
    vpp_app_team_id integer NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: vpp_app_team_software_categories_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.vpp_app_team_software_categories ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.vpp_app_team_software_categories_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: vpp_app_upcoming_activities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.vpp_app_upcoming_activities (
    upcoming_activity_id bigint NOT NULL,
    adam_id character varying(255) NOT NULL,
    platform character varying(10) NOT NULL,
    vpp_token_id integer,
    policy_id integer,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: vpp_apps; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.vpp_apps (
    adam_id character varying(255) NOT NULL,
    title_id integer,
    bundle_identifier character varying(255) DEFAULT ''::character varying NOT NULL,
    icon_url character varying(255) DEFAULT ''::character varying NOT NULL,
    name character varying(255) DEFAULT ''::character varying NOT NULL,
    latest_version character varying(255) DEFAULT ''::character varying NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    platform character varying(10) NOT NULL,
    country_code character varying(4) DEFAULT NULL::character varying
);


--
-- Name: vpp_apps_teams; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.vpp_apps_teams (
    id integer NOT NULL,
    adam_id character varying(255) NOT NULL,
    team_id integer,
    global_or_team_id integer DEFAULT 0 NOT NULL,
    platform character varying(10) NOT NULL,
    self_service boolean DEFAULT false NOT NULL,
    vpp_token_id integer,
    install_during_setup boolean DEFAULT false NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: vpp_apps_teams_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.vpp_apps_teams ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.vpp_apps_teams_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: vpp_client_users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.vpp_client_users (
    id integer NOT NULL,
    vpp_token_id integer NOT NULL,
    managed_apple_id character varying(255) NOT NULL,
    client_user_id character varying(36) NOT NULL,
    apple_user_id character varying(255) DEFAULT NULL::character varying,
    status character varying(255) DEFAULT 'pending'::character varying NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT vpp_client_users_status_check CHECK (((status)::text = ANY (ARRAY[('pending'::character varying)::text, ('registered'::character varying)::text, ('retired'::character varying)::text])))
);


--
-- Name: vpp_client_users_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.vpp_client_users ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.vpp_client_users_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: vpp_token_teams; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.vpp_token_teams (
    id integer NOT NULL,
    vpp_token_id integer NOT NULL,
    team_id integer,
    null_team_type text DEFAULT 'none'::text
);


--
-- Name: vpp_token_teams_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.vpp_token_teams ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.vpp_token_teams_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: vpp_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.vpp_tokens (
    id integer NOT NULL,
    organization_name character varying(255) NOT NULL,
    location character varying(255) NOT NULL,
    renew_at timestamp without time zone NOT NULL,
    token bytea NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    country_code character varying(4) DEFAULT NULL::character varying
);


--
-- Name: vpp_tokens_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.vpp_tokens ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.vpp_tokens_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: vulnerability_host_counts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.vulnerability_host_counts (
    cve character varying(20) NOT NULL,
    team_id integer DEFAULT 0 NOT NULL,
    host_count integer DEFAULT 0 NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    global_stats boolean DEFAULT false NOT NULL
);


--
-- Name: windows_mdm_command_queue; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.windows_mdm_command_queue (
    enrollment_id integer NOT NULL,
    command_uuid character varying(127) NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    acked_at timestamp(6) without time zone DEFAULT NULL::timestamp without time zone
);


--
-- Name: windows_mdm_command_results; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.windows_mdm_command_results (
    enrollment_id integer NOT NULL,
    command_uuid character varying(127) NOT NULL,
    raw_result text NOT NULL,
    response_id integer NOT NULL,
    status_code character varying(31) NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: windows_mdm_commands; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.windows_mdm_commands (
    command_uuid character varying(127) NOT NULL,
    raw_command text NOT NULL,
    target_loc_uri character varying(255) NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: windows_mdm_responses; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.windows_mdm_responses (
    id integer NOT NULL,
    enrollment_id integer NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    raw_response_gz bytea NOT NULL
);


--
-- Name: windows_mdm_responses_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.windows_mdm_responses ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.windows_mdm_responses_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: wstep_cert_auth_associations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.wstep_cert_auth_associations (
    id character varying(255) NOT NULL,
    sha256 character(64) NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: wstep_certificates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.wstep_certificates (
    serial bigint NOT NULL,
    name character varying(1024) NOT NULL,
    not_valid_before timestamp without time zone NOT NULL,
    not_valid_after timestamp without time zone NOT NULL,
    certificate_pem text NOT NULL,
    revoked boolean DEFAULT false NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: wstep_serials; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.wstep_serials (
    serial bigint NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: wstep_serials_serial_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.wstep_serials ALTER COLUMN serial ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.wstep_serials_serial_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: yara_rules; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.yara_rules (
    id integer NOT NULL,
    name character varying(255) NOT NULL,
    contents text NOT NULL
);


--
-- Name: yara_rules_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.yara_rules ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.yara_rules_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: host_scd_data id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_scd_data ALTER COLUMN id SET DEFAULT nextval('public.host_scd_data_id_seq'::regclass);


--
-- Name: migration_status_data id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.migration_status_data ALTER COLUMN id SET DEFAULT nextval('public.migration_status_data_id_seq'::regclass);


--
-- Name: abm_tokens abm_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.abm_tokens
    ADD CONSTRAINT abm_tokens_pkey PRIMARY KEY (id);


--
-- Name: acme_accounts acme_accounts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.acme_accounts
    ADD CONSTRAINT acme_accounts_pkey PRIMARY KEY (id);


--
-- Name: acme_authorizations acme_authorizations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.acme_authorizations
    ADD CONSTRAINT acme_authorizations_pkey PRIMARY KEY (id);


--
-- Name: acme_challenges acme_challenges_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.acme_challenges
    ADD CONSTRAINT acme_challenges_pkey PRIMARY KEY (id);


--
-- Name: acme_enrollments acme_enrollments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.acme_enrollments
    ADD CONSTRAINT acme_enrollments_pkey PRIMARY KEY (id);


--
-- Name: acme_orders acme_orders_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.acme_orders
    ADD CONSTRAINT acme_orders_pkey PRIMARY KEY (id);


--
-- Name: activity_host_past activity_host_past_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.activity_host_past
    ADD CONSTRAINT activity_host_past_pkey PRIMARY KEY (host_id, activity_id);


--
-- Name: activity_past activity_past_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.activity_past
    ADD CONSTRAINT activity_past_pkey PRIMARY KEY (id);


--
-- Name: aggregated_stats aggregated_stats_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.aggregated_stats
    ADD CONSTRAINT aggregated_stats_pkey PRIMARY KEY (id, type, global_stats);


--
-- Name: android_app_configurations android_app_configurations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.android_app_configurations
    ADD CONSTRAINT android_app_configurations_pkey PRIMARY KEY (id);


--
-- Name: android_devices android_devices_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.android_devices
    ADD CONSTRAINT android_devices_pkey PRIMARY KEY (id);


--
-- Name: android_enterprises android_enterprises_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.android_enterprises
    ADD CONSTRAINT android_enterprises_pkey PRIMARY KEY (id);


--
-- Name: android_policy_requests android_policy_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.android_policy_requests
    ADD CONSTRAINT android_policy_requests_pkey PRIMARY KEY (request_uuid);


--
-- Name: app_config_json app_config_json_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.app_config_json
    ADD CONSTRAINT app_config_json_pkey PRIMARY KEY (id);


--
-- Name: batch_activities batch_activities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.batch_activities
    ADD CONSTRAINT batch_activities_pkey PRIMARY KEY (id);


--
-- Name: batch_activity_host_results batch_activity_host_results_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.batch_activity_host_results
    ADD CONSTRAINT batch_activity_host_results_pkey PRIMARY KEY (id);


--
-- Name: ca_config_assets ca_config_assets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ca_config_assets
    ADD CONSTRAINT ca_config_assets_pkey PRIMARY KEY (id);


--
-- Name: calendar_events calendar_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.calendar_events
    ADD CONSTRAINT calendar_events_pkey PRIMARY KEY (id);


--
-- Name: carve_blocks carve_blocks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.carve_blocks
    ADD CONSTRAINT carve_blocks_pkey PRIMARY KEY (metadata_id, block_id);


--
-- Name: carve_metadata carve_metadata_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.carve_metadata
    ADD CONSTRAINT carve_metadata_pkey PRIMARY KEY (id);


--
-- Name: certificate_authorities certificate_authorities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.certificate_authorities
    ADD CONSTRAINT certificate_authorities_pkey PRIMARY KEY (id);


--
-- Name: certificate_templates certificate_templates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.certificate_templates
    ADD CONSTRAINT certificate_templates_pkey PRIMARY KEY (id);


--
-- Name: challenges challenges_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.challenges
    ADD CONSTRAINT challenges_pkey PRIMARY KEY (challenge);


--
-- Name: conditional_access_scep_certificates conditional_access_scep_certificates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conditional_access_scep_certificates
    ADD CONSTRAINT conditional_access_scep_certificates_pkey PRIMARY KEY (serial);


--
-- Name: conditional_access_scep_serials conditional_access_scep_serials_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conditional_access_scep_serials
    ADD CONSTRAINT conditional_access_scep_serials_pkey PRIMARY KEY (serial);


--
-- Name: pack_targets constraint_pack_target_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pack_targets
    ADD CONSTRAINT constraint_pack_target_unique UNIQUE (pack_id, target_id, type);


--
-- Name: cron_stats cron_stats_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cron_stats
    ADD CONSTRAINT cron_stats_pkey PRIMARY KEY (id);


--
-- Name: custom_host_vitals custom_host_vitals_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_host_vitals
    ADD CONSTRAINT custom_host_vitals_pkey PRIMARY KEY (id);


--
-- Name: cve_meta cve_meta_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cve_meta
    ADD CONSTRAINT cve_meta_pkey PRIMARY KEY (cve);


--
-- Name: distributed_query_campaign_targets distributed_query_campaign_targets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.distributed_query_campaign_targets
    ADD CONSTRAINT distributed_query_campaign_targets_pkey PRIMARY KEY (id);


--
-- Name: distributed_query_campaigns distributed_query_campaigns_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.distributed_query_campaigns
    ADD CONSTRAINT distributed_query_campaigns_pkey PRIMARY KEY (id);


--
-- Name: email_changes email_changes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.email_changes
    ADD CONSTRAINT email_changes_pkey PRIMARY KEY (id);


--
-- Name: enroll_secrets enroll_secrets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.enroll_secrets
    ADD CONSTRAINT enroll_secrets_pkey PRIMARY KEY (secret);


--
-- Name: eulas eulas_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.eulas
    ADD CONSTRAINT eulas_pkey PRIMARY KEY (id);


--
-- Name: fleet_maintained_apps fleet_maintained_apps_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.fleet_maintained_apps
    ADD CONSTRAINT fleet_maintained_apps_pkey PRIMARY KEY (id);


--
-- Name: fleet_variables fleet_variables_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.fleet_variables
    ADD CONSTRAINT fleet_variables_pkey PRIMARY KEY (id);


--
-- Name: in_house_apps global_or_team_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.in_house_apps
    ADD CONSTRAINT global_or_team_id UNIQUE (global_or_team_id, filename, platform);


--
-- Name: host_additional host_additional_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_additional
    ADD CONSTRAINT host_additional_pkey PRIMARY KEY (host_id);


--
-- Name: host_batteries host_batteries_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_batteries
    ADD CONSTRAINT host_batteries_pkey PRIMARY KEY (id);


--
-- Name: host_calendar_events host_calendar_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_calendar_events
    ADD CONSTRAINT host_calendar_events_pkey PRIMARY KEY (id);


--
-- Name: host_certificate_sources host_certificate_sources_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_certificate_sources
    ADD CONSTRAINT host_certificate_sources_pkey PRIMARY KEY (id);


--
-- Name: host_certificate_templates host_certificate_templates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_certificate_templates
    ADD CONSTRAINT host_certificate_templates_pkey PRIMARY KEY (id);


--
-- Name: host_certificates host_certificates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_certificates
    ADD CONSTRAINT host_certificates_pkey PRIMARY KEY (id);


--
-- Name: host_conditional_access host_conditional_access_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_conditional_access
    ADD CONSTRAINT host_conditional_access_pkey PRIMARY KEY (id);


--
-- Name: host_custom_host_vitals host_custom_host_vitals_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_custom_host_vitals
    ADD CONSTRAINT host_custom_host_vitals_pkey PRIMARY KEY (id);


--
-- Name: host_dep_assignments host_dep_assignments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_dep_assignments
    ADD CONSTRAINT host_dep_assignments_pkey PRIMARY KEY (host_id);


--
-- Name: host_device_auth host_device_auth_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_device_auth
    ADD CONSTRAINT host_device_auth_pkey PRIMARY KEY (host_id);


--
-- Name: host_disk_encryption_keys_archive host_disk_encryption_keys_archive_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_disk_encryption_keys_archive
    ADD CONSTRAINT host_disk_encryption_keys_archive_pkey PRIMARY KEY (id);


--
-- Name: host_disk_encryption_keys host_disk_encryption_keys_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_disk_encryption_keys
    ADD CONSTRAINT host_disk_encryption_keys_pkey PRIMARY KEY (host_id);


--
-- Name: host_disks host_disks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_disks
    ADD CONSTRAINT host_disks_pkey PRIMARY KEY (host_id);


--
-- Name: host_display_names host_display_names_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_display_names
    ADD CONSTRAINT host_display_names_pkey PRIMARY KEY (host_id);


--
-- Name: host_emails host_emails_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_emails
    ADD CONSTRAINT host_emails_pkey PRIMARY KEY (id);


--
-- Name: host_identity_scep_certificates host_identity_scep_certificates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_identity_scep_certificates
    ADD CONSTRAINT host_identity_scep_certificates_pkey PRIMARY KEY (serial);


--
-- Name: host_identity_scep_serials host_identity_scep_serials_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_identity_scep_serials
    ADD CONSTRAINT host_identity_scep_serials_pkey PRIMARY KEY (serial);


--
-- Name: host_in_house_software_installs host_in_house_software_installs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_in_house_software_installs
    ADD CONSTRAINT host_in_house_software_installs_pkey PRIMARY KEY (id);


--
-- Name: host_issues host_issues_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_issues
    ADD CONSTRAINT host_issues_pkey PRIMARY KEY (host_id);


--
-- Name: host_last_known_locations host_last_known_locations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_last_known_locations
    ADD CONSTRAINT host_last_known_locations_pkey PRIMARY KEY (host_id);


--
-- Name: host_managed_local_account_passwords host_managed_local_account_passwords_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_managed_local_account_passwords
    ADD CONSTRAINT host_managed_local_account_passwords_pkey PRIMARY KEY (host_uuid);


--
-- Name: host_mdm_actions host_mdm_actions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_mdm_actions
    ADD CONSTRAINT host_mdm_actions_pkey PRIMARY KEY (host_id);


--
-- Name: host_mdm_android_profiles host_mdm_android_profiles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_mdm_android_profiles
    ADD CONSTRAINT host_mdm_android_profiles_pkey PRIMARY KEY (host_uuid, profile_uuid);


--
-- Name: host_mdm_apple_awaiting_configuration host_mdm_apple_awaiting_configuration_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_mdm_apple_awaiting_configuration
    ADD CONSTRAINT host_mdm_apple_awaiting_configuration_pkey PRIMARY KEY (host_uuid);


--
-- Name: host_mdm_apple_bootstrap_packages host_mdm_apple_bootstrap_packages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_mdm_apple_bootstrap_packages
    ADD CONSTRAINT host_mdm_apple_bootstrap_packages_pkey PRIMARY KEY (host_uuid);


--
-- Name: host_mdm_apple_declarations host_mdm_apple_declarations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_mdm_apple_declarations
    ADD CONSTRAINT host_mdm_apple_declarations_pkey PRIMARY KEY (host_uuid, declaration_uuid);


--
-- Name: host_mdm_apple_device_names host_mdm_apple_device_names_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_mdm_apple_device_names
    ADD CONSTRAINT host_mdm_apple_device_names_pkey PRIMARY KEY (host_uuid);


--
-- Name: host_mdm_apple_enrollment_permissions host_mdm_apple_enrollment_permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_mdm_apple_enrollment_permissions
    ADD CONSTRAINT host_mdm_apple_enrollment_permissions_pkey PRIMARY KEY (host_uuid);


--
-- Name: host_mdm_apple_profiles host_mdm_apple_profiles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_mdm_apple_profiles
    ADD CONSTRAINT host_mdm_apple_profiles_pkey PRIMARY KEY (host_uuid, profile_uuid);


--
-- Name: host_mdm_commands host_mdm_commands_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_mdm_commands
    ADD CONSTRAINT host_mdm_commands_pkey PRIMARY KEY (host_id, command_type);


--
-- Name: host_mdm_idp_accounts host_mdm_idp_accounts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_mdm_idp_accounts
    ADD CONSTRAINT host_mdm_idp_accounts_pkey PRIMARY KEY (id);


--
-- Name: host_mdm_managed_certificates host_mdm_managed_certificates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_mdm_managed_certificates
    ADD CONSTRAINT host_mdm_managed_certificates_pkey PRIMARY KEY (host_uuid, profile_uuid, ca_name);


--
-- Name: host_mdm host_mdm_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_mdm
    ADD CONSTRAINT host_mdm_pkey PRIMARY KEY (host_id);


--
-- Name: host_mdm_windows_profiles host_mdm_windows_profiles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_mdm_windows_profiles
    ADD CONSTRAINT host_mdm_windows_profiles_pkey PRIMARY KEY (host_uuid, profile_uuid);


--
-- Name: host_mdm_windows_profiles_status host_mdm_windows_profiles_status_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_mdm_windows_profiles_status
    ADD CONSTRAINT host_mdm_windows_profiles_status_pkey PRIMARY KEY (host_uuid);


--
-- Name: host_munki_info host_munki_info_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_munki_info
    ADD CONSTRAINT host_munki_info_pkey PRIMARY KEY (host_id);


--
-- Name: host_munki_issues host_munki_issues_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_munki_issues
    ADD CONSTRAINT host_munki_issues_pkey PRIMARY KEY (host_id, munki_issue_id);


--
-- Name: host_operating_system host_operating_system_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_operating_system
    ADD CONSTRAINT host_operating_system_pkey PRIMARY KEY (host_id);


--
-- Name: host_orbit_info host_orbit_info_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_orbit_info
    ADD CONSTRAINT host_orbit_info_pkey PRIMARY KEY (host_id);


--
-- Name: host_recovery_key_passwords host_recovery_key_passwords_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_recovery_key_passwords
    ADD CONSTRAINT host_recovery_key_passwords_pkey PRIMARY KEY (host_uuid);


--
-- Name: host_scd_data host_scd_data_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_scd_data
    ADD CONSTRAINT host_scd_data_pkey PRIMARY KEY (id);


--
-- Name: host_scim_user host_scim_user_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_scim_user
    ADD CONSTRAINT host_scim_user_pkey PRIMARY KEY (host_id);


--
-- Name: host_script_results host_script_results_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_script_results
    ADD CONSTRAINT host_script_results_pkey PRIMARY KEY (id);


--
-- Name: host_seen_times host_seen_times_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_seen_times
    ADD CONSTRAINT host_seen_times_pkey PRIMARY KEY (host_id);


--
-- Name: host_software_installed_paths host_software_installed_paths_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_software_installed_paths
    ADD CONSTRAINT host_software_installed_paths_pkey PRIMARY KEY (id);


--
-- Name: host_software_installs host_software_installs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_software_installs
    ADD CONSTRAINT host_software_installs_pkey PRIMARY KEY (id);


--
-- Name: host_software host_software_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_software
    ADD CONSTRAINT host_software_pkey PRIMARY KEY (host_id, software_id);


--
-- Name: host_updates host_updates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_updates
    ADD CONSTRAINT host_updates_pkey PRIMARY KEY (host_id);


--
-- Name: host_users host_users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_users
    ADD CONSTRAINT host_users_pkey PRIMARY KEY (host_id, uid, username);


--
-- Name: host_vpp_software_installs host_vpp_software_installs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_vpp_software_installs
    ADD CONSTRAINT host_vpp_software_installs_pkey PRIMARY KEY (id);


--
-- Name: hosts hosts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.hosts
    ADD CONSTRAINT hosts_pkey PRIMARY KEY (id);


--
-- Name: default_team_config_json id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.default_team_config_json
    ADD CONSTRAINT id PRIMARY KEY (id);


--
-- Name: in_house_app_labels id_in_house_app_labels_in_house_app_id_label_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.in_house_app_labels
    ADD CONSTRAINT id_in_house_app_labels_in_house_app_id_label_id UNIQUE (in_house_app_id, label_id);


--
-- Name: abm_tokens idx_abm_tokens_enrollment_url_token; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.abm_tokens
    ADD CONSTRAINT idx_abm_tokens_enrollment_url_token UNIQUE (enrollment_url_token);


--
-- Name: abm_tokens idx_abm_tokens_organization_name; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.abm_tokens
    ADD CONSTRAINT idx_abm_tokens_organization_name UNIQUE (organization_name);


--
-- Name: android_devices idx_android_devices_device_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.android_devices
    ADD CONSTRAINT idx_android_devices_device_id UNIQUE (device_id);


--
-- Name: android_devices idx_android_devices_enterprise_specific_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.android_devices
    ADD CONSTRAINT idx_android_devices_enterprise_specific_id UNIQUE (enterprise_specific_id);


--
-- Name: android_devices idx_android_devices_host_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.android_devices
    ADD CONSTRAINT idx_android_devices_host_id UNIQUE (host_id);


--
-- Name: batch_activities idx_batch_script_executions_execution_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.batch_activities
    ADD CONSTRAINT idx_batch_script_executions_execution_id UNIQUE (execution_id);


--
-- Name: ca_config_assets idx_ca_config_assets_name; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ca_config_assets
    ADD CONSTRAINT idx_ca_config_assets_name UNIQUE (name);


--
-- Name: certificate_authorities idx_ca_type_name; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.certificate_authorities
    ADD CONSTRAINT idx_ca_type_name UNIQUE (type, name);


--
-- Name: calendar_events idx_calendar_events_uuid_bin_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.calendar_events
    ADD CONSTRAINT idx_calendar_events_uuid_bin_unique UNIQUE (uuid_bin);


--
-- Name: certificate_templates idx_cert_team_name; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.certificate_templates
    ADD CONSTRAINT idx_cert_team_name UNIQUE (team_id, name);


--
-- Name: custom_host_vitals idx_custom_host_vitals_name; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_host_vitals
    ADD CONSTRAINT idx_custom_host_vitals_name UNIQUE (name);


--
-- Name: acme_accounts idx_enrollment_id_thumbprint; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.acme_accounts
    ADD CONSTRAINT idx_enrollment_id_thumbprint UNIQUE (acme_enrollment_id, json_web_key_thumbprint);


--
-- Name: mdm_apple_enrollment_profiles idx_enrollment_profiles_token; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_apple_enrollment_profiles
    ADD CONSTRAINT idx_enrollment_profiles_token UNIQUE (token);


--
-- Name: mdm_apple_enrollment_profiles idx_enrollment_profiles_type; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_apple_enrollment_profiles
    ADD CONSTRAINT idx_enrollment_profiles_type UNIQUE (type);


--
-- Name: fleet_maintained_apps idx_fleet_library_apps_token; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.fleet_maintained_apps
    ADD CONSTRAINT idx_fleet_library_apps_token UNIQUE (slug);


--
-- Name: fleet_variables idx_fleet_variables_name_is_prefix; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.fleet_variables
    ADD CONSTRAINT idx_fleet_variables_name_is_prefix UNIQUE (name, is_prefix);


--
-- Name: vpp_apps_teams idx_global_or_team_id_adam_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vpp_apps_teams
    ADD CONSTRAINT idx_global_or_team_id_adam_id UNIQUE (global_or_team_id, adam_id, platform);


--
-- Name: android_app_configurations idx_global_or_team_id_application_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.android_app_configurations
    ADD CONSTRAINT idx_global_or_team_id_application_id UNIQUE (global_or_team_id, application_id);


--
-- Name: host_batteries idx_host_batteries_host_id_serial_number; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_batteries
    ADD CONSTRAINT idx_host_batteries_host_id_serial_number UNIQUE (host_id, serial_number);


--
-- Name: host_certificate_sources idx_host_certificate_sources_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_certificate_sources
    ADD CONSTRAINT idx_host_certificate_sources_unique UNIQUE (host_certificate_id, source, username);


--
-- Name: host_certificate_templates idx_host_certificate_templates_host_template; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_certificate_templates
    ADD CONSTRAINT idx_host_certificate_templates_host_template UNIQUE (host_uuid, certificate_template_id);


--
-- Name: host_conditional_access idx_host_conditional_access_host_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_conditional_access
    ADD CONSTRAINT idx_host_conditional_access_host_id UNIQUE (host_id);


--
-- Name: host_custom_host_vitals idx_host_custom_host_vitals_host_vital; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_custom_host_vitals
    ADD CONSTRAINT idx_host_custom_host_vitals_host_vital UNIQUE (host_id, custom_host_vital_id);


--
-- Name: host_device_auth idx_host_device_auth_token; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_device_auth
    ADD CONSTRAINT idx_host_device_auth_token UNIQUE (token);


--
-- Name: host_in_house_software_installs idx_host_in_house_software_installs_command_uuid; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_in_house_software_installs
    ADD CONSTRAINT idx_host_in_house_software_installs_command_uuid UNIQUE (command_uuid);


--
-- Name: host_mdm_idp_accounts idx_host_mdm_idp_accounts; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_mdm_idp_accounts
    ADD CONSTRAINT idx_host_mdm_idp_accounts UNIQUE (host_uuid);


--
-- Name: host_script_results idx_host_script_results_execution_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_script_results
    ADD CONSTRAINT idx_host_script_results_execution_id UNIQUE (execution_id);


--
-- Name: host_software_installs idx_host_software_installs_execution_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_software_installs
    ADD CONSTRAINT idx_host_software_installs_execution_id UNIQUE (execution_id);


--
-- Name: hosts idx_host_unique_nodekey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.hosts
    ADD CONSTRAINT idx_host_unique_nodekey UNIQUE (node_key);


--
-- Name: hosts idx_host_unique_orbitnodekey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.hosts
    ADD CONSTRAINT idx_host_unique_orbitnodekey UNIQUE (orbit_node_key);


--
-- Name: host_vpp_software_installs idx_host_vpp_software_installs_command_uuid; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_vpp_software_installs
    ADD CONSTRAINT idx_host_vpp_software_installs_command_uuid UNIQUE (command_uuid);


--
-- Name: in_house_app_configurations idx_in_house_app_config_app; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.in_house_app_configurations
    ADD CONSTRAINT idx_in_house_app_config_app UNIQUE (in_house_app_id);


--
-- Name: invites idx_invite_unique_email; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.invites
    ADD CONSTRAINT idx_invite_unique_email UNIQUE (email);


--
-- Name: invites idx_invite_unique_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.invites
    ADD CONSTRAINT idx_invite_unique_key UNIQUE (token);


--
-- Name: acme_orders idx_issued_certificate_serial; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.acme_orders
    ADD CONSTRAINT idx_issued_certificate_serial UNIQUE (issued_certificate_serial);


--
-- Name: labels idx_label_unique_name; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.labels
    ADD CONSTRAINT idx_label_unique_name UNIQUE (name);


--
-- Name: mdm_adue_enrollment_challenges idx_mdm_adue_challenge; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_adue_enrollment_challenges
    ADD CONSTRAINT idx_mdm_adue_challenge UNIQUE (challenge);


--
-- Name: mdm_android_configuration_profiles idx_mdm_android_auto_increment; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_android_configuration_profiles
    ADD CONSTRAINT idx_mdm_android_auto_increment UNIQUE (auto_increment);


--
-- Name: mdm_android_configuration_profiles idx_mdm_android_configuration_profiles_team_id_name; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_android_configuration_profiles
    ADD CONSTRAINT idx_mdm_android_configuration_profiles_team_id_name UNIQUE (team_id, name);


--
-- Name: mdm_apple_configuration_profiles idx_mdm_apple_config_prof_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_apple_configuration_profiles
    ADD CONSTRAINT idx_mdm_apple_config_prof_id UNIQUE (profile_id);


--
-- Name: mdm_apple_configuration_profiles idx_mdm_apple_config_prof_team_identifier; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_apple_configuration_profiles
    ADD CONSTRAINT idx_mdm_apple_config_prof_team_identifier UNIQUE (team_id, identifier);


--
-- Name: mdm_apple_configuration_profiles idx_mdm_apple_config_prof_team_name; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_apple_configuration_profiles
    ADD CONSTRAINT idx_mdm_apple_config_prof_team_name UNIQUE (team_id, name);


--
-- Name: mdm_apple_declaration_assets idx_mdm_apple_decl_asset_team_identifier; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_apple_declaration_assets
    ADD CONSTRAINT idx_mdm_apple_decl_asset_team_identifier UNIQUE (team_id, identifier);


--
-- Name: mdm_apple_declaration_assets idx_mdm_apple_decl_asset_team_name; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_apple_declaration_assets
    ADD CONSTRAINT idx_mdm_apple_decl_asset_team_name UNIQUE (team_id, name);


--
-- Name: mdm_apple_declarations idx_mdm_apple_declaration_team_identifier; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_apple_declarations
    ADD CONSTRAINT idx_mdm_apple_declaration_team_identifier UNIQUE (team_id, identifier);


--
-- Name: mdm_apple_declarations idx_mdm_apple_declaration_team_name; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_apple_declarations
    ADD CONSTRAINT idx_mdm_apple_declaration_team_name UNIQUE (team_id, name);


--
-- Name: mdm_apple_declarations idx_mdm_apple_declarations_auto_increment; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_apple_declarations
    ADD CONSTRAINT idx_mdm_apple_declarations_auto_increment UNIQUE (auto_increment);


--
-- Name: mdm_apple_setup_assistant_profiles idx_mdm_apple_setup_assistant_profiles_asst_id_tok_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_apple_setup_assistant_profiles
    ADD CONSTRAINT idx_mdm_apple_setup_assistant_profiles_asst_id_tok_id UNIQUE (setup_assistant_id, abm_token_id);


--
-- Name: mdm_config_assets idx_mdm_config_assets_name_deletion_uuid; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_config_assets
    ADD CONSTRAINT idx_mdm_config_assets_name_deletion_uuid UNIQUE (name, deletion_uuid);


--
-- Name: mdm_configuration_profile_update_settings idx_mdm_config_profile_update_settings_apple_decl; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_configuration_profile_update_settings
    ADD CONSTRAINT idx_mdm_config_profile_update_settings_apple_decl UNIQUE (apple_declaration_uuid);


--
-- Name: mdm_configuration_profile_update_settings idx_mdm_config_profile_update_settings_windows_profile; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_configuration_profile_update_settings
    ADD CONSTRAINT idx_mdm_config_profile_update_settings_windows_profile UNIQUE (windows_profile_uuid);


--
-- Name: mdm_configuration_profile_labels idx_mdm_configuration_profile_labels_android_label_name; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_configuration_profile_labels
    ADD CONSTRAINT idx_mdm_configuration_profile_labels_android_label_name UNIQUE (android_profile_uuid, label_name);


--
-- Name: mdm_configuration_profile_labels idx_mdm_configuration_profile_labels_apple_label_name; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_configuration_profile_labels
    ADD CONSTRAINT idx_mdm_configuration_profile_labels_apple_label_name UNIQUE (apple_profile_uuid, label_name);


--
-- Name: mdm_configuration_profile_labels idx_mdm_configuration_profile_labels_windows_label_name; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_configuration_profile_labels
    ADD CONSTRAINT idx_mdm_configuration_profile_labels_windows_label_name UNIQUE (windows_profile_uuid, label_name);


--
-- Name: mdm_configuration_profile_variables idx_mdm_configuration_profile_variables_apple_variable; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_configuration_profile_variables
    ADD CONSTRAINT idx_mdm_configuration_profile_variables_apple_variable UNIQUE (apple_profile_uuid, fleet_variable_id);


--
-- Name: mdm_configuration_profile_variables idx_mdm_configuration_profile_variables_windows_label_name; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_configuration_profile_variables
    ADD CONSTRAINT idx_mdm_configuration_profile_variables_windows_label_name UNIQUE (windows_profile_uuid, fleet_variable_id);


--
-- Name: mdm_declaration_labels idx_mdm_declaration_labels_label_name; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_declaration_labels
    ADD CONSTRAINT idx_mdm_declaration_labels_label_name UNIQUE (apple_declaration_uuid, label_name);


--
-- Name: mdm_apple_default_setup_assistants idx_mdm_default_setup_assistant_global_or_team_id_abm_token_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_apple_default_setup_assistants
    ADD CONSTRAINT idx_mdm_default_setup_assistant_global_or_team_id_abm_token_id UNIQUE (global_or_team_id, abm_token_id);


--
-- Name: mdm_apple_setup_assistants idx_mdm_setup_assistant_global_or_team_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_apple_setup_assistants
    ADD CONSTRAINT idx_mdm_setup_assistant_global_or_team_id UNIQUE (global_or_team_id);


--
-- Name: mdm_windows_configuration_profiles idx_mdm_win_config_auto_increment; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_windows_configuration_profiles
    ADD CONSTRAINT idx_mdm_win_config_auto_increment UNIQUE (auto_increment);


--
-- Name: mdm_windows_configuration_profiles idx_mdm_windows_configuration_profiles_team_id_name; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_windows_configuration_profiles
    ADD CONSTRAINT idx_mdm_windows_configuration_profiles_team_id_name UNIQUE (team_id, name);


--
-- Name: microsoft_compliance_partner_integrations idx_microsoft_compliance_partner_tenant_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.microsoft_compliance_partner_integrations
    ADD CONSTRAINT idx_microsoft_compliance_partner_tenant_id UNIQUE (tenant_id);


--
-- Name: mobile_device_management_solutions idx_mobile_device_management_solutions_name; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mobile_device_management_solutions
    ADD CONSTRAINT idx_mobile_device_management_solutions_name UNIQUE (name, server_url);


--
-- Name: munki_issues idx_munki_issues_name; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.munki_issues
    ADD CONSTRAINT idx_munki_issues_name UNIQUE (name, issue_type);


--
-- Name: carve_metadata idx_name; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.carve_metadata
    ADD CONSTRAINT idx_name UNIQUE (name);


--
-- Name: teams idx_name_bin; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.teams
    ADD CONSTRAINT idx_name_bin UNIQUE (name_bin);


--
-- Name: queries idx_name_team_id_unq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.queries
    ADD CONSTRAINT idx_name_team_id_unq UNIQUE (name, team_id_char);


--
-- Name: network_interfaces idx_network_interfaces_unique_ip_host_intf; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.network_interfaces
    ADD CONSTRAINT idx_network_interfaces_unique_ip_host_intf UNIQUE (ip_address, host_id, interface);


--
-- Name: calendar_events idx_one_calendar_event_per_email; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.calendar_events
    ADD CONSTRAINT idx_one_calendar_event_per_email UNIQUE (email);


--
-- Name: host_calendar_events idx_one_calendar_event_per_host; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_calendar_events
    ADD CONSTRAINT idx_one_calendar_event_per_host UNIQUE (host_id);


--
-- Name: operating_system_vulnerabilities idx_os_vulnerabilities_unq_os_id_cve; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operating_system_vulnerabilities
    ADD CONSTRAINT idx_os_vulnerabilities_unq_os_id_cve UNIQUE (operating_system_id, cve);


--
-- Name: hosts idx_osquery_host_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.hosts
    ADD CONSTRAINT idx_osquery_host_id UNIQUE (osquery_host_id);


--
-- Name: packs idx_pack_unique_name; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.packs
    ADD CONSTRAINT idx_pack_unique_name UNIQUE (name);


--
-- Name: acme_enrollments idx_path_identifier; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.acme_enrollments
    ADD CONSTRAINT idx_path_identifier UNIQUE (path_identifier);


--
-- Name: policies idx_policies_checksum; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.policies
    ADD CONSTRAINT idx_policies_checksum UNIQUE (checksum);


--
-- Name: policy_labels idx_policy_labels_policy_label; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.policy_labels
    ADD CONSTRAINT idx_policy_labels_policy_label UNIQUE (policy_id, label_id);


--
-- Name: query_labels idx_query_labels_query_label; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.query_labels
    ADD CONSTRAINT idx_query_labels_query_label UNIQUE (query_id, label_id);


--
-- Name: scim_groups idx_scim_groups_display_name; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scim_groups
    ADD CONSTRAINT idx_scim_groups_display_name UNIQUE (display_name);


--
-- Name: scim_users idx_scim_users_user_name; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scim_users
    ADD CONSTRAINT idx_scim_users_user_name UNIQUE (user_name);


--
-- Name: script_contents idx_script_contents_md5_checksum; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.script_contents
    ADD CONSTRAINT idx_script_contents_md5_checksum UNIQUE (md5_checksum);


--
-- Name: scripts idx_scripts_global_or_team_id_name; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scripts
    ADD CONSTRAINT idx_scripts_global_or_team_id_name UNIQUE (global_or_team_id, name);


--
-- Name: scripts idx_scripts_team_name; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scripts
    ADD CONSTRAINT idx_scripts_team_name UNIQUE (team_id, name);


--
-- Name: secret_variables idx_secret_variables_name; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.secret_variables
    ADD CONSTRAINT idx_secret_variables_name UNIQUE (name);


--
-- Name: carve_metadata idx_session_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.carve_metadata
    ADD CONSTRAINT idx_session_id UNIQUE (session_id);


--
-- Name: sessions idx_session_unique_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT idx_session_unique_key UNIQUE (key);


--
-- Name: setup_experience_scripts idx_setup_experience_scripts_global_or_team_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.setup_experience_scripts
    ADD CONSTRAINT idx_setup_experience_scripts_global_or_team_id UNIQUE (global_or_team_id);


--
-- Name: software_categories idx_software_categories_team_id_name; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.software_categories
    ADD CONSTRAINT idx_software_categories_team_id_name UNIQUE (team_id, name);


--
-- Name: software idx_software_checksum; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.software
    ADD CONSTRAINT idx_software_checksum UNIQUE (checksum);


--
-- Name: software_installer_labels idx_software_installer_labels_software_installer_id_label_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.software_installer_labels
    ADD CONSTRAINT idx_software_installer_labels_software_installer_id_label_id UNIQUE (software_installer_id, label_id);


--
-- Name: software_titles idx_software_titles_bundle_identifier; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.software_titles
    ADD CONSTRAINT idx_software_titles_bundle_identifier UNIQUE (bundle_identifier, additional_identifier);


--
-- Name: queries idx_team_id_name_unq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.queries
    ADD CONSTRAINT idx_team_id_name_unq UNIQUE (team_id_char, name);


--
-- Name: software_update_schedules idx_team_title; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.software_update_schedules
    ADD CONSTRAINT idx_team_title UNIQUE (team_id, title_id);


--
-- Name: teams idx_teams_filename; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.teams
    ADD CONSTRAINT idx_teams_filename UNIQUE (filename);


--
-- Name: mdm_apple_bootstrap_packages idx_token; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_apple_bootstrap_packages
    ADD CONSTRAINT idx_token UNIQUE (token);


--
-- Name: email_changes idx_unique_email_changes_token; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.email_changes
    ADD CONSTRAINT idx_unique_email_changes_token UNIQUE (token);


--
-- Name: nano_users idx_unique_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nano_users
    ADD CONSTRAINT idx_unique_id UNIQUE (id);


--
-- Name: in_house_app_software_categories idx_unique_in_house_app_id_software_category_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.in_house_app_software_categories
    ADD CONSTRAINT idx_unique_in_house_app_id_software_category_id UNIQUE (in_house_app_id, software_category_id);


--
-- Name: operating_systems idx_unique_os; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operating_systems
    ADD CONSTRAINT idx_unique_os UNIQUE (name, version, arch, kernel_version, platform, display_version, installation_type);


--
-- Name: software_installer_software_categories idx_unique_software_installer_id_software_category_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.software_installer_software_categories
    ADD CONSTRAINT idx_unique_software_installer_id_software_category_id UNIQUE (software_installer_id, software_category_id);


--
-- Name: software_titles idx_unique_sw_titles; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.software_titles
    ADD CONSTRAINT idx_unique_sw_titles UNIQUE (unique_identifier, source, extension_for);


--
-- Name: software_title_display_names idx_unique_team_id_title_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.software_title_display_names
    ADD CONSTRAINT idx_unique_team_id_title_id UNIQUE (team_id, software_title_id);


--
-- Name: software_title_icons idx_unique_team_id_title_id_storage_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.software_title_icons
    ADD CONSTRAINT idx_unique_team_id_title_id_storage_id UNIQUE (team_id, software_title_id);


--
-- Name: vpp_app_team_software_categories idx_unique_vpp_app_team_id_software_category_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vpp_app_team_software_categories
    ADD CONSTRAINT idx_unique_vpp_app_team_id_software_category_id UNIQUE (vpp_app_team_id, software_category_id);


--
-- Name: upcoming_activities idx_upcoming_activities_execution_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.upcoming_activities
    ADD CONSTRAINT idx_upcoming_activities_execution_id UNIQUE (execution_id);


--
-- Name: users idx_user_unique_email; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT idx_user_unique_email UNIQUE (email);


--
-- Name: vpp_app_configurations idx_vpp_app_config_team_app_platform; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vpp_app_configurations
    ADD CONSTRAINT idx_vpp_app_config_team_app_platform UNIQUE (team_id, application_id, platform);


--
-- Name: vpp_app_team_labels idx_vpp_app_team_labels_vpp_app_team_id_label_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vpp_app_team_labels
    ADD CONSTRAINT idx_vpp_app_team_labels_vpp_app_team_id_label_id UNIQUE (vpp_app_team_id, label_id);


--
-- Name: vpp_client_users idx_vpp_client_users_token_apple_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vpp_client_users
    ADD CONSTRAINT idx_vpp_client_users_token_apple_id UNIQUE (vpp_token_id, managed_apple_id);


--
-- Name: vpp_client_users idx_vpp_client_users_token_client_user_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vpp_client_users
    ADD CONSTRAINT idx_vpp_client_users_token_client_user_id UNIQUE (vpp_token_id, client_user_id);


--
-- Name: vpp_token_teams idx_vpp_token_teams_team_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vpp_token_teams
    ADD CONSTRAINT idx_vpp_token_teams_team_id UNIQUE (team_id);


--
-- Name: vpp_tokens idx_vpp_tokens_location; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vpp_tokens
    ADD CONSTRAINT idx_vpp_tokens_location UNIQUE (location);


--
-- Name: yara_rules idx_yara_rules_name; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.yara_rules
    ADD CONSTRAINT idx_yara_rules_name UNIQUE (name);


--
-- Name: in_house_app_configurations in_house_app_configurations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.in_house_app_configurations
    ADD CONSTRAINT in_house_app_configurations_pkey PRIMARY KEY (id);


--
-- Name: in_house_app_install_tokens in_house_app_install_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.in_house_app_install_tokens
    ADD CONSTRAINT in_house_app_install_tokens_pkey PRIMARY KEY (token);


--
-- Name: in_house_app_labels in_house_app_labels_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.in_house_app_labels
    ADD CONSTRAINT in_house_app_labels_pkey PRIMARY KEY (id);


--
-- Name: in_house_app_software_categories in_house_app_software_categories_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.in_house_app_software_categories
    ADD CONSTRAINT in_house_app_software_categories_pkey PRIMARY KEY (id);


--
-- Name: in_house_app_upcoming_activities in_house_app_upcoming_activities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.in_house_app_upcoming_activities
    ADD CONSTRAINT in_house_app_upcoming_activities_pkey PRIMARY KEY (upcoming_activity_id);


--
-- Name: in_house_apps in_house_apps_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.in_house_apps
    ADD CONSTRAINT in_house_apps_pkey PRIMARY KEY (id);


--
-- Name: users invite_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT invite_id UNIQUE (invite_id);


--
-- Name: invite_teams invite_teams_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.invite_teams
    ADD CONSTRAINT invite_teams_pkey PRIMARY KEY (invite_id, team_id);


--
-- Name: invites invites_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.invites
    ADD CONSTRAINT invites_pkey PRIMARY KEY (id);


--
-- Name: jobs jobs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.jobs
    ADD CONSTRAINT jobs_pkey PRIMARY KEY (id);


--
-- Name: kernel_host_counts kernel_host_counts_os_version_id_team_id_software_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kernel_host_counts
    ADD CONSTRAINT kernel_host_counts_os_version_id_team_id_software_id_key UNIQUE (os_version_id, team_id, software_id);


--
-- Name: kernel_host_counts kernel_host_counts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kernel_host_counts
    ADD CONSTRAINT kernel_host_counts_pkey PRIMARY KEY (id);


--
-- Name: label_membership label_membership_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.label_membership
    ADD CONSTRAINT label_membership_pkey PRIMARY KEY (host_id, label_id);


--
-- Name: labels labels_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.labels
    ADD CONSTRAINT labels_pkey PRIMARY KEY (id);


--
-- Name: legacy_host_filevault_profiles legacy_host_filevault_profiles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.legacy_host_filevault_profiles
    ADD CONSTRAINT legacy_host_filevault_profiles_pkey PRIMARY KEY (id);


--
-- Name: legacy_host_mdm_enroll_refs legacy_host_mdm_enroll_refs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.legacy_host_mdm_enroll_refs
    ADD CONSTRAINT legacy_host_mdm_enroll_refs_pkey PRIMARY KEY (id);


--
-- Name: legacy_host_mdm_idp_accounts legacy_host_mdm_idp_accounts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.legacy_host_mdm_idp_accounts
    ADD CONSTRAINT legacy_host_mdm_idp_accounts_pkey PRIMARY KEY (id);


--
-- Name: locks locks_idx_name; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.locks
    ADD CONSTRAINT locks_idx_name UNIQUE (name);


--
-- Name: locks locks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.locks
    ADD CONSTRAINT locks_pkey PRIMARY KEY (id);


--
-- Name: mdm_adue_enrollment_challenges mdm_adue_enrollment_challenges_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_adue_enrollment_challenges
    ADD CONSTRAINT mdm_adue_enrollment_challenges_pkey PRIMARY KEY (id);


--
-- Name: mdm_android_commands mdm_android_commands_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_android_commands
    ADD CONSTRAINT mdm_android_commands_pkey PRIMARY KEY (command_uuid);


--
-- Name: mdm_android_configuration_profiles mdm_android_configuration_profiles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_android_configuration_profiles
    ADD CONSTRAINT mdm_android_configuration_profiles_pkey PRIMARY KEY (profile_uuid);


--
-- Name: mdm_apple_bootstrap_packages mdm_apple_bootstrap_packages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_apple_bootstrap_packages
    ADD CONSTRAINT mdm_apple_bootstrap_packages_pkey PRIMARY KEY (team_id);


--
-- Name: mdm_apple_configuration_profiles mdm_apple_configuration_profiles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_apple_configuration_profiles
    ADD CONSTRAINT mdm_apple_configuration_profiles_pkey PRIMARY KEY (profile_uuid);


--
-- Name: mdm_apple_declaration_activation_references mdm_apple_declaration_activation_references_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_apple_declaration_activation_references
    ADD CONSTRAINT mdm_apple_declaration_activation_references_pkey PRIMARY KEY (declaration_uuid, reference);


--
-- Name: mdm_apple_declaration_asset_references mdm_apple_declaration_asset_references_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_apple_declaration_asset_references
    ADD CONSTRAINT mdm_apple_declaration_asset_references_pkey PRIMARY KEY (declaration_uuid, asset_uuid);


--
-- Name: mdm_apple_declaration_assets mdm_apple_declaration_assets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_apple_declaration_assets
    ADD CONSTRAINT mdm_apple_declaration_assets_pkey PRIMARY KEY (asset_uuid);


--
-- Name: mdm_apple_declarations mdm_apple_declarations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_apple_declarations
    ADD CONSTRAINT mdm_apple_declarations_pkey PRIMARY KEY (declaration_uuid);


--
-- Name: mdm_apple_declarative_requests mdm_apple_declarative_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_apple_declarative_requests
    ADD CONSTRAINT mdm_apple_declarative_requests_pkey PRIMARY KEY (id);


--
-- Name: mdm_apple_default_setup_assistants mdm_apple_default_setup_assistants_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_apple_default_setup_assistants
    ADD CONSTRAINT mdm_apple_default_setup_assistants_pkey PRIMARY KEY (id);


--
-- Name: mdm_apple_enrollment_profiles mdm_apple_enrollment_profiles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_apple_enrollment_profiles
    ADD CONSTRAINT mdm_apple_enrollment_profiles_pkey PRIMARY KEY (id);


--
-- Name: mdm_apple_installers mdm_apple_installers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_apple_installers
    ADD CONSTRAINT mdm_apple_installers_pkey PRIMARY KEY (id);


--
-- Name: mdm_apple_psso_devices mdm_apple_psso_devices_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_apple_psso_devices
    ADD CONSTRAINT mdm_apple_psso_devices_pkey PRIMARY KEY (host_uuid);


--
-- Name: mdm_apple_psso_keys mdm_apple_psso_keys_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_apple_psso_keys
    ADD CONSTRAINT mdm_apple_psso_keys_pkey PRIMARY KEY (kid);


--
-- Name: mdm_apple_setup_assistant_profiles mdm_apple_setup_assistant_profiles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_apple_setup_assistant_profiles
    ADD CONSTRAINT mdm_apple_setup_assistant_profiles_pkey PRIMARY KEY (id);


--
-- Name: mdm_apple_setup_assistants mdm_apple_setup_assistants_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_apple_setup_assistants
    ADD CONSTRAINT mdm_apple_setup_assistants_pkey PRIMARY KEY (id);


--
-- Name: mdm_config_assets mdm_config_assets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_config_assets
    ADD CONSTRAINT mdm_config_assets_pkey PRIMARY KEY (id);


--
-- Name: mdm_configuration_profile_labels mdm_configuration_profile_labels_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_configuration_profile_labels
    ADD CONSTRAINT mdm_configuration_profile_labels_pkey PRIMARY KEY (id);


--
-- Name: mdm_configuration_profile_update_settings mdm_configuration_profile_update_settings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_configuration_profile_update_settings
    ADD CONSTRAINT mdm_configuration_profile_update_settings_pkey PRIMARY KEY (id);


--
-- Name: mdm_configuration_profile_variables mdm_configuration_profile_variables_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_configuration_profile_variables
    ADD CONSTRAINT mdm_configuration_profile_variables_pkey PRIMARY KEY (id);


--
-- Name: mdm_declaration_labels mdm_declaration_labels_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_declaration_labels
    ADD CONSTRAINT mdm_declaration_labels_pkey PRIMARY KEY (id);


--
-- Name: mdm_delivery_status mdm_delivery_status_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_delivery_status
    ADD CONSTRAINT mdm_delivery_status_pkey PRIMARY KEY (status);


--
-- Name: mdm_idp_accounts mdm_idp_accounts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_idp_accounts
    ADD CONSTRAINT mdm_idp_accounts_pkey PRIMARY KEY (uuid);


--
-- Name: mdm_operation_types mdm_operation_types_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_operation_types
    ADD CONSTRAINT mdm_operation_types_pkey PRIMARY KEY (operation_type);


--
-- Name: mdm_windows_configuration_profiles mdm_windows_configuration_profiles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_windows_configuration_profiles
    ADD CONSTRAINT mdm_windows_configuration_profiles_pkey PRIMARY KEY (profile_uuid);


--
-- Name: mdm_windows_configuration_profiles_prior_content mdm_windows_configuration_profiles_prior_content_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_windows_configuration_profiles_prior_content
    ADD CONSTRAINT mdm_windows_configuration_profiles_prior_content_pkey PRIMARY KEY (profile_uuid, checksum);


--
-- Name: mdm_windows_enrollments mdm_windows_enrollments_idx_type; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_windows_enrollments
    ADD CONSTRAINT mdm_windows_enrollments_idx_type UNIQUE (mdm_hardware_id);


--
-- Name: mdm_windows_enrollments mdm_windows_enrollments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_windows_enrollments
    ADD CONSTRAINT mdm_windows_enrollments_pkey PRIMARY KEY (id);


--
-- Name: microsoft_compliance_partner_host_statuses microsoft_compliance_partner_host_statuses_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.microsoft_compliance_partner_host_statuses
    ADD CONSTRAINT microsoft_compliance_partner_host_statuses_pkey PRIMARY KEY (host_id);


--
-- Name: microsoft_compliance_partner_integrations microsoft_compliance_partner_integrations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.microsoft_compliance_partner_integrations
    ADD CONSTRAINT microsoft_compliance_partner_integrations_pkey PRIMARY KEY (id);


--
-- Name: migration_status_data migration_status_data_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.migration_status_data
    ADD CONSTRAINT migration_status_data_pkey PRIMARY KEY (id);


--
-- Name: migration_status_tables migration_status_tables_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.migration_status_tables
    ADD CONSTRAINT migration_status_tables_pkey PRIMARY KEY (id);


--
-- Name: mobile_device_management_solutions mobile_device_management_solutions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mobile_device_management_solutions
    ADD CONSTRAINT mobile_device_management_solutions_pkey PRIMARY KEY (id);


--
-- Name: munki_issues munki_issues_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.munki_issues
    ADD CONSTRAINT munki_issues_pkey PRIMARY KEY (id);


--
-- Name: nano_cert_auth_associations nano_cert_auth_associations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nano_cert_auth_associations
    ADD CONSTRAINT nano_cert_auth_associations_pkey PRIMARY KEY (id, sha256);


--
-- Name: nano_command_results nano_command_results_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nano_command_results
    ADD CONSTRAINT nano_command_results_pkey PRIMARY KEY (id, command_uuid);


--
-- Name: nano_commands nano_commands_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nano_commands
    ADD CONSTRAINT nano_commands_pkey PRIMARY KEY (command_uuid);


--
-- Name: nano_dep_names nano_dep_names_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nano_dep_names
    ADD CONSTRAINT nano_dep_names_pkey PRIMARY KEY (name);


--
-- Name: nano_devices nano_devices_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nano_devices
    ADD CONSTRAINT nano_devices_pkey PRIMARY KEY (id);


--
-- Name: nano_enrollment_queue nano_enrollment_queue_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nano_enrollment_queue
    ADD CONSTRAINT nano_enrollment_queue_pkey PRIMARY KEY (id, command_uuid);


--
-- Name: nano_enrollments nano_enrollments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nano_enrollments
    ADD CONSTRAINT nano_enrollments_pkey PRIMARY KEY (id);


--
-- Name: nano_push_certs nano_push_certs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nano_push_certs
    ADD CONSTRAINT nano_push_certs_pkey PRIMARY KEY (topic);


--
-- Name: nano_users nano_users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nano_users
    ADD CONSTRAINT nano_users_pkey PRIMARY KEY (id, device_id);


--
-- Name: network_interfaces network_interfaces_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.network_interfaces
    ADD CONSTRAINT network_interfaces_pkey PRIMARY KEY (id);


--
-- Name: operating_system_version_vulnerabilities operating_system_version_vulnerabilities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operating_system_version_vulnerabilities
    ADD CONSTRAINT operating_system_version_vulnerabilities_pkey PRIMARY KEY (id);


--
-- Name: operating_system_vulnerabilities operating_system_vulnerabilities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operating_system_vulnerabilities
    ADD CONSTRAINT operating_system_vulnerabilities_pkey PRIMARY KEY (id);


--
-- Name: operating_systems operating_systems_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operating_systems
    ADD CONSTRAINT operating_systems_pkey PRIMARY KEY (id);


--
-- Name: org_logo org_logo_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.org_logo
    ADD CONSTRAINT org_logo_pkey PRIMARY KEY (mode);


--
-- Name: osquery_options osquery_options_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.osquery_options
    ADD CONSTRAINT osquery_options_pkey PRIMARY KEY (id);


--
-- Name: pack_targets pack_targets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pack_targets
    ADD CONSTRAINT pack_targets_pkey PRIMARY KEY (id);


--
-- Name: packs packs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.packs
    ADD CONSTRAINT packs_pkey PRIMARY KEY (id);


--
-- Name: password_reset_requests password_reset_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.password_reset_requests
    ADD CONSTRAINT password_reset_requests_pkey PRIMARY KEY (id);


--
-- Name: policies policies_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.policies
    ADD CONSTRAINT policies_pkey PRIMARY KEY (id);


--
-- Name: policy_automation_iterations policy_automation_iterations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.policy_automation_iterations
    ADD CONSTRAINT policy_automation_iterations_pkey PRIMARY KEY (policy_id);


--
-- Name: policy_stats policy_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.policy_stats
    ADD CONSTRAINT policy_id UNIQUE (policy_id, inherited_team_id_char);


--
-- Name: policy_labels policy_labels_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.policy_labels
    ADD CONSTRAINT policy_labels_pkey PRIMARY KEY (id);


--
-- Name: policy_membership policy_membership_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.policy_membership
    ADD CONSTRAINT policy_membership_pkey PRIMARY KEY (policy_id, host_id);


--
-- Name: policy_stats policy_stats_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.policy_stats
    ADD CONSTRAINT policy_stats_pkey PRIMARY KEY (id);


--
-- Name: queries queries_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.queries
    ADD CONSTRAINT queries_pkey PRIMARY KEY (id);


--
-- Name: query_labels query_labels_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.query_labels
    ADD CONSTRAINT query_labels_pkey PRIMARY KEY (id);


--
-- Name: query_results query_results_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.query_results
    ADD CONSTRAINT query_results_pkey PRIMARY KEY (id);


--
-- Name: identity_certificates scep_certificates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.identity_certificates
    ADD CONSTRAINT scep_certificates_pkey PRIMARY KEY (serial);


--
-- Name: identity_serials scep_serials_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.identity_serials
    ADD CONSTRAINT scep_serials_pkey PRIMARY KEY (serial);


--
-- Name: scheduled_queries scheduled_queries_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scheduled_queries
    ADD CONSTRAINT scheduled_queries_pkey PRIMARY KEY (id);


--
-- Name: scheduled_query_stats scheduled_query_stats_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scheduled_query_stats
    ADD CONSTRAINT scheduled_query_stats_pkey PRIMARY KEY (host_id, scheduled_query_id, query_type);


--
-- Name: scim_groups scim_groups_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scim_groups
    ADD CONSTRAINT scim_groups_pkey PRIMARY KEY (id);


--
-- Name: scim_last_request scim_last_request_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scim_last_request
    ADD CONSTRAINT scim_last_request_pkey PRIMARY KEY (id);


--
-- Name: scim_user_emails scim_user_emails_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scim_user_emails
    ADD CONSTRAINT scim_user_emails_pkey PRIMARY KEY (id);


--
-- Name: scim_user_group scim_user_group_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scim_user_group
    ADD CONSTRAINT scim_user_group_pkey PRIMARY KEY (scim_user_id, group_id);


--
-- Name: scim_users scim_users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scim_users
    ADD CONSTRAINT scim_users_pkey PRIMARY KEY (id);


--
-- Name: script_contents script_contents_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.script_contents
    ADD CONSTRAINT script_contents_pkey PRIMARY KEY (id);


--
-- Name: script_upcoming_activities script_upcoming_activities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.script_upcoming_activities
    ADD CONSTRAINT script_upcoming_activities_pkey PRIMARY KEY (upcoming_activity_id);


--
-- Name: scripts scripts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scripts
    ADD CONSTRAINT scripts_pkey PRIMARY KEY (id);


--
-- Name: secret_variables secret_variables_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.secret_variables
    ADD CONSTRAINT secret_variables_pkey PRIMARY KEY (id);


--
-- Name: sessions sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT sessions_pkey PRIMARY KEY (id);


--
-- Name: setup_experience_scripts setup_experience_scripts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.setup_experience_scripts
    ADD CONSTRAINT setup_experience_scripts_pkey PRIMARY KEY (id);


--
-- Name: setup_experience_software_installers setup_experience_software_installers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.setup_experience_software_installers
    ADD CONSTRAINT setup_experience_software_installers_pkey PRIMARY KEY (software_installer_id, platform);


--
-- Name: setup_experience_status_results setup_experience_status_results_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.setup_experience_status_results
    ADD CONSTRAINT setup_experience_status_results_pkey PRIMARY KEY (id);


--
-- Name: software_categories software_categories_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.software_categories
    ADD CONSTRAINT software_categories_pkey PRIMARY KEY (id);


--
-- Name: software_cpe software_cpe_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.software_cpe
    ADD CONSTRAINT software_cpe_pkey PRIMARY KEY (id);


--
-- Name: software_cve software_cve_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.software_cve
    ADD CONSTRAINT software_cve_pkey PRIMARY KEY (id);


--
-- Name: software_host_counts software_host_counts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.software_host_counts
    ADD CONSTRAINT software_host_counts_pkey PRIMARY KEY (software_id, team_id, global_stats);


--
-- Name: software_install_upcoming_activities software_install_upcoming_activities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.software_install_upcoming_activities
    ADD CONSTRAINT software_install_upcoming_activities_pkey PRIMARY KEY (upcoming_activity_id);


--
-- Name: software_installer_labels software_installer_labels_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.software_installer_labels
    ADD CONSTRAINT software_installer_labels_pkey PRIMARY KEY (id);


--
-- Name: software_installer_software_categories software_installer_software_categories_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.software_installer_software_categories
    ADD CONSTRAINT software_installer_software_categories_pkey PRIMARY KEY (id);


--
-- Name: software_installers software_installers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.software_installers
    ADD CONSTRAINT software_installers_pkey PRIMARY KEY (id);


--
-- Name: software software_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.software
    ADD CONSTRAINT software_pkey PRIMARY KEY (id);


--
-- Name: software_title_display_names software_title_display_names_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.software_title_display_names
    ADD CONSTRAINT software_title_display_names_pkey PRIMARY KEY (id);


--
-- Name: software_title_icons software_title_icons_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.software_title_icons
    ADD CONSTRAINT software_title_icons_pkey PRIMARY KEY (id);


--
-- Name: software_title_team_pins software_title_team_pins_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.software_title_team_pins
    ADD CONSTRAINT software_title_team_pins_pkey PRIMARY KEY (team_id, title_id);


--
-- Name: software_titles_host_counts software_titles_host_counts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.software_titles_host_counts
    ADD CONSTRAINT software_titles_host_counts_pkey PRIMARY KEY (software_title_id, team_id, global_stats);


--
-- Name: software_titles software_titles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.software_titles
    ADD CONSTRAINT software_titles_pkey PRIMARY KEY (id);


--
-- Name: software_update_schedules software_update_schedules_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.software_update_schedules
    ADD CONSTRAINT software_update_schedules_pkey PRIMARY KEY (id);


--
-- Name: statistics statistics_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.statistics
    ADD CONSTRAINT statistics_pkey PRIMARY KEY (id);


--
-- Name: teams teams_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.teams
    ADD CONSTRAINT teams_pkey PRIMARY KEY (id);


--
-- Name: verification_tokens token; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.verification_tokens
    ADD CONSTRAINT token UNIQUE (token);


--
-- Name: trace_sampler_settings trace_sampler_settings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.trace_sampler_settings
    ADD CONSTRAINT trace_sampler_settings_pkey PRIMARY KEY (id);


--
-- Name: host_scd_data uniq_entity_bucket; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_scd_data
    ADD CONSTRAINT uniq_entity_bucket UNIQUE (dataset, entity_id, valid_from);


--
-- Name: batch_activity_host_results unique_batch_host_results_execution_hostid; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.batch_activity_host_results
    ADD CONSTRAINT unique_batch_host_results_execution_hostid UNIQUE (batch_execution_id, host_id);


--
-- Name: mdm_idp_accounts unique_idp_email; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_idp_accounts
    ADD CONSTRAINT unique_idp_email UNIQUE (email);


--
-- Name: scheduled_queries unique_names_in_packs; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scheduled_queries
    ADD CONSTRAINT unique_names_in_packs UNIQUE (name, pack_id);


--
-- Name: software_cpe unq_software_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.software_cpe
    ADD CONSTRAINT unq_software_id UNIQUE (software_id);


--
-- Name: software_cve unq_software_id_cve; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.software_cve
    ADD CONSTRAINT unq_software_id_cve UNIQUE (software_id, cve);


--
-- Name: upcoming_activities upcoming_activities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.upcoming_activities
    ADD CONSTRAINT upcoming_activities_pkey PRIMARY KEY (id);


--
-- Name: user_api_endpoints user_api_endpoints_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_api_endpoints
    ADD CONSTRAINT user_api_endpoints_pkey PRIMARY KEY (user_id, path, method);


--
-- Name: nano_enrollments user_id; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nano_enrollments
    ADD CONSTRAINT user_id UNIQUE (user_id);


--
-- Name: user_teams user_teams_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_teams
    ADD CONSTRAINT user_teams_pkey PRIMARY KEY (user_id, team_id);


--
-- Name: users_deleted users_deleted_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users_deleted
    ADD CONSTRAINT users_deleted_pkey PRIMARY KEY (id);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: verification_tokens verification_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.verification_tokens
    ADD CONSTRAINT verification_tokens_pkey PRIMARY KEY (id);


--
-- Name: vpp_app_configurations vpp_app_configurations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vpp_app_configurations
    ADD CONSTRAINT vpp_app_configurations_pkey PRIMARY KEY (id);


--
-- Name: vpp_app_team_labels vpp_app_team_labels_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vpp_app_team_labels
    ADD CONSTRAINT vpp_app_team_labels_pkey PRIMARY KEY (id);


--
-- Name: vpp_app_team_software_categories vpp_app_team_software_categories_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vpp_app_team_software_categories
    ADD CONSTRAINT vpp_app_team_software_categories_pkey PRIMARY KEY (id);


--
-- Name: vpp_app_upcoming_activities vpp_app_upcoming_activities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vpp_app_upcoming_activities
    ADD CONSTRAINT vpp_app_upcoming_activities_pkey PRIMARY KEY (upcoming_activity_id);


--
-- Name: vpp_apps vpp_apps_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vpp_apps
    ADD CONSTRAINT vpp_apps_pkey PRIMARY KEY (adam_id, platform);


--
-- Name: vpp_apps_teams vpp_apps_teams_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vpp_apps_teams
    ADD CONSTRAINT vpp_apps_teams_pkey PRIMARY KEY (id);


--
-- Name: vpp_client_users vpp_client_users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vpp_client_users
    ADD CONSTRAINT vpp_client_users_pkey PRIMARY KEY (id);


--
-- Name: vpp_token_teams vpp_token_teams_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vpp_token_teams
    ADD CONSTRAINT vpp_token_teams_pkey PRIMARY KEY (id);


--
-- Name: vpp_tokens vpp_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vpp_tokens
    ADD CONSTRAINT vpp_tokens_pkey PRIMARY KEY (id);


--
-- Name: vulnerability_host_counts vulnerability_host_counts_cve_team_id_global_stats_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vulnerability_host_counts
    ADD CONSTRAINT vulnerability_host_counts_cve_team_id_global_stats_key UNIQUE (cve, team_id, global_stats);


--
-- Name: windows_mdm_command_queue windows_mdm_command_queue_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.windows_mdm_command_queue
    ADD CONSTRAINT windows_mdm_command_queue_pkey PRIMARY KEY (enrollment_id, command_uuid);


--
-- Name: windows_mdm_command_results windows_mdm_command_results_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.windows_mdm_command_results
    ADD CONSTRAINT windows_mdm_command_results_pkey PRIMARY KEY (enrollment_id, command_uuid);


--
-- Name: windows_mdm_commands windows_mdm_commands_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.windows_mdm_commands
    ADD CONSTRAINT windows_mdm_commands_pkey PRIMARY KEY (command_uuid);


--
-- Name: windows_mdm_responses windows_mdm_responses_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.windows_mdm_responses
    ADD CONSTRAINT windows_mdm_responses_pkey PRIMARY KEY (id);


--
-- Name: wstep_cert_auth_associations wstep_cert_auth_associations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wstep_cert_auth_associations
    ADD CONSTRAINT wstep_cert_auth_associations_pkey PRIMARY KEY (id, sha256);


--
-- Name: wstep_certificates wstep_certificates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wstep_certificates
    ADD CONSTRAINT wstep_certificates_pkey PRIMARY KEY (serial);


--
-- Name: wstep_serials wstep_serials_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wstep_serials
    ADD CONSTRAINT wstep_serials_pkey PRIMARY KEY (serial);


--
-- Name: yara_rules yara_rules_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.yara_rules
    ADD CONSTRAINT yara_rules_pkey PRIMARY KEY (id);


--
-- Name: acme_account_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX acme_account_id ON public.acme_orders USING btree (acme_account_id);


--
-- Name: acme_authorization_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX acme_authorization_id ON public.acme_challenges USING btree (acme_authorization_id);


--
-- Name: acme_order_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX acme_order_id ON public.acme_authorizations USING btree (acme_order_id);


--
-- Name: activities_created_at_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX activities_created_at_idx ON public.activity_past USING btree (created_at);


--
-- Name: activities_streamed_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX activities_streamed_idx ON public.activity_past USING btree (streamed);


--
-- Name: adam_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX adam_id ON public.host_vpp_software_installs USING btree (adam_id, platform);


--
-- Name: aggregated_stats_type_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX aggregated_stats_type_idx ON public.aggregated_stats USING btree (type);


--
-- Name: author_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX author_id ON public.labels USING btree (author_id);


--
-- Name: auto_increment; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX auto_increment ON public.mdm_android_configuration_profiles USING btree (auto_increment);


--
-- Name: batch_script_executions_script_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX batch_script_executions_script_id ON public.batch_activities USING btree (script_id);


--
-- Name: calendar_event_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX calendar_event_id ON public.host_calendar_events USING btree (calendar_event_id);


--
-- Name: certificate_authority_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX certificate_authority_id ON public.certificate_templates USING btree (certificate_authority_id);


--
-- Name: command_uuid; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX command_uuid ON public.host_mdm_apple_bootstrap_packages USING btree (command_uuid);


--
-- Name: deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX deleted ON public.host_recovery_key_passwords USING btree (deleted);


--
-- Name: device_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX device_id ON public.nano_enrollments USING btree (device_id);


--
-- Name: device_request_uuid; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX device_request_uuid ON public.host_mdm_android_profiles USING btree (device_request_uuid);


--
-- Name: display_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX display_name ON public.host_display_names USING btree (display_name);


--
-- Name: enrollment_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX enrollment_id ON public.windows_mdm_responses USING btree (enrollment_id);


--
-- Name: fk_abm_tokens_ios_default_team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_abm_tokens_ios_default_team_id ON public.abm_tokens USING btree (ios_default_team_id);


--
-- Name: fk_abm_tokens_ipados_default_team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_abm_tokens_ipados_default_team_id ON public.abm_tokens USING btree (ipados_default_team_id);


--
-- Name: fk_abm_tokens_macos_default_team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_abm_tokens_macos_default_team_id ON public.abm_tokens USING btree (macos_default_team_id);


--
-- Name: fk_activities_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_activities_user_id ON public.activity_past USING btree (user_id);


--
-- Name: fk_email_changes_users; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_email_changes_users ON public.email_changes USING btree (user_id);


--
-- Name: fk_enroll_secrets_team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_enroll_secrets_team_id ON public.enroll_secrets USING btree (team_id);


--
-- Name: fk_hmlap_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_hmlap_status ON public.host_managed_local_account_passwords USING btree (status);


--
-- Name: fk_host_activities_activity_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_host_activities_activity_id ON public.activity_host_past USING btree (activity_id);


--
-- Name: fk_host_certificate_templates_operation_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_host_certificate_templates_operation_type ON public.host_certificate_templates USING btree (operation_type);


--
-- Name: fk_host_dep_assignments_abm_token_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_host_dep_assignments_abm_token_id ON public.host_dep_assignments USING btree (abm_token_id);


--
-- Name: fk_host_in_house_software_installs_in_house_app_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_host_in_house_software_installs_in_house_app_id ON public.host_in_house_software_installs USING btree (in_house_app_id);


--
-- Name: fk_host_in_house_software_installs_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_host_in_house_software_installs_user_id ON public.host_in_house_software_installs USING btree (user_id);


--
-- Name: fk_host_scim_scim_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_host_scim_scim_user_id ON public.host_scim_user USING btree (scim_user_id);


--
-- Name: fk_host_script_results_script_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_host_script_results_script_id ON public.host_script_results USING btree (script_id);


--
-- Name: fk_host_script_results_setup_experience_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_host_script_results_setup_experience_id ON public.host_script_results USING btree (setup_experience_script_id);


--
-- Name: fk_host_script_results_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_host_script_results_user_id ON public.host_script_results USING btree (user_id);


--
-- Name: fk_host_software_installs_installer_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_host_software_installs_installer_id ON public.host_software_installs USING btree (software_installer_id);


--
-- Name: fk_host_software_installs_software_title_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_host_software_installs_software_title_id ON public.host_software_installs USING btree (software_title_id);


--
-- Name: fk_host_software_installs_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_host_software_installs_user_id ON public.host_software_installs USING btree (user_id);


--
-- Name: fk_host_vpp_software_installs_policy_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_host_vpp_software_installs_policy_id ON public.host_vpp_software_installs USING btree (policy_id);


--
-- Name: fk_host_vpp_software_installs_vpp_token_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_host_vpp_software_installs_vpp_token_id ON public.host_vpp_software_installs USING btree (vpp_token_id);


--
-- Name: fk_hosts_team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_hosts_team_id ON public.hosts USING btree (team_id);


--
-- Name: fk_in_house_app_upcoming_activities_in_house_app_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_in_house_app_upcoming_activities_in_house_app_id ON public.in_house_app_upcoming_activities USING btree (in_house_app_id);


--
-- Name: fk_in_house_app_upcoming_activities_software_title_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_in_house_app_upcoming_activities_software_title_id ON public.in_house_app_upcoming_activities USING btree (software_title_id);


--
-- Name: fk_in_house_apps_title; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_in_house_apps_title ON public.in_house_apps USING btree (title_id);


--
-- Name: fk_mdm_apple_setup_assistant_profiles_abm_token_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_mdm_apple_setup_assistant_profiles_abm_token_id ON public.mdm_apple_setup_assistant_profiles USING btree (abm_token_id);


--
-- Name: fk_mdm_default_setup_assistant_abm_token_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_mdm_default_setup_assistant_abm_token_id ON public.mdm_apple_default_setup_assistants USING btree (abm_token_id);


--
-- Name: fk_mdm_default_setup_assistant_team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_mdm_default_setup_assistant_team_id ON public.mdm_apple_default_setup_assistants USING btree (team_id);


--
-- Name: fk_mdm_setup_assistant_team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_mdm_setup_assistant_team_id ON public.mdm_apple_setup_assistants USING btree (team_id);


--
-- Name: fk_nano_devices_team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_nano_devices_team_id ON public.nano_devices USING btree (enroll_team_id);


--
-- Name: fk_patch_software_title_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_patch_software_title_id ON public.policies USING btree (patch_software_title_id);


--
-- Name: fk_policies_script_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_policies_script_id ON public.policies USING btree (script_id);


--
-- Name: fk_policies_software_installer_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_policies_software_installer_id ON public.policies USING btree (software_installer_id);


--
-- Name: fk_policies_vpp_apps_team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_policies_vpp_apps_team_id ON public.policies USING btree (vpp_apps_teams_id);


--
-- Name: fk_scheduled_queries_queries; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_scheduled_queries_queries ON public.scheduled_queries USING btree (team_id_char, query_name);


--
-- Name: fk_scim_user_emails_scim_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_scim_user_emails_scim_user_id ON public.scim_user_emails USING btree (scim_user_id);


--
-- Name: fk_scim_user_group_group_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_scim_user_group_group_id ON public.scim_user_group USING btree (group_id);


--
-- Name: fk_script_result_policy_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_script_result_policy_id ON public.host_script_results USING btree (policy_id);


--
-- Name: fk_script_upcoming_activities_policy_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_script_upcoming_activities_policy_id ON public.script_upcoming_activities USING btree (policy_id);


--
-- Name: fk_script_upcoming_activities_script_content_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_script_upcoming_activities_script_content_id ON public.script_upcoming_activities USING btree (script_content_id);


--
-- Name: fk_script_upcoming_activities_script_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_script_upcoming_activities_script_id ON public.script_upcoming_activities USING btree (script_id);


--
-- Name: fk_script_upcoming_activities_setup_experience_script_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_script_upcoming_activities_setup_experience_script_id ON public.script_upcoming_activities USING btree (setup_experience_script_id);


--
-- Name: fk_setup_experience_scripts_ibfk_1; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_setup_experience_scripts_ibfk_1 ON public.setup_experience_scripts USING btree (team_id);


--
-- Name: fk_setup_experience_status_results_ses_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_setup_experience_status_results_ses_id ON public.setup_experience_status_results USING btree (setup_experience_script_id);


--
-- Name: fk_setup_experience_status_results_si_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_setup_experience_status_results_si_id ON public.setup_experience_status_results USING btree (software_installer_id);


--
-- Name: fk_setup_experience_status_results_va_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_setup_experience_status_results_va_id ON public.setup_experience_status_results USING btree (vpp_app_team_id);


--
-- Name: fk_software_install_policy_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_software_install_policy_id ON public.host_software_installs USING btree (policy_id);


--
-- Name: fk_software_install_upcoming_activities_policy_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_software_install_upcoming_activities_policy_id ON public.software_install_upcoming_activities USING btree (policy_id);


--
-- Name: fk_software_install_upcoming_activities_software_installer_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_software_install_upcoming_activities_software_installer_id ON public.software_install_upcoming_activities USING btree (software_installer_id);


--
-- Name: fk_software_install_upcoming_activities_software_title_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_software_install_upcoming_activities_software_title_id ON public.software_install_upcoming_activities USING btree (software_title_id);


--
-- Name: fk_software_installers_fleet_library_app_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_software_installers_fleet_library_app_id ON public.software_installers USING btree (fleet_maintained_app_id);


--
-- Name: fk_software_installers_install_script_content_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_software_installers_install_script_content_id ON public.software_installers USING btree (install_script_content_id);


--
-- Name: fk_software_installers_post_install_script_content_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_software_installers_post_install_script_content_id ON public.software_installers USING btree (post_install_script_content_id);


--
-- Name: fk_software_installers_team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_software_installers_team_id ON public.software_installers USING btree (team_id);


--
-- Name: fk_software_installers_title; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_software_installers_title ON public.software_installers USING btree (title_id);


--
-- Name: fk_software_installers_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_software_installers_user_id ON public.software_installers USING btree (user_id);


--
-- Name: fk_team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_team_id ON public.invite_teams USING btree (team_id);


--
-- Name: fk_uninstall_script_content_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_uninstall_script_content_id ON public.software_installers USING btree (uninstall_script_content_id);


--
-- Name: fk_upcoming_activities_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_upcoming_activities_user_id ON public.upcoming_activities USING btree (user_id);


--
-- Name: fk_user_teams_team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_user_teams_team_id ON public.user_teams USING btree (team_id);


--
-- Name: fk_vpp_app_configurations_app; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_vpp_app_configurations_app ON public.vpp_app_configurations USING btree (application_id, platform);


--
-- Name: fk_vpp_app_upcoming_activities_adam_id_platform; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_vpp_app_upcoming_activities_adam_id_platform ON public.vpp_app_upcoming_activities USING btree (adam_id, platform);


--
-- Name: fk_vpp_app_upcoming_activities_policy_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_vpp_app_upcoming_activities_policy_id ON public.vpp_app_upcoming_activities USING btree (policy_id);


--
-- Name: fk_vpp_app_upcoming_activities_vpp_token_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_vpp_app_upcoming_activities_vpp_token_id ON public.vpp_app_upcoming_activities USING btree (vpp_token_id);


--
-- Name: fk_vpp_apps_teams_vpp_token_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_vpp_apps_teams_vpp_token_id ON public.vpp_apps_teams USING btree (vpp_token_id);


--
-- Name: fk_vpp_apps_title; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_vpp_apps_title ON public.vpp_apps USING btree (title_id);


--
-- Name: fk_vpp_token_teams_vpp_token_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fk_vpp_token_teams_vpp_token_id ON public.vpp_token_teams USING btree (vpp_token_id);


--
-- Name: host_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX host_id ON public.carve_metadata USING btree (host_id);


--
-- Name: host_id_software_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX host_id_software_id_idx ON public.host_software_installed_paths USING btree (host_id, software_id);


--
-- Name: host_mdm_enrolled_installed_from_dep_is_personal_enrollment_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX host_mdm_enrolled_installed_from_dep_is_personal_enrollment_idx ON public.host_mdm USING btree (enrolled, installed_from_dep, is_personal_enrollment);


--
-- Name: host_mdm_mdm_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX host_mdm_mdm_id_idx ON public.host_mdm USING btree (mdm_id);


--
-- Name: hosts_platform_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX hosts_platform_idx ON public.hosts USING btree (platform);


--
-- Name: idx_abm_tokens_byod_default_team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_abm_tokens_byod_default_team_id ON public.abm_tokens USING btree (byod_default_team_id);


--
-- Name: idx_activities_activity_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_activities_activity_type ON public.activity_past USING btree (activity_type);


--
-- Name: idx_activities_type_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_activities_type_created ON public.activity_past USING btree (activity_type, created_at);


--
-- Name: idx_activities_user_email; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_activities_user_email ON public.activity_past USING btree (user_email);


--
-- Name: idx_activities_user_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_activities_user_name ON public.activity_past USING btree (user_name);


--
-- Name: idx_aggregated_stats_updated_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_aggregated_stats_updated_at ON public.aggregated_stats USING btree (updated_at);


--
-- Name: idx_android_devices_team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_android_devices_team_id ON public.android_devices USING btree (team_id);


--
-- Name: idx_auto_rotate_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_auto_rotate_at ON public.host_recovery_key_passwords USING btree (auto_rotate_at);


--
-- Name: idx_batch_activities_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_batch_activities_status ON public.batch_activities USING btree (status);


--
-- Name: idx_batch_script_execution_host_result_execution_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_batch_script_execution_host_result_execution_id ON public.batch_activity_host_results USING btree (batch_execution_id);


--
-- Name: idx_conditional_access_host_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_conditional_access_host_id ON public.conditional_access_scep_certificates USING btree (host_id);


--
-- Name: idx_cron_stats_name_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cron_stats_name_created_at ON public.cron_stats USING btree (name, created_at);


--
-- Name: idx_cve_meta_cvss_score; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cve_meta_cvss_score ON public.cve_meta USING btree (cvss_score, cve);


--
-- Name: idx_cve_meta_exploit; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cve_meta_exploit ON public.cve_meta USING btree (cisa_known_exploit, cve);


--
-- Name: idx_dataset_range; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_dataset_range ON public.host_scd_data USING btree (dataset, valid_from, valid_to);


--
-- Name: idx_distributed_query_campaign_targets_campaign_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_distributed_query_campaign_targets_campaign_id ON public.distributed_query_campaign_targets USING btree (distributed_query_campaign_id);


--
-- Name: idx_hdep_hardware_serial; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_hdep_hardware_serial ON public.host_dep_assignments USING btree (hardware_serial);


--
-- Name: idx_hdep_response; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_hdep_response ON public.host_dep_assignments USING btree (assign_profile_response, response_updated_at);


--
-- Name: idx_hmlap_auto_rotate_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_hmlap_auto_rotate_at ON public.host_managed_local_account_passwords USING btree (auto_rotate_at);


--
-- Name: idx_hmlap_command_uuid; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_hmlap_command_uuid ON public.host_managed_local_account_passwords USING btree (command_uuid);


--
-- Name: idx_host_certificate_templates_not_valid_after; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_certificate_templates_not_valid_after ON public.host_certificate_templates USING btree (not_valid_after);


--
-- Name: idx_host_certs_hid_cn; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_certs_hid_cn ON public.host_certificates USING btree (host_id, common_name);


--
-- Name: idx_host_certs_not_valid_after; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_certs_not_valid_after ON public.host_certificates USING btree (host_id, not_valid_after);


--
-- Name: idx_host_certs_origin_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_certs_origin_deleted ON public.host_certificates USING btree (origin, deleted_at);


--
-- Name: idx_host_custom_host_vitals_custom_host_vital_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_custom_host_vitals_custom_host_vital_id ON public.host_custom_host_vitals USING btree (custom_host_vital_id);


--
-- Name: idx_host_device_auth_previous_token; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_device_auth_previous_token ON public.host_device_auth USING btree (previous_token);


--
-- Name: idx_host_disk_encryption_keys_archive_host_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_disk_encryption_keys_archive_host_created_at ON public.host_disk_encryption_keys_archive USING btree (host_id, created_at DESC);


--
-- Name: idx_host_disk_encryption_keys_decryptable; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_disk_encryption_keys_decryptable ON public.host_disk_encryption_keys USING btree (decryptable);


--
-- Name: idx_host_disks_gigs_disk_space_available; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_disks_gigs_disk_space_available ON public.host_disks USING btree (gigs_disk_space_available);


--
-- Name: idx_host_emails_email; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_emails_email ON public.host_emails USING btree (email);


--
-- Name: idx_host_emails_host_id_email; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_emails_host_id_email ON public.host_emails USING btree (host_id, email);


--
-- Name: idx_host_id_scep_host_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_id_scep_host_id ON public.host_identity_scep_certificates USING btree (host_id);


--
-- Name: idx_host_id_scep_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_id_scep_name ON public.host_identity_scep_certificates USING btree (name);


--
-- Name: idx_host_in_house_software_installs_verification; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_in_house_software_installs_verification ON public.host_in_house_software_installs USING btree ((((verification_at IS NULL) AND (verification_failed_at IS NULL))));


--
-- Name: idx_host_mdm_apple_declarations_operation_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_mdm_apple_declarations_operation_type ON public.host_mdm_apple_declarations USING btree (operation_type);


--
-- Name: idx_host_mdm_apple_declarations_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_mdm_apple_declarations_status ON public.host_mdm_apple_declarations USING btree (status);


--
-- Name: idx_host_mdm_apple_declarations_token; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_mdm_apple_declarations_token ON public.host_mdm_apple_declarations USING btree (token);


--
-- Name: idx_host_mdm_apple_device_names_command_uuid; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_mdm_apple_device_names_command_uuid ON public.host_mdm_apple_device_names USING btree (command_uuid);


--
-- Name: idx_host_mdm_apple_device_names_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_mdm_apple_device_names_status ON public.host_mdm_apple_device_names USING btree (status);


--
-- Name: idx_host_mdm_apple_profiles_operation_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_mdm_apple_profiles_operation_type ON public.host_mdm_apple_profiles USING btree (operation_type);


--
-- Name: idx_host_mdm_apple_profiles_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_mdm_apple_profiles_status ON public.host_mdm_apple_profiles USING btree (status);


--
-- Name: idx_host_mdm_windows_profiles_operation_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_mdm_windows_profiles_operation_type ON public.host_mdm_windows_profiles USING btree (operation_type);


--
-- Name: idx_host_mdm_windows_profiles_profile_uuid_checksum; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_mdm_windows_profiles_profile_uuid_checksum ON public.host_mdm_windows_profiles USING btree (profile_uuid, checksum);


--
-- Name: idx_host_mdm_windows_profiles_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_mdm_windows_profiles_status ON public.host_mdm_windows_profiles USING btree (status);


--
-- Name: idx_host_mdm_windows_profiles_status_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_mdm_windows_profiles_status_status ON public.host_mdm_windows_profiles_status USING btree (status);


--
-- Name: idx_host_operating_system_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_operating_system_id ON public.host_operating_system USING btree (os_id);


--
-- Name: idx_host_orbit_info_version; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_orbit_info_version ON public.host_orbit_info USING btree (version);


--
-- Name: idx_host_recovery_key_passwords_operation_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_recovery_key_passwords_operation_type ON public.host_recovery_key_passwords USING btree (operation_type);


--
-- Name: idx_host_recovery_key_passwords_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_recovery_key_passwords_status ON public.host_recovery_key_passwords USING btree (status);


--
-- Name: idx_host_script_canceled_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_script_canceled_created_at ON public.host_script_results USING btree (host_id, script_id, canceled, created_at DESC);


--
-- Name: idx_host_script_results_host_exit_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_script_results_host_exit_created ON public.host_script_results USING btree (host_id, exit_code, created_at);


--
-- Name: idx_host_script_results_host_policy; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_script_results_host_policy ON public.host_script_results USING btree (host_id, policy_id);


--
-- Name: idx_host_seen_times_seen_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_seen_times_seen_time ON public.host_seen_times USING btree (seen_time);


--
-- Name: idx_host_software_installs_host_installer; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_software_installs_host_installer ON public.host_software_installs USING btree (host_id, software_installer_id);


--
-- Name: idx_host_software_installs_host_policy; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_software_installs_host_policy ON public.host_software_installs USING btree (host_id, policy_id);


--
-- Name: idx_host_software_software_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_software_software_id ON public.host_software USING btree (software_id);


--
-- Name: idx_host_vpp_software_installs_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_vpp_software_installs_user_id ON public.host_vpp_software_installs USING btree (user_id);


--
-- Name: idx_host_vpp_software_installs_verification; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_host_vpp_software_installs_verification ON public.host_vpp_software_installs USING btree ((((verification_at IS NULL) AND (verification_failed_at IS NULL))));


--
-- Name: idx_hosts_hardware_serial; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_hosts_hardware_serial ON public.hosts USING btree (hardware_serial);


--
-- Name: idx_hosts_hostname; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_hosts_hostname ON public.hosts USING btree (hostname);


--
-- Name: idx_hosts_uuid; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_hosts_uuid ON public.hosts USING btree (uuid);


--
-- Name: idx_in_house_app_install_tokens_expires_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_in_house_app_install_tokens_expires_at ON public.in_house_app_install_tokens USING btree (expires_at);


--
-- Name: idx_jobs_name_state; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_jobs_name_state ON public.jobs USING btree (name, state);


--
-- Name: idx_jobs_state_not_before_updated_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_jobs_state_not_before_updated_at ON public.jobs USING btree (state, not_before, updated_at);


--
-- Name: idx_labels_team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_labels_team_id ON public.labels USING btree (team_id);


--
-- Name: idx_legacy_enroll_refs_host_uuid; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_legacy_enroll_refs_host_uuid ON public.legacy_host_mdm_enroll_refs USING btree (host_uuid);


--
-- Name: idx_lm_label_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_lm_label_id ON public.label_membership USING btree (label_id);


--
-- Name: idx_mdm_adue_enrollment_challenges_abm_token_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mdm_adue_enrollment_challenges_abm_token_id ON public.mdm_adue_enrollment_challenges USING btree (abm_token_id);


--
-- Name: idx_mdm_adue_enrollment_challenges_idp_account_uuid; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mdm_adue_enrollment_challenges_idp_account_uuid ON public.mdm_adue_enrollment_challenges USING btree (idp_account_uuid);


--
-- Name: idx_mdm_adue_expires; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mdm_adue_expires ON public.mdm_adue_enrollment_challenges USING btree (expires_at);


--
-- Name: idx_mdm_android_commands_host_uuid; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mdm_android_commands_host_uuid ON public.mdm_android_commands USING btree (host_uuid);


--
-- Name: idx_mdm_android_commands_operation_name; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_mdm_android_commands_operation_name ON public.mdm_android_commands USING btree (operation_name);


--
-- Name: idx_mdm_apple_declaration_asset_refs_asset_uuid; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mdm_apple_declaration_asset_refs_asset_uuid ON public.mdm_apple_declaration_asset_references USING btree (asset_uuid);


--
-- Name: idx_mdm_apple_psso_keys_host_uuid; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mdm_apple_psso_keys_host_uuid ON public.mdm_apple_psso_keys USING btree (host_uuid);


--
-- Name: idx_mdm_config_profile_vars_apple_decl_variable; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_mdm_config_profile_vars_apple_decl_variable ON public.mdm_configuration_profile_variables USING btree (apple_declaration_uuid, fleet_variable_id);


--
-- Name: idx_mdm_configuration_profile_labels_label_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mdm_configuration_profile_labels_label_id ON public.mdm_configuration_profile_labels USING btree (label_id);


--
-- Name: idx_mdm_configuration_profile_variables_android_variable; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_mdm_configuration_profile_variables_android_variable ON public.mdm_configuration_profile_variables USING btree (android_profile_uuid, fleet_variable_id);


--
-- Name: idx_mdm_configuration_profile_variables_app_config_variable; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_mdm_configuration_profile_variables_app_config_variable ON public.mdm_configuration_profile_variables USING btree (android_app_configuration_id, fleet_variable_id);


--
-- Name: idx_mdm_configuration_profile_variables_cert_template_variable; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_mdm_configuration_profile_variables_cert_template_variable ON public.mdm_configuration_profile_variables USING btree (certificate_template_id, fleet_variable_id);


--
-- Name: idx_mdm_declaration_labels_label_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mdm_declaration_labels_label_id ON public.mdm_declaration_labels USING btree (label_id);


--
-- Name: idx_mdm_windows_enrollments_host_uuid; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mdm_windows_enrollments_host_uuid ON public.mdm_windows_enrollments USING btree (host_uuid);


--
-- Name: idx_mdm_windows_enrollments_mdm_device_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mdm_windows_enrollments_mdm_device_id ON public.mdm_windows_enrollments USING btree (mdm_device_id);


--
-- Name: idx_nano_command_results_command_uuid; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_nano_command_results_command_uuid ON public.nano_command_results USING btree (command_uuid);


--
-- Name: idx_nano_command_results_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_nano_command_results_status ON public.nano_command_results USING btree (status);


--
-- Name: idx_nano_enrollment_queue_command_uuid; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_nano_enrollment_queue_command_uuid ON public.nano_enrollment_queue USING btree (command_uuid);


--
-- Name: idx_nano_users_device_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_nano_users_device_id ON public.nano_users USING btree (device_id);


--
-- Name: idx_ncr_lookup; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ncr_lookup ON public.nano_command_results USING btree (id, command_uuid, status);


--
-- Name: idx_neq_filter; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_neq_filter ON public.nano_enrollment_queue USING btree (active, priority, created_at);


--
-- Name: idx_neq_next_command; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_neq_next_command ON public.nano_enrollment_queue USING btree (id, active, priority DESC, created_at);


--
-- Name: idx_network_interfaces_hosts_fk; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_network_interfaces_hosts_fk ON public.network_interfaces USING btree (host_id);


--
-- Name: idx_os_version_vulnerabilities_os_version_team_cve; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_os_version_vulnerabilities_os_version_team_cve ON public.operating_system_version_vulnerabilities USING btree (team_id, os_version_id, cve);


--
-- Name: idx_os_version_vulnerabilities_unq_os_version_team_cve2; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_os_version_vulnerabilities_unq_os_version_team_cve2 ON public.operating_system_version_vulnerabilities USING btree (COALESCE(team_id, '-1'::integer), os_version_id, cve);


--
-- Name: idx_os_version_vulnerabilities_updated_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_os_version_vulnerabilities_updated_at ON public.operating_system_version_vulnerabilities USING btree (updated_at);


--
-- Name: idx_os_vulnerabilities_cve; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_os_vulnerabilities_cve ON public.operating_system_vulnerabilities USING btree (cve);


--
-- Name: idx_policies_author_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_policies_author_id ON public.policies USING btree (author_id);


--
-- Name: idx_policies_needs_full_membership_cleanup; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_policies_needs_full_membership_cleanup ON public.policies USING btree (needs_full_membership_cleanup);


--
-- Name: idx_policies_team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_policies_team_id ON public.policies USING btree (team_id);


--
-- Name: idx_policy_membership_host_id_passes; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_policy_membership_host_id_passes ON public.policy_membership USING btree (host_id, passes);


--
-- Name: idx_policy_membership_passes; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_policy_membership_passes ON public.policy_membership USING btree (passes);


--
-- Name: idx_queries_author_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_queries_author_id ON public.queries USING btree (author_id);


--
-- Name: idx_queries_schedule_automations; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_queries_schedule_automations ON public.queries USING btree (is_scheduled, automations_enabled);


--
-- Name: idx_query_id_has_data_host_id_last_fetched; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_query_id_has_data_host_id_last_fetched ON public.query_results USING btree (query_id, has_data, host_id, last_fetched);


--
-- Name: idx_query_id_host_id_last_fetched; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_query_id_host_id_last_fetched ON public.query_results USING btree (query_id, host_id, last_fetched);


--
-- Name: idx_scim_groups_external_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_scim_groups_external_id ON public.scim_groups USING btree (external_id);


--
-- Name: idx_scim_user_emails_email_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_scim_user_emails_email_type ON public.scim_user_emails USING btree (type, email);


--
-- Name: idx_scim_users_external_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_scim_users_external_id ON public.scim_users USING btree (external_id);


--
-- Name: idx_script_content_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_script_content_id ON public.setup_experience_scripts USING btree (script_content_id);


--
-- Name: idx_scripts_script_content_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_scripts_script_content_id ON public.scripts USING btree (script_content_id);


--
-- Name: idx_seti_team_platform; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_seti_team_platform ON public.setup_experience_software_installers USING btree (global_or_team_id, platform);


--
-- Name: idx_setup_experience_scripts_host_uuid; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_setup_experience_scripts_host_uuid ON public.setup_experience_status_results USING btree (host_uuid);


--
-- Name: idx_setup_experience_scripts_hsi_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_setup_experience_scripts_hsi_id ON public.setup_experience_status_results USING btree (host_software_installs_execution_id);


--
-- Name: idx_setup_experience_scripts_nano_command_uuid; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_setup_experience_scripts_nano_command_uuid ON public.setup_experience_status_results USING btree (nano_command_uuid);


--
-- Name: idx_setup_experience_scripts_script_execution_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_setup_experience_scripts_script_execution_id ON public.setup_experience_status_results USING btree (script_execution_id);


--
-- Name: idx_software_bundle_identifier; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_software_bundle_identifier ON public.software USING btree (bundle_identifier);


--
-- Name: idx_software_cve_cve; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_software_cve_cve ON public.software_cve USING btree (cve);


--
-- Name: idx_software_installer_labels_label_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_software_installer_labels_label_id ON public.software_installer_labels USING btree (label_id);


--
-- Name: idx_software_installers_dedup; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_software_installers_dedup ON public.software_installers USING btree (global_or_team_id, title_id, dedup_token);


--
-- Name: idx_software_installers_global_or_team_id_url; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_software_installers_global_or_team_id_url ON public.software_installers USING btree (global_or_team_id, "left"((url)::text, 255));


--
-- Name: idx_software_title_display_names_software_title_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_software_title_display_names_software_title_id ON public.software_title_display_names USING btree (software_title_id);


--
-- Name: idx_software_title_icons_software_title_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_software_title_icons_software_title_id ON public.software_title_icons USING btree (software_title_id);


--
-- Name: idx_software_title_team_pins_title_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_software_title_team_pins_title_id ON public.software_title_team_pins USING btree (title_id);


--
-- Name: idx_software_update_schedules_title_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_software_update_schedules_title_id ON public.software_update_schedules USING btree (title_id);


--
-- Name: idx_storage_id_team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_storage_id_team_id ON public.software_title_icons USING btree (storage_id, team_id);


--
-- Name: idx_sw_name_source_browser; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sw_name_source_browser ON public.software USING btree (name, source, extension_for);


--
-- Name: idx_sw_titles; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sw_titles ON public.software_titles USING btree (name, source, extension_for);


--
-- Name: idx_team_id_patch_software_title_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_team_id_patch_software_title_id ON public.policies USING btree (team_id, patch_software_title_id);


--
-- Name: idx_team_id_saved_auto_interval; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_team_id_saved_auto_interval ON public.queries USING btree (team_id, saved, automations_enabled, schedule_interval);


--
-- Name: idx_type; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_type ON public.mdm_apple_enrollment_profiles USING btree (type);


--
-- Name: idx_upcoming_activities_host_id_activity_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_upcoming_activities_host_id_activity_type ON public.upcoming_activities USING btree (activity_type, host_id);


--
-- Name: idx_upcoming_activities_host_id_priority_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_upcoming_activities_host_id_priority_created_at ON public.upcoming_activities USING btree (host_id, priority, created_at);


--
-- Name: idx_users_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_name ON public.users USING btree (name);


--
-- Name: idx_valid_to_dataset; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_valid_to_dataset ON public.host_scd_data USING btree (valid_to, dataset, entity_id);


--
-- Name: idx_vhc_scope_cve; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_vhc_scope_cve ON public.vulnerability_host_counts USING btree (global_stats, team_id, host_count, cve);


--
-- Name: idx_vpp_app_team_labels_label_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_vpp_app_team_labels_label_id ON public.vpp_app_team_labels USING btree (label_id);


--
-- Name: idx_vpp_app_team_sw_categories_software_category_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_vpp_app_team_sw_categories_software_category_id ON public.vpp_app_team_software_categories USING btree (software_category_id);


--
-- Name: idx_vpp_apps_teams_adam_id_platform; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_vpp_apps_teams_adam_id_platform ON public.vpp_apps_teams USING btree (adam_id, platform);


--
-- Name: idx_vpp_apps_teams_team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_vpp_apps_teams_team_id ON public.vpp_apps_teams USING btree (team_id);


--
-- Name: idx_win_mdm_cmd_queue_acked; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_win_mdm_cmd_queue_acked ON public.windows_mdm_command_queue USING btree (acked_at);


--
-- Name: idx_win_mdm_cmd_queue_enrollment_acked; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_win_mdm_cmd_queue_enrollment_acked ON public.windows_mdm_command_queue USING btree (enrollment_id, acked_at);


--
-- Name: idx_windows_mdm_command_queue_command_uuid; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_windows_mdm_command_queue_command_uuid ON public.windows_mdm_command_queue USING btree (command_uuid);


--
-- Name: idx_windows_mdm_command_results_command_uuid; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_windows_mdm_command_results_command_uuid ON public.windows_mdm_command_results USING btree (command_uuid);


--
-- Name: in_house_app_software_categories_ibfk_2; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX in_house_app_software_categories_ibfk_2 ON public.in_house_app_software_categories USING btree (software_category_id);


--
-- Name: kernel_host_counts_os_version_id_software_id_hosts_cou_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX kernel_host_counts_os_version_id_software_id_hosts_cou_idx ON public.kernel_host_counts USING btree (os_version_id, software_id, hosts_count);


--
-- Name: kernel_host_counts_os_version_id_team_id_software_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX kernel_host_counts_os_version_id_team_id_software_id_idx ON public.kernel_host_counts USING btree (os_version_id, team_id, software_id);


--
-- Name: kernel_host_counts_software_title_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX kernel_host_counts_software_title_id_idx ON public.kernel_host_counts USING btree (software_title_id);


--
-- Name: label_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX label_id ON public.in_house_app_labels USING btree (label_id);


--
-- Name: mdm_apple_declarative_requests_enrollment_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX mdm_apple_declarative_requests_enrollment_id ON public.mdm_apple_declarative_requests USING btree (enrollment_id);


--
-- Name: mdm_configuration_profile_variables_fleet_variable_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX mdm_configuration_profile_variables_fleet_variable_id ON public.mdm_configuration_profile_variables USING btree (fleet_variable_id);


--
-- Name: operation_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX operation_type ON public.host_mdm_android_profiles USING btree (operation_type);


--
-- Name: policy_labels_label_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX policy_labels_label_id ON public.policy_labels USING btree (label_id);


--
-- Name: policy_request_uuid; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX policy_request_uuid ON public.host_mdm_android_profiles USING btree (policy_request_uuid);


--
-- Name: priority; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX priority ON public.nano_enrollment_queue USING btree (priority DESC, created_at);


--
-- Name: query_labels_label_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX query_labels_label_id ON public.query_labels USING btree (label_id);


--
-- Name: reference; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX reference ON public.mdm_apple_declaration_activation_references USING btree (reference);


--
-- Name: renew_command_uuid_fk; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX renew_command_uuid_fk ON public.nano_cert_auth_associations USING btree (renew_command_uuid);


--
-- Name: response_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX response_id ON public.windows_mdm_command_results USING btree (response_id);


--
-- Name: scheduled_queries_pack_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX scheduled_queries_pack_id ON public.scheduled_queries USING btree (pack_id);


--
-- Name: scheduled_queries_query_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX scheduled_queries_query_name ON public.scheduled_queries USING btree (query_name);


--
-- Name: scheduled_query_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX scheduled_query_id ON public.scheduled_query_stats USING btree (scheduled_query_id);


--
-- Name: script_content_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX script_content_id ON public.host_script_results USING btree (script_content_id);


--
-- Name: serial_number; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX serial_number ON public.nano_devices USING btree (serial_number);


--
-- Name: software_category_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX software_category_id ON public.software_installer_software_categories USING btree (software_category_id);


--
-- Name: software_cpe_cpe_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX software_cpe_cpe_idx ON public.software_cpe USING btree (cpe);


--
-- Name: software_host_counts_team_id_global_stats_hosts_count__idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX software_host_counts_team_id_global_stats_hosts_count__idx ON public.software_host_counts USING btree (team_id, global_stats, hosts_count DESC, software_id);


--
-- Name: software_host_counts_updated_at_software_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX software_host_counts_updated_at_software_id_idx ON public.software_host_counts USING btree (updated_at, software_id);


--
-- Name: software_listing_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX software_listing_idx ON public.software USING btree (name);


--
-- Name: software_source_vendor_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX software_source_vendor_idx ON public.software USING btree (source, vendor_old);


--
-- Name: software_titles_host_counts_s_team_id_global_stats_hosts_co_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX software_titles_host_counts_s_team_id_global_stats_hosts_co_idx ON public.software_titles_host_counts USING btree (team_id, global_stats, hosts_count, software_title_id);


--
-- Name: software_titles_host_counts_sw_updated_at_software_title_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX software_titles_host_counts_sw_updated_at_software_title_id_idx ON public.software_titles_host_counts USING btree (updated_at, software_title_id);


--
-- Name: status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX status ON public.host_mdm_android_profiles USING btree (status);


--
-- Name: team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX team_id ON public.android_app_configurations USING btree (team_id);


--
-- Name: title_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX title_id ON public.software USING btree (title_id);


--
-- Name: total_issues_count; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX total_issues_count ON public.host_issues USING btree (total_issues_count);


--
-- Name: type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX type ON public.nano_enrollments USING btree (type);


--
-- Name: verification_tokens_users; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX verification_tokens_users ON public.verification_tokens USING btree (user_id);


--
-- Name: vulnerability_host_counts_cve_team_id_global_stats_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX vulnerability_host_counts_cve_team_id_global_stats_idx ON public.vulnerability_host_counts USING btree (cve, team_id, global_stats);


--
-- Name: abm_tokens abm_tokens_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER abm_tokens_set_updated_at BEFORE UPDATE ON public.abm_tokens FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: acme_accounts acme_accounts_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER acme_accounts_set_updated_at BEFORE UPDATE ON public.acme_accounts FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: acme_authorizations acme_authorizations_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER acme_authorizations_set_updated_at BEFORE UPDATE ON public.acme_authorizations FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: acme_challenges acme_challenges_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER acme_challenges_set_updated_at BEFORE UPDATE ON public.acme_challenges FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: acme_enrollments acme_enrollments_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER acme_enrollments_set_updated_at BEFORE UPDATE ON public.acme_enrollments FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: acme_orders acme_orders_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER acme_orders_set_updated_at BEFORE UPDATE ON public.acme_orders FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: aggregated_stats aggregated_stats_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER aggregated_stats_set_updated_at BEFORE UPDATE ON public.aggregated_stats FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: android_app_configurations android_app_configurations_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER android_app_configurations_set_updated_at BEFORE UPDATE ON public.android_app_configurations FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: android_devices android_devices_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER android_devices_set_updated_at BEFORE UPDATE ON public.android_devices FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: android_enterprises android_enterprises_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER android_enterprises_set_updated_at BEFORE UPDATE ON public.android_enterprises FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: android_policy_requests android_policy_requests_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER android_policy_requests_set_updated_at BEFORE UPDATE ON public.android_policy_requests FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: app_config_json app_config_json_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER app_config_json_set_updated_at BEFORE UPDATE ON public.app_config_json FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: batch_activities batch_activities_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER batch_activities_set_updated_at BEFORE UPDATE ON public.batch_activities FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: batch_activity_host_results batch_activity_host_results_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER batch_activity_host_results_set_updated_at BEFORE UPDATE ON public.batch_activity_host_results FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: ca_config_assets ca_config_assets_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER ca_config_assets_set_updated_at BEFORE UPDATE ON public.ca_config_assets FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: calendar_events calendar_events_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER calendar_events_set_updated_at BEFORE UPDATE ON public.calendar_events FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: calendar_events calendar_events_uuid; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER calendar_events_uuid BEFORE INSERT OR UPDATE ON public.calendar_events FOR EACH ROW EXECUTE FUNCTION public.calendar_events_set_uuid();


--
-- Name: certificate_authorities certificate_authorities_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER certificate_authorities_set_updated_at BEFORE UPDATE ON public.certificate_authorities FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: certificate_templates certificate_templates_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER certificate_templates_set_updated_at BEFORE UPDATE ON public.certificate_templates FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: challenges challenges_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER challenges_set_updated_at BEFORE UPDATE ON public.challenges FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: conditional_access_scep_certificates conditional_access_scep_certificates_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER conditional_access_scep_certificates_set_updated_at BEFORE UPDATE ON public.conditional_access_scep_certificates FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: cron_stats cron_stats_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER cron_stats_set_updated_at BEFORE UPDATE ON public.cron_stats FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: custom_host_vitals custom_host_vitals_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER custom_host_vitals_set_updated_at BEFORE UPDATE ON public.custom_host_vitals FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: default_team_config_json default_team_config_json_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER default_team_config_json_set_updated_at BEFORE UPDATE ON public.default_team_config_json FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: distributed_query_campaigns distributed_query_campaigns_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER distributed_query_campaigns_set_updated_at BEFORE UPDATE ON public.distributed_query_campaigns FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: fleet_maintained_apps fleet_maintained_apps_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER fleet_maintained_apps_set_updated_at BEFORE UPDATE ON public.fleet_maintained_apps FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: host_batteries host_batteries_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER host_batteries_set_updated_at BEFORE UPDATE ON public.host_batteries FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: host_calendar_events host_calendar_events_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER host_calendar_events_set_updated_at BEFORE UPDATE ON public.host_calendar_events FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: host_certificate_templates host_certificate_templates_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER host_certificate_templates_set_updated_at BEFORE UPDATE ON public.host_certificate_templates FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: host_conditional_access host_conditional_access_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER host_conditional_access_set_updated_at BEFORE UPDATE ON public.host_conditional_access FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: host_custom_host_vitals host_custom_host_vitals_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER host_custom_host_vitals_set_updated_at BEFORE UPDATE ON public.host_custom_host_vitals FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: host_device_auth host_device_auth_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER host_device_auth_set_updated_at BEFORE UPDATE ON public.host_device_auth FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: host_disk_encryption_keys host_disk_encryption_keys_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER host_disk_encryption_keys_set_updated_at BEFORE UPDATE ON public.host_disk_encryption_keys FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: host_disks host_disks_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER host_disks_set_updated_at BEFORE UPDATE ON public.host_disks FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: host_emails host_emails_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER host_emails_set_updated_at BEFORE UPDATE ON public.host_emails FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: host_identity_scep_certificates host_identity_scep_certificates_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER host_identity_scep_certificates_set_updated_at BEFORE UPDATE ON public.host_identity_scep_certificates FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: host_in_house_software_installs host_in_house_software_installs_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER host_in_house_software_installs_set_updated_at BEFORE UPDATE ON public.host_in_house_software_installs FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: host_issues host_issues_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER host_issues_set_updated_at BEFORE UPDATE ON public.host_issues FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: host_last_known_locations host_last_known_locations_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER host_last_known_locations_set_updated_at BEFORE UPDATE ON public.host_last_known_locations FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: host_managed_local_account_passwords host_managed_local_account_passwords_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER host_managed_local_account_passwords_set_updated_at BEFORE UPDATE ON public.host_managed_local_account_passwords FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: host_mdm_android_profiles host_mdm_android_profiles_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER host_mdm_android_profiles_set_updated_at BEFORE UPDATE ON public.host_mdm_android_profiles FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: host_mdm_apple_device_names host_mdm_apple_device_names_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER host_mdm_apple_device_names_set_updated_at BEFORE UPDATE ON public.host_mdm_apple_device_names FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: host_mdm_apple_enrollment_permissions host_mdm_apple_enrollment_permissions_set_delivered_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER host_mdm_apple_enrollment_permissions_set_delivered_at BEFORE UPDATE ON public.host_mdm_apple_enrollment_permissions FOR EACH ROW EXECUTE FUNCTION public.fleet_touch_column('delivered_at');


--
-- Name: host_mdm_apple_enrollment_permissions host_mdm_apple_enrollment_permissions_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER host_mdm_apple_enrollment_permissions_set_updated_at BEFORE UPDATE ON public.host_mdm_apple_enrollment_permissions FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: host_mdm_apple_profiles host_mdm_apple_profiles_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER host_mdm_apple_profiles_set_updated_at BEFORE UPDATE ON public.host_mdm_apple_profiles FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: host_mdm_commands host_mdm_commands_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER host_mdm_commands_set_updated_at BEFORE UPDATE ON public.host_mdm_commands FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: host_mdm host_mdm_enrollment_status; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER host_mdm_enrollment_status BEFORE INSERT OR UPDATE ON public.host_mdm FOR EACH ROW EXECUTE FUNCTION public.host_mdm_set_enrollment_status();


--
-- Name: host_mdm_idp_accounts host_mdm_idp_accounts_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER host_mdm_idp_accounts_set_updated_at BEFORE UPDATE ON public.host_mdm_idp_accounts FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: host_mdm_managed_certificates host_mdm_managed_certificates_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER host_mdm_managed_certificates_set_updated_at BEFORE UPDATE ON public.host_mdm_managed_certificates FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: host_mdm host_mdm_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER host_mdm_set_updated_at BEFORE UPDATE ON public.host_mdm FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: host_mdm_windows_profiles host_mdm_windows_profiles_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER host_mdm_windows_profiles_set_updated_at BEFORE UPDATE ON public.host_mdm_windows_profiles FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: host_mdm_windows_profiles_status host_mdm_windows_profiles_status_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER host_mdm_windows_profiles_status_set_updated_at BEFORE UPDATE ON public.host_mdm_windows_profiles_status FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: host_recovery_key_passwords host_recovery_key_passwords_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER host_recovery_key_passwords_set_updated_at BEFORE UPDATE ON public.host_recovery_key_passwords FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: host_scd_data host_scd_data_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER host_scd_data_set_updated_at BEFORE UPDATE ON public.host_scd_data FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: host_script_results host_script_results_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER host_script_results_set_updated_at BEFORE UPDATE ON public.host_script_results FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: host_software_installs host_software_installs_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER host_software_installs_set_updated_at BEFORE UPDATE ON public.host_software_installs FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: host_software_installs host_software_installs_statuses; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER host_software_installs_statuses BEFORE INSERT OR UPDATE ON public.host_software_installs FOR EACH ROW EXECUTE FUNCTION public.host_software_installs_set_statuses();


--
-- Name: host_vpp_software_installs host_vpp_software_installs_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER host_vpp_software_installs_set_updated_at BEFORE UPDATE ON public.host_vpp_software_installs FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: hosts hosts_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER hosts_set_updated_at BEFORE UPDATE ON public.hosts FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: identity_certificates identity_certificates_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER identity_certificates_set_updated_at BEFORE UPDATE ON public.identity_certificates FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: in_house_app_configurations in_house_app_configurations_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER in_house_app_configurations_set_updated_at BEFORE UPDATE ON public.in_house_app_configurations FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: in_house_app_labels in_house_app_labels_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER in_house_app_labels_set_updated_at BEFORE UPDATE ON public.in_house_app_labels FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: in_house_app_upcoming_activities in_house_app_upcoming_activities_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER in_house_app_upcoming_activities_set_updated_at BEFORE UPDATE ON public.in_house_app_upcoming_activities FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: in_house_apps in_house_apps_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER in_house_apps_set_updated_at BEFORE UPDATE ON public.in_house_apps FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: invites invites_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER invites_set_updated_at BEFORE UPDATE ON public.invites FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: jobs jobs_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER jobs_set_updated_at BEFORE UPDATE ON public.jobs FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: label_membership label_membership_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER label_membership_set_updated_at BEFORE UPDATE ON public.label_membership FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: labels labels_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER labels_set_updated_at BEFORE UPDATE ON public.labels FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: mdm_adue_enrollment_challenges mdm_adue_enrollment_challenges_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER mdm_adue_enrollment_challenges_set_updated_at BEFORE UPDATE ON public.mdm_adue_enrollment_challenges FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: mdm_android_commands mdm_android_commands_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER mdm_android_commands_set_updated_at BEFORE UPDATE ON public.mdm_android_commands FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: mdm_apple_bootstrap_packages mdm_apple_bootstrap_packages_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER mdm_apple_bootstrap_packages_set_updated_at BEFORE UPDATE ON public.mdm_apple_bootstrap_packages FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: mdm_apple_declarations mdm_apple_declarations_token; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER mdm_apple_declarations_token BEFORE INSERT OR UPDATE ON public.mdm_apple_declarations FOR EACH ROW EXECUTE FUNCTION public.mdm_apple_declarations_set_token();


--
-- Name: mdm_apple_default_setup_assistants mdm_apple_default_setup_assistants_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER mdm_apple_default_setup_assistants_set_updated_at BEFORE UPDATE ON public.mdm_apple_default_setup_assistants FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: mdm_apple_enrollment_profiles mdm_apple_enrollment_profiles_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER mdm_apple_enrollment_profiles_set_updated_at BEFORE UPDATE ON public.mdm_apple_enrollment_profiles FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: mdm_apple_psso_devices mdm_apple_psso_devices_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER mdm_apple_psso_devices_set_updated_at BEFORE UPDATE ON public.mdm_apple_psso_devices FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: mdm_apple_psso_keys mdm_apple_psso_keys_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER mdm_apple_psso_keys_set_updated_at BEFORE UPDATE ON public.mdm_apple_psso_keys FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: mdm_apple_setup_assistant_profiles mdm_apple_setup_assistant_profiles_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER mdm_apple_setup_assistant_profiles_set_updated_at BEFORE UPDATE ON public.mdm_apple_setup_assistant_profiles FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: mdm_apple_setup_assistants mdm_apple_setup_assistants_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER mdm_apple_setup_assistants_set_updated_at BEFORE UPDATE ON public.mdm_apple_setup_assistants FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: mdm_configuration_profile_labels mdm_configuration_profile_labels_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER mdm_configuration_profile_labels_set_updated_at BEFORE UPDATE ON public.mdm_configuration_profile_labels FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: mdm_idp_accounts mdm_idp_accounts_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER mdm_idp_accounts_set_updated_at BEFORE UPDATE ON public.mdm_idp_accounts FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: mdm_windows_configuration_profiles mdm_windows_configuration_profiles_checksum; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER mdm_windows_configuration_profiles_checksum BEFORE INSERT OR UPDATE ON public.mdm_windows_configuration_profiles FOR EACH ROW EXECUTE FUNCTION public.mdm_windows_configuration_profiles_set_checksum();


--
-- Name: mdm_windows_enrollments mdm_windows_enrollments_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER mdm_windows_enrollments_set_updated_at BEFORE UPDATE ON public.mdm_windows_enrollments FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: microsoft_compliance_partner_host_statuses microsoft_compliance_partner_host_statuses_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER microsoft_compliance_partner_host_statuses_set_updated_at BEFORE UPDATE ON public.microsoft_compliance_partner_host_statuses FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: microsoft_compliance_partner_integrations microsoft_compliance_partner_integrations_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER microsoft_compliance_partner_integrations_set_updated_at BEFORE UPDATE ON public.microsoft_compliance_partner_integrations FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: mobile_device_management_solutions mobile_device_management_solutions_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER mobile_device_management_solutions_set_updated_at BEFORE UPDATE ON public.mobile_device_management_solutions FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: nano_cert_auth_associations nano_cert_auth_associations_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER nano_cert_auth_associations_set_updated_at BEFORE UPDATE ON public.nano_cert_auth_associations FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: nano_command_results nano_command_results_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER nano_command_results_set_updated_at BEFORE UPDATE ON public.nano_command_results FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: nano_commands nano_commands_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER nano_commands_set_updated_at BEFORE UPDATE ON public.nano_commands FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: nano_dep_names nano_dep_names_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER nano_dep_names_set_updated_at BEFORE UPDATE ON public.nano_dep_names FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: nano_devices nano_devices_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER nano_devices_set_updated_at BEFORE UPDATE ON public.nano_devices FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: nano_enrollment_queue nano_enrollment_queue_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER nano_enrollment_queue_set_updated_at BEFORE UPDATE ON public.nano_enrollment_queue FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: nano_enrollments nano_enrollments_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER nano_enrollments_set_updated_at BEFORE UPDATE ON public.nano_enrollments FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: nano_push_certs nano_push_certs_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER nano_push_certs_set_updated_at BEFORE UPDATE ON public.nano_push_certs FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: nano_users nano_users_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER nano_users_set_updated_at BEFORE UPDATE ON public.nano_users FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: network_interfaces network_interfaces_set_created_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER network_interfaces_set_created_at BEFORE UPDATE ON public.network_interfaces FOR EACH ROW EXECUTE FUNCTION public.fleet_touch_column('created_at');


--
-- Name: network_interfaces network_interfaces_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER network_interfaces_set_updated_at BEFORE UPDATE ON public.network_interfaces FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: operating_system_version_vulnerabilities operating_system_version_vulnerabilities_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER operating_system_version_vulnerabilities_set_updated_at BEFORE UPDATE ON public.operating_system_version_vulnerabilities FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: operating_system_vulnerabilities operating_system_vulnerabilities_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER operating_system_vulnerabilities_set_updated_at BEFORE UPDATE ON public.operating_system_vulnerabilities FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: packs packs_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER packs_set_updated_at BEFORE UPDATE ON public.packs FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: password_reset_requests password_reset_requests_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER password_reset_requests_set_updated_at BEFORE UPDATE ON public.password_reset_requests FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: policies policies_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER policies_set_updated_at BEFORE UPDATE ON public.policies FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: policy_labels policy_labels_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER policy_labels_set_updated_at BEFORE UPDATE ON public.policy_labels FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: policy_membership policy_membership_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER policy_membership_set_updated_at BEFORE UPDATE ON public.policy_membership FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: policy_stats policy_stats_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER policy_stats_set_updated_at BEFORE UPDATE ON public.policy_stats FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: queries queries_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER queries_set_updated_at BEFORE UPDATE ON public.queries FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: query_labels query_labels_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER query_labels_set_updated_at BEFORE UPDATE ON public.query_labels FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: scheduled_queries scheduled_queries_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER scheduled_queries_set_updated_at BEFORE UPDATE ON public.scheduled_queries FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: scim_groups scim_groups_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER scim_groups_set_updated_at BEFORE UPDATE ON public.scim_groups FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: scim_last_request scim_last_request_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER scim_last_request_set_updated_at BEFORE UPDATE ON public.scim_last_request FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: scim_user_emails scim_user_emails_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER scim_user_emails_set_updated_at BEFORE UPDATE ON public.scim_user_emails FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: scim_users scim_users_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER scim_users_set_updated_at BEFORE UPDATE ON public.scim_users FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: script_upcoming_activities script_upcoming_activities_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER script_upcoming_activities_set_updated_at BEFORE UPDATE ON public.script_upcoming_activities FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: scripts scripts_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER scripts_set_updated_at BEFORE UPDATE ON public.scripts FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: secret_variables secret_variables_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER secret_variables_set_updated_at BEFORE UPDATE ON public.secret_variables FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: sessions sessions_set_accessed_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER sessions_set_accessed_at BEFORE UPDATE ON public.sessions FOR EACH ROW EXECUTE FUNCTION public.fleet_touch_column('accessed_at');


--
-- Name: setup_experience_scripts setup_experience_scripts_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER setup_experience_scripts_set_updated_at BEFORE UPDATE ON public.setup_experience_scripts FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: software_categories software_categories_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER software_categories_set_updated_at BEFORE UPDATE ON public.software_categories FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: software_cpe software_cpe_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER software_cpe_set_updated_at BEFORE UPDATE ON public.software_cpe FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: software_cve software_cve_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER software_cve_set_updated_at BEFORE UPDATE ON public.software_cve FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: software_host_counts software_host_counts_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER software_host_counts_set_updated_at BEFORE UPDATE ON public.software_host_counts FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: software_install_upcoming_activities software_install_upcoming_activities_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER software_install_upcoming_activities_set_updated_at BEFORE UPDATE ON public.software_install_upcoming_activities FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: software_installer_labels software_installer_labels_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER software_installer_labels_set_updated_at BEFORE UPDATE ON public.software_installer_labels FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: software_installers software_installers_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER software_installers_set_updated_at BEFORE UPDATE ON public.software_installers FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: software_title_team_pins software_title_team_pins_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER software_title_team_pins_set_updated_at BEFORE UPDATE ON public.software_title_team_pins FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: software_titles software_titles_additional_identifier; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER software_titles_additional_identifier BEFORE INSERT OR UPDATE ON public.software_titles FOR EACH ROW EXECUTE FUNCTION public.software_titles_set_additional_identifier();


--
-- Name: software_titles_host_counts software_titles_host_counts_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER software_titles_host_counts_set_updated_at BEFORE UPDATE ON public.software_titles_host_counts FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: software_titles software_titles_set_unique_id; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER software_titles_set_unique_id BEFORE INSERT OR UPDATE ON public.software_titles FOR EACH ROW EXECUTE FUNCTION public.fleet_software_titles_set_unique_id();


--
-- Name: statistics statistics_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER statistics_set_updated_at BEFORE UPDATE ON public.statistics FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: trace_sampler_settings trace_sampler_settings_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trace_sampler_settings_set_updated_at BEFORE UPDATE ON public.trace_sampler_settings FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: upcoming_activities upcoming_activities_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER upcoming_activities_set_updated_at BEFORE UPDATE ON public.upcoming_activities FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: users_deleted users_deleted_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER users_deleted_set_updated_at BEFORE UPDATE ON public.users_deleted FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: users users_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER users_set_updated_at BEFORE UPDATE ON public.users FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: vpp_app_configurations vpp_app_configurations_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER vpp_app_configurations_set_updated_at BEFORE UPDATE ON public.vpp_app_configurations FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: vpp_app_team_labels vpp_app_team_labels_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER vpp_app_team_labels_set_updated_at BEFORE UPDATE ON public.vpp_app_team_labels FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: vpp_app_upcoming_activities vpp_app_upcoming_activities_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER vpp_app_upcoming_activities_set_updated_at BEFORE UPDATE ON public.vpp_app_upcoming_activities FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: vpp_apps vpp_apps_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER vpp_apps_set_updated_at BEFORE UPDATE ON public.vpp_apps FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: vpp_apps_teams vpp_apps_teams_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER vpp_apps_teams_set_updated_at BEFORE UPDATE ON public.vpp_apps_teams FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: vpp_client_users vpp_client_users_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER vpp_client_users_set_updated_at BEFORE UPDATE ON public.vpp_client_users FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: vpp_tokens vpp_tokens_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER vpp_tokens_set_updated_at BEFORE UPDATE ON public.vpp_tokens FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: vulnerability_host_counts vulnerability_host_counts_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER vulnerability_host_counts_set_updated_at BEFORE UPDATE ON public.vulnerability_host_counts FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: windows_mdm_command_queue windows_mdm_command_queue_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER windows_mdm_command_queue_set_updated_at BEFORE UPDATE ON public.windows_mdm_command_queue FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: windows_mdm_command_results windows_mdm_command_results_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER windows_mdm_command_results_set_updated_at BEFORE UPDATE ON public.windows_mdm_command_results FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: windows_mdm_commands windows_mdm_commands_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER windows_mdm_commands_set_updated_at BEFORE UPDATE ON public.windows_mdm_commands FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: windows_mdm_responses windows_mdm_responses_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER windows_mdm_responses_set_updated_at BEFORE UPDATE ON public.windows_mdm_responses FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: wstep_cert_auth_associations wstep_cert_auth_associations_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER wstep_cert_auth_associations_set_updated_at BEFORE UPDATE ON public.wstep_cert_auth_associations FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: wstep_certificates wstep_certificates_set_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER wstep_certificates_set_updated_at BEFORE UPDATE ON public.wstep_certificates FOR EACH ROW EXECUTE FUNCTION public.fleet_set_updated_at();


--
-- Name: abm_tokens abm_tokens_byod_team_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.abm_tokens
    ADD CONSTRAINT abm_tokens_byod_team_fk FOREIGN KEY (byod_default_team_id) REFERENCES public.teams(id) ON DELETE SET NULL;


--
-- Name: acme_accounts fk_acme_accounts_enrollment; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.acme_accounts
    ADD CONSTRAINT fk_acme_accounts_enrollment FOREIGN KEY (acme_enrollment_id) REFERENCES public.acme_enrollments(id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: acme_authorizations fk_acme_authorizations_order; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.acme_authorizations
    ADD CONSTRAINT fk_acme_authorizations_order FOREIGN KEY (acme_order_id) REFERENCES public.acme_orders(id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: acme_challenges fk_acme_challenges_authorization; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.acme_challenges
    ADD CONSTRAINT fk_acme_challenges_authorization FOREIGN KEY (acme_authorization_id) REFERENCES public.acme_authorizations(id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: acme_orders fk_acme_orders_account; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.acme_orders
    ADD CONSTRAINT fk_acme_orders_account FOREIGN KEY (acme_account_id) REFERENCES public.acme_accounts(id) ON UPDATE CASCADE ON DELETE CASCADE;


--
-- Name: android_devices fk_android_devices_team_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.android_devices
    ADD CONSTRAINT fk_android_devices_team_id FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE SET NULL;


--
-- Name: host_managed_local_account_passwords fk_hmlap_status; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_managed_local_account_passwords
    ADD CONSTRAINT fk_hmlap_status FOREIGN KEY (status) REFERENCES public.mdm_delivery_status(status) ON UPDATE CASCADE;


--
-- Name: host_custom_host_vitals fk_host_custom_host_vitals_custom_host_vital_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_custom_host_vitals
    ADD CONSTRAINT fk_host_custom_host_vitals_custom_host_vital_id FOREIGN KEY (custom_host_vital_id) REFERENCES public.custom_host_vitals(id) ON DELETE CASCADE;


--
-- Name: in_house_app_configurations fk_in_house_app_configurations_app; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.in_house_app_configurations
    ADD CONSTRAINT fk_in_house_app_configurations_app FOREIGN KEY (in_house_app_id) REFERENCES public.in_house_apps(id) ON DELETE CASCADE;


--
-- Name: mdm_apple_psso_keys fk_mdm_apple_psso_keys_host_uuid; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_apple_psso_keys
    ADD CONSTRAINT fk_mdm_apple_psso_keys_host_uuid FOREIGN KEY (host_uuid) REFERENCES public.mdm_apple_psso_devices(host_uuid) ON DELETE CASCADE;


--
-- Name: mdm_configuration_profile_update_settings fk_mdm_config_profile_update_settings_apple_decl_uuid; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_configuration_profile_update_settings
    ADD CONSTRAINT fk_mdm_config_profile_update_settings_apple_decl_uuid FOREIGN KEY (apple_declaration_uuid) REFERENCES public.mdm_apple_declarations(declaration_uuid) ON DELETE CASCADE;


--
-- Name: mdm_configuration_profile_update_settings fk_mdm_config_profile_update_settings_windows_profile_uuid; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_configuration_profile_update_settings
    ADD CONSTRAINT fk_mdm_config_profile_update_settings_windows_profile_uuid FOREIGN KEY (windows_profile_uuid) REFERENCES public.mdm_windows_configuration_profiles(profile_uuid) ON DELETE CASCADE;


--
-- Name: mdm_configuration_profile_variables fk_mdm_configuration_profile_variables_android_profile_uuid; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_configuration_profile_variables
    ADD CONSTRAINT fk_mdm_configuration_profile_variables_android_profile_uuid FOREIGN KEY (android_profile_uuid) REFERENCES public.mdm_android_configuration_profiles(profile_uuid) ON DELETE CASCADE;


--
-- Name: mdm_configuration_profile_variables fk_mdm_configuration_profile_variables_app_config_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_configuration_profile_variables
    ADD CONSTRAINT fk_mdm_configuration_profile_variables_app_config_id FOREIGN KEY (android_app_configuration_id) REFERENCES public.android_app_configurations(id) ON DELETE CASCADE;


--
-- Name: mdm_configuration_profile_variables fk_mdm_configuration_profile_variables_cert_template_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_configuration_profile_variables
    ADD CONSTRAINT fk_mdm_configuration_profile_variables_cert_template_id FOREIGN KEY (certificate_template_id) REFERENCES public.certificate_templates(id) ON DELETE CASCADE;


--
-- Name: software_title_team_pins fk_pin_title; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.software_title_team_pins
    ADD CONSTRAINT fk_pin_title FOREIGN KEY (title_id) REFERENCES public.software_titles(id) ON DELETE CASCADE;


--
-- Name: setup_experience_software_installers fk_seti_installer; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.setup_experience_software_installers
    ADD CONSTRAINT fk_seti_installer FOREIGN KEY (software_installer_id) REFERENCES public.software_installers(id) ON DELETE CASCADE;


--
-- Name: user_api_endpoints fk_user_api_endpoints_user; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_api_endpoints
    ADD CONSTRAINT fk_user_api_endpoints_user FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: vpp_app_configurations fk_vpp_app_configurations_app; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vpp_app_configurations
    ADD CONSTRAINT fk_vpp_app_configurations_app FOREIGN KEY (application_id, platform) REFERENCES public.vpp_apps(adam_id, platform) ON DELETE CASCADE;


--
-- Name: vpp_client_users fk_vpp_client_users_vpp_token_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vpp_client_users
    ADD CONSTRAINT fk_vpp_client_users_vpp_token_id FOREIGN KEY (vpp_token_id) REFERENCES public.vpp_tokens(id) ON DELETE CASCADE;


--
-- Name: host_mdm_apple_device_names host_mdm_apple_device_names_status; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_mdm_apple_device_names
    ADD CONSTRAINT host_mdm_apple_device_names_status FOREIGN KEY (status) REFERENCES public.mdm_delivery_status(status) ON UPDATE CASCADE;


--
-- Name: mdm_adue_enrollment_challenges mdm_adue_abm_token_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_adue_enrollment_challenges
    ADD CONSTRAINT mdm_adue_abm_token_fk FOREIGN KEY (abm_token_id) REFERENCES public.abm_tokens(id) ON DELETE CASCADE;


--
-- Name: mdm_adue_enrollment_challenges mdm_adue_idp_account_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_adue_enrollment_challenges
    ADD CONSTRAINT mdm_adue_idp_account_fk FOREIGN KEY (idp_account_uuid) REFERENCES public.mdm_idp_accounts(uuid) ON DELETE CASCADE;


--
-- Name: mdm_apple_declaration_asset_references mdm_apple_declaration_asset_references_asset_uuid_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_apple_declaration_asset_references
    ADD CONSTRAINT mdm_apple_declaration_asset_references_asset_uuid_fkey FOREIGN KEY (asset_uuid) REFERENCES public.mdm_apple_declaration_assets(asset_uuid);


--
-- Name: mdm_apple_declaration_asset_references mdm_apple_declaration_asset_references_declaration_uuid_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_apple_declaration_asset_references
    ADD CONSTRAINT mdm_apple_declaration_asset_references_declaration_uuid_fkey FOREIGN KEY (declaration_uuid) REFERENCES public.mdm_apple_declarations(declaration_uuid) ON DELETE CASCADE;


--
-- Name: mdm_configuration_profile_labels mdm_configuration_profile_labels_ibfk_1; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_configuration_profile_labels
    ADD CONSTRAINT mdm_configuration_profile_labels_ibfk_1 FOREIGN KEY (apple_profile_uuid) REFERENCES public.mdm_apple_configuration_profiles(profile_uuid) ON DELETE CASCADE;


--
-- Name: mdm_configuration_profile_labels mdm_configuration_profile_labels_ibfk_2; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_configuration_profile_labels
    ADD CONSTRAINT mdm_configuration_profile_labels_ibfk_2 FOREIGN KEY (windows_profile_uuid) REFERENCES public.mdm_windows_configuration_profiles(profile_uuid) ON DELETE CASCADE;


--
-- Name: mdm_configuration_profile_labels mdm_configuration_profile_labels_ibfk_4; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_configuration_profile_labels
    ADD CONSTRAINT mdm_configuration_profile_labels_ibfk_4 FOREIGN KEY (android_profile_uuid) REFERENCES public.mdm_android_configuration_profiles(profile_uuid) ON DELETE CASCADE;


--
-- Name: mdm_configuration_profile_labels mdm_configuration_profile_labels_ibfk_label; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_configuration_profile_labels
    ADD CONSTRAINT mdm_configuration_profile_labels_ibfk_label FOREIGN KEY (label_id) REFERENCES public.labels(id) ON DELETE RESTRICT;


--
-- Name: mdm_declaration_labels mdm_declaration_labels_ibfk_label; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mdm_declaration_labels
    ADD CONSTRAINT mdm_declaration_labels_ibfk_label FOREIGN KEY (label_id) REFERENCES public.labels(id) ON DELETE RESTRICT;


--
-- PostgreSQL database dump complete
--


