// Package mysql is a MySQL implementation of the Datastore interface.
package mysql

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/WatchBeam/clock"
	"github.com/XSAM/otelsql"
	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	condaccessdepot "github.com/fleetdm/fleet/v4/ee/server/service/condaccess/depot"
	hostidscepdepot "github.com/fleetdm/fleet/v4/ee/server/service/hostidentity/depot"
	"github.com/fleetdm/fleet/v4/server/config"
	"github.com/fleetdm/fleet/v4/server/contexts/ctxdb"
	"github.com/fleetdm/fleet/v4/server/contexts/ctxerr"
	"github.com/fleetdm/fleet/v4/server/datastore/mysql/migrations/data"
	"github.com/fleetdm/fleet/v4/server/datastore/mysql/migrations/tables"
	"github.com/fleetdm/fleet/v4/server/datastore/mysql/rdsauth"
	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/goose"
	"github.com/fleetdm/fleet/v4/server/mdm/android"
	nano_push "github.com/fleetdm/fleet/v4/server/mdm/nanomdm/push"
	scep_depot "github.com/fleetdm/fleet/v4/server/mdm/scep/depot"
	common_mysql "github.com/fleetdm/fleet/v4/server/platform/mysql"
	pg "github.com/fleetdm/fleet/v4/server/platform/postgres" // register pgx-rebind driver for PostgreSQL
	"github.com/go-sql-driver/mysql"
	"github.com/hashicorp/go-multierror"
	"github.com/jmoiron/sqlx"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"golang.org/x/sync/singleflight"
)

const (
	mySQLTimestampFormat = "2006-01-02 15:04:05" // %Y/%m/%d %H:%M:%S

	// Migration IDs needed for fixing broken migrations that some customers encountered with fleet v4.73.2
	// See https://github.com/fleetdm/fleet/issues/33562
	fleet4732BadMigrationID1  = 20250918154557 // was 20250918154557_AddKernelHostCountsIndexForVulnQueries.go
	fleet4732GoodMigrationID1 = 20250817154557 // 20250817154557_AddKernelHostCountsIndexForVulnQueries.go

	fleet4732BadMigrationID2  = 20250904115553 // was 20250904115553_OptimizeHostScriptResultsIndex.go
	fleet4732GoodMigrationID2 = 20250816115553 // 20250816115553_OptimizeHostScriptResultsIndex.go

	fleet4731GoodMigrationID = 20250815130115
)

// Datastore is an implementation of fleet.Datastore interface backed by
// MySQL
type Datastore struct {
	replica fleet.DBReader // so it cannot be used to perform writes
	primary *sqlx.DB

	logger  *slog.Logger
	clock   clock.Clock
	config  config.MysqlConfig
	dialect DialectHelper
	pusher  nano_push.Pusher
	android.Datastore

	// nil if no read replica
	readReplicaConfig *common_mysql.MysqlConfig

	// minimum interval between software last_opened_at timestamp to update the
	// database (see file software.go).
	minLastOpenedAtDiff time.Duration

	writeCh chan itemToWrite

	// stmtCacheMu protects access to stmtCache.
	stmtCacheMu sync.Mutex
	// stmtCache holds statements for queries.
	stmtCache map[string]*sqlx.Stmt

	// for tests, set to override the default batch size.
	testDeleteMDMProfilesBatchSize int
	// for tests, set to override the default batch size.
	testUpsertMDMDesiredProfilesBatchSize int
	// for tests, set to override the default page size of ReconcileWindowsProfilesStatus.
	testWindowsProfilesStatusReconcileBatchSize int
	// for tests, run dispatchWindowsProfilesStatusRollupRefresh synchronously so tests can assert
	// rollup state immediately after bulk operations.
	testSynchronousWindowsRollupDispatch bool

	// set this to the execution ids of activities that should be activated in
	// the next call to activateNextUpcomingActivity, instead of picking the next
	// available activity based on normal prioritization and creation date
	// ordering.
	testActivateSpecificNextActivities []string

	// This key is used to encrypt sensitive data stored in the Fleet DB, for example MDM
	// certificates and keys.
	serverPrivateKey string

	// knownSoftwareTitleKeys caches title keys that are known to exist in software_titles.
	// This eliminates redundant INSERT IGNORE statements during concurrent software ingestion,
	// preventing lock convoys on the unique index when many hosts report the same software catalog.
	// The cache evicts an arbitrary half of entries once it reaches a fixed size cap to avoid
	// unbounded growth on long-lived servers without forcing a full cold start.
	knownSoftwareTitleKeys map[string]struct{}
	// knownSoftwareTitleKeysMu serializes cache writes and clears; reads use RLock.
	knownSoftwareTitleKeysMu sync.RWMutex

	// titleInsertSF deduplicates concurrent INSERT IGNORE INTO software_titles calls for the
	// same title key. Only one goroutine per title actually executes the INSERT; others wait
	// and share the result. This prevents lock convoys on cold-start (#48719).
	titleInsertSF singleflight.Group
}

// maxKnownSoftwareTitleKeys caps the in-process software title cache at roughly 100k entries so
// long-lived servers do not retain every title they have ever seen.
const maxKnownSoftwareTitleKeys = 100_000

// evictKnownSoftwareTitleKeys removes half the cache when the cap is hit. Keeping the other half
// preserves most steady-state hits while avoiding a full cold start that would reintroduce a burst
// of INSERT IGNORE statements.
const evictKnownSoftwareTitleKeys = maxKnownSoftwareTitleKeys / 2

// WithPusher sets an APNs pusher for the datastore, used when activating
// next activities that require MDM commands.
func (ds *Datastore) WithPusher(p nano_push.Pusher) {
	ds.pusher = p
}

// reader returns the DB instance to use for read-only statements, which is the
// replica unless the primary has been explicitly required via
// ctxdb.RequirePrimary.
func (ds *Datastore) reader(ctx context.Context) fleet.DBReader {
	if ctxdb.IsPrimaryRequired(ctx) {
		return ds.primary
	}
	return ds.replica
}

// currentDatabaseFn returns the SQL function to get the current database name.
// MySQL: DATABASE(), PostgreSQL: current_database()
func (ds *Datastore) currentDatabaseFn() string {
	if ds.dialect.IsPostgres() {
		return "current_database()"
	}
	return "(SELECT DATABASE())"
}

// writer returns the DB instance to use for write statements, which is always
// the primary.
func (ds *Datastore) writer(ctx context.Context) *sqlx.DB {
	return ds.primary
}

// Querier is any type that can execute SQL (sqlx.DB, sqlx.Tx, sqlx.ExtContext).
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// insertAndGetID executes an INSERT and returns the auto-generated ID.
// For MySQL, uses LastInsertId(). For PostgreSQL, appends RETURNING <col>
// where <col> is looked up per-table from the PG identity-column map (most
// tables use "id", a handful use "serial", "profile_id", or "auto_increment").
func (ds *Datastore) insertAndGetID(ctx context.Context, q Querier, query string, args ...any) (int64, error) {
	if ds.dialect.IsPostgres() {
		var id int64
		err := q.QueryRowContext(ctx, pgReturningQuery(query), args...).Scan(&id)
		return id, err
	}
	res, err := q.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// insertAndGetIDTx is like insertAndGetID but for sqlx.ExtContext (transactions).
func insertAndGetIDTx(ctx context.Context, tx sqlx.ExtContext, dialect DialectHelper, query string, args ...any) (int64, error) {
	if dialect.IsPostgres() {
		var id int64
		err := tx.QueryRowxContext(ctx, pgReturningQuery(query), args...).Scan(&id)
		return id, err
	}
	res, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// pgReturningQuery rewrites an INSERT statement to append RETURNING <col>,
// stripping any trailing semicolon first. The column is determined per-table
// via the embedded PG identity-column map (postgres.IdentityColumnFor):
// "id" for most tables, "serial" for nano-style counter tables, etc. Falls
// back to "id" when the table is unknown so callers that target a table not
// in the map keep working.
func pgReturningQuery(query string) string {
	trimmed := strings.TrimRight(query, " \t\r\n;")
	col := "id"
	if m := pgInsertTablePattern.FindStringSubmatch(trimmed); m != nil {
		if c, ok := pg.IdentityColumnFor(m[1]); ok {
			col = c
		}
	}
	return trimmed + " RETURNING " + col
}

var pgInsertTablePattern = regexp.MustCompile(`(?is)^\s*INSERT\s+INTO\s+(?:public\.)?["` + "`" + `]?([a-zA-Z_][a-zA-Z0-9_]*)`)

// loadOrPrepareStmt will load a statement from the statement cache.
// If not available, it will attempt to prepare (create) it.
// Returns nil if it failed to prepare a statement.
//
// IMPORTANT: Adding prepare statements consumes MySQL server resources and is limited by the MySQL max_prepared_stmt_count
// system variable. This method may create 1 prepare statement for EACH database connection. Customers must be notified
// to update their MySQL configurations when additional prepare statements are added.
// For more detail, see: https://github.com/fleetdm/fleet/issues/15476
func (ds *Datastore) loadOrPrepareStmt(ctx context.Context, query string) *sqlx.Stmt {
	// the cache is only available on the replica
	if ctxdb.IsPrimaryRequired(ctx) {
		return nil
	}

	ds.stmtCacheMu.Lock()
	defer ds.stmtCacheMu.Unlock()

	stmt, ok := ds.stmtCache[query]
	if !ok {
		var err error
		stmt, err = sqlx.PreparexContext(ctx, ds.replica, query)
		if err != nil {
			ds.logger.ErrorContext(ctx, "failed to prepare statement",
				"query", query,
				"err", err,
			)
			return nil
		}
		ds.stmtCache[query] = stmt
	}
	return stmt
}

func (ds *Datastore) deleteCachedStmt(ctx context.Context, query string) {
	ds.stmtCacheMu.Lock()
	defer ds.stmtCacheMu.Unlock()
	stmt, ok := ds.stmtCache[query]
	if ok {
		if err := stmt.Close(); err != nil {
			ds.logger.ErrorContext(ctx, "failed to close prepared statement before deleting it",
				"query", query,
				"err", err,
			)
		}
		delete(ds.stmtCache, query)
	}
}

// NewSCEPDepot returns a scep_depot.Depot that uses the Datastore
// underlying MySQL writer *sql.DB.
func (ds *Datastore) NewSCEPDepot() (scep_depot.Depot, error) {
	return newSCEPDepot(ds.primary.DB, ds)
}

// NewHostIdentitySCEPDepot returns a scep_depot.Depot for host identity certs that uses the Datastore
// underlying MySQL writer *sql.DB.
func (ds *Datastore) NewHostIdentitySCEPDepot(logger *slog.Logger, cfg *config.FleetConfig) (scep_depot.Depot, error) {
	return hostidscepdepot.NewHostIdentitySCEPDepot(ds.primary, ds, logger, cfg)
}

// NewConditionalAccessSCEPDepot returns a new conditional access SCEP depot that uses the
// underlying MySQL writer *sql.DB.
func (ds *Datastore) NewConditionalAccessSCEPDepot(logger *slog.Logger, cfg *config.FleetConfig) (scep_depot.Depot, error) {
	return condaccessdepot.NewConditionalAccessSCEPDepot(ds.primary, ds, logger, cfg)
}

type entity struct {
	name string
}

var (
	hostsTable    = entity{"hosts"}
	invitesTable  = entity{"invites"}
	packsTable    = entity{"packs"}
	queriesTable  = entity{"queries"}
	sessionsTable = entity{"sessions"}
	usersTable    = entity{"users"}
)

func (ds *Datastore) withRetryTxx(ctx context.Context, fn common_mysql.TxFn) (err error) {
	return common_mysql.WithRetryTxx(ctx, ds.writer(ctx), fn, ds.logger)
}

// withTx provides a common way to commit/rollback a txFn
func (ds *Datastore) withTx(ctx context.Context, fn common_mysql.TxFn) (err error) {
	return common_mysql.WithTxx(ctx, ds.writer(ctx), fn, ds.logger)
}

// withReadTx runs fn in a read-only transaction with a consistent snapshot of the DB
// for executing multiple SELECT queries in an isolated fashion. It should be preferred
// over withTx for these usecases as mysql applies some optimizations to transactions
// declared as read-only versus.
func (ds *Datastore) withReadTx(ctx context.Context, fn common_mysql.ReadTxFn) (err error) {
	reader := ds.reader(ctx)
	readerDB, ok := reader.(*sqlx.DB)
	if !ok {
		return ctxerr.New(ctx, "failed to cast reader to *sqlx.DB")
	}
	return common_mysql.WithReadOnlyTxx(ctx, readerDB, fn, ds.logger)
}

// NewDBConnections creates database connections from config.
// The returned connections can be used to create multiple datastores
// that share the same underlying database connections.
func NewDBConnections(cfg config.MysqlConfig, opts ...DBOption) (*common_mysql.DBConnections, error) {
	options := &common_mysql.DBOptions{
		MinLastOpenedAtDiff: defaultMinLastOpenedAtDiff,
		MaxAttempts:         defaultMaxAttempts,
		Logger:              slog.New(slog.DiscardHandler),
	}

	for _, setOpt := range opts {
		if setOpt != nil {
			if err := setOpt(options); err != nil {
				return nil, err
			}
		}
	}

	if err := checkAndModifyConfig(&cfg); err != nil {
		return nil, err
	}

	// Set migration client dialects to match the configured driver.
	if cfg.Driver == "postgres" {
		tables.SetDialect("postgres")
		data.SetDialect("postgres")
	}

	// Convert replica config once so that checkAndModifyConfig mutations are preserved for the later NewDB call.
	var replicaConf *config.MysqlConfig
	if options.ReplicaConfig != nil {
		replicaConf = fromCommonMysqlConfig(options.ReplicaConfig)
		if err := checkAndModifyConfig(replicaConf); err != nil {
			return nil, fmt.Errorf("replica: %w", err)
		}
	}

	// Set up IAM authentication connector factory if needed
	if err := setupIAMAuthIfNeeded(&cfg, options); err != nil {
		return nil, err
	}

	dbWriter, err := NewDB(&cfg, options)
	if err != nil {
		return nil, err
	}
	dbReader := dbWriter
	if replicaConf != nil {
		// Set up IAM auth for replica if needed (may have different region/credentials)
		replicaOptions := *options
		// Reset ConnectorFactory - replica may have different auth requirements than primary
		replicaOptions.ConnectorFactory = nil
		if err := setupIAMAuthIfNeeded(replicaConf, &replicaOptions); err != nil {
			return nil, fmt.Errorf("replica: %w", err)
		}
		dbReader, err = NewDB(replicaConf, &replicaOptions)
		if err != nil {
			return nil, err
		}
	}

	return &common_mysql.DBConnections{Primary: dbWriter, Replica: dbReader, Options: options}, nil
}

// NewDatastore creates a Datastore using existing database connections.
// Use this when you need to share database connections with other bounded context datastores.
func NewDatastore(conns *common_mysql.DBConnections, cfg config.MysqlConfig, c clock.Clock) (*Datastore, error) {
	ds := &Datastore{
		primary:                conns.Primary,
		replica:                conns.Replica,
		logger:                 conns.Options.Logger,
		clock:                  c,
		config:                 cfg,
		dialect:                dialectForDriver(cfg.Driver),
		readReplicaConfig:      conns.Options.ReplicaConfig,
		writeCh:                make(chan itemToWrite),
		stmtCache:              make(map[string]*sqlx.Stmt),
		minLastOpenedAtDiff:    conns.Options.MinLastOpenedAtDiff,
		serverPrivateKey:       conns.Options.PrivateKey,
		knownSoftwareTitleKeys: make(map[string]struct{}),
		Datastore:              NewAndroidDatastore(conns.Options.Logger, conns.Primary, conns.Replica, dialectForDriver(cfg.Driver)),
	}

	go ds.writeChanLoop()

	return ds, nil
}

// New creates a MySQL datastore.
func New(cfg config.MysqlConfig, c clock.Clock, opts ...DBOption) (*Datastore, error) {
	conns, err := NewDBConnections(cfg, opts...)
	if err != nil {
		return nil, err
	}
	return NewDatastore(conns, cfg, c)
}

type itemToWrite struct {
	ctx   context.Context
	errCh chan error
	item  interface{}
}

type hostXUpdatedAt struct {
	hostID    uint
	updatedAt time.Time
	what      string
}

func (ds *Datastore) writeChanLoop() {
	for item := range ds.writeCh {
		switch actualItem := item.item.(type) {
		case *fleet.Host:
			item.errCh <- ds.UpdateHost(item.ctx, actualItem)
		case hostXUpdatedAt:
			err := ds.withRetryTxx(
				item.ctx, func(tx sqlx.ExtContext) error {
					query := fmt.Sprintf(`UPDATE hosts SET %s = ? WHERE id=?`, actualItem.what)
					_, err := tx.ExecContext(item.ctx, query, actualItem.updatedAt, actualItem.hostID)
					return err
				},
			)
			item.errCh <- ctxerr.Wrap(item.ctx, err, "updating hosts label updated at")
		}
	}
}

var otelTracedDriverName string

func init() {
	var err error
	otelTracedDriverName, err = otelsql.Register("mysql",
		otelsql.WithAttributes(
			attribute.String("db.system", "mysql"),
			semconv.DBSystemNameMySQL,
		),
		otelsql.WithSpanOptions(otelsql.SpanOptions{
			// DisableErrSkip ignores driver.ErrSkip errors which are frequently returned by the MySQL driver
			// when certain optional methods or paths are not implemented/taken.
			// For example: interpolateParams=false (the secure default) will not do a parametrized sql.conn.query directly without preparing it first, causing driver.ErrSkip
			DisableErrSkip: true,
			// Omitting span for sql.conn.reset_session since it takes ~1us and doesn't provide useful information
			OmitConnResetSession: true,
			// Omitting span for sql.rows since it is very quick and typically doesn't provide useful information beyond what's already reported by prepare/exec/query
			OmitRows: true,
		}),
		// WithSpanNameFormatter allows us to customize the span name, which is especially useful for SQL queries run outside an HTTPS transaction,
		// which do not belong to a parent span, show up as their own trace, and would otherwise be named "sql.conn.query" or "sql.conn.exec".
		otelsql.WithSpanNameFormatter(func(ctx context.Context, method otelsql.Method, query string) string {
			if query == "" {
				return string(method)
			}
			// Append query with extra whitespaces removed
			query = strings.Join(strings.Fields(query), " ")
			const maxQueryLen = 100
			if len(query) > maxQueryLen {
				query = query[:maxQueryLen] + "..."
			}
			return string(method) + ": " + query
		}),
	)
	if err != nil {
		panic(err)
	}
}

func NewDB(conf *config.MysqlConfig, opts *common_mysql.DBOptions) (*sqlx.DB, error) {
	if conf.Driver == "postgres" {
		return newPostgresDB(conf)
	}
	return common_mysql.NewDB(toCommonMysqlConfig(conf), opts, otelTracedDriverName)
}

// newPostgresDB opens a PostgreSQL connection using pgx/stdlib.
func newPostgresDB(conf *config.MysqlConfig) (*sqlx.DB, error) {
	// Build PostgreSQL DSN from the MySQL-style config fields.
	// Address is expected as "host:port".
	host, port, err := net.SplitHostPort(conf.Address)
	if err != nil {
		host = conf.Address
		port = "5432"
	}
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, conf.Username, conf.Password, conf.Database,
	)
	if conf.TLSCA != "" {
		dsn = fmt.Sprintf(
			"host=%s port=%s user=%s password=%s dbname=%s sslmode=verify-ca sslrootcert=%s",
			host, port, conf.Username, conf.Password, conf.Database, conf.TLSCA,
		)
	}

	// Use "pgx-rebind" driver which wraps pgx/stdlib and auto-converts
	// MySQL-style ? placeholders to PostgreSQL $N placeholders.
	db, err := sqlx.Open("pgx-rebind", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	db.SetMaxOpenConns(conf.MaxOpenConns)
	db.SetMaxIdleConns(conf.MaxIdleConns)
	return db, nil
}

// toCommonMysqlConfig converts a config.MysqlConfig to common_mysql.MysqlConfig.
func toCommonMysqlConfig(conf *config.MysqlConfig) *common_mysql.MysqlConfig {
	return &common_mysql.MysqlConfig{
		Protocol:        conf.Protocol,
		Address:         conf.Address,
		Username:        conf.Username,
		Password:        conf.Password,
		PasswordPath:    conf.PasswordPath,
		Database:        conf.Database,
		TLSCert:         conf.TLSCert,
		TLSKey:          conf.TLSKey,
		TLSCA:           conf.TLSCA,
		TLSServerName:   conf.TLSServerName,
		TLSConfig:       conf.TLSConfig,
		MaxOpenConns:    conf.MaxOpenConns,
		MaxIdleConns:    conf.MaxIdleConns,
		ConnMaxLifetime: conf.ConnMaxLifetime,
		SQLMode:         conf.SQLMode,
		Region:          conf.Region,
	}
}

// toCommonLoggingConfig converts a config.LoggingConfig to common_mysql.LoggingConfig.
func toCommonLoggingConfig(conf *config.LoggingConfig) *common_mysql.LoggingConfig {
	if conf == nil {
		return nil
	}
	return &common_mysql.LoggingConfig{
		TracingEnabled: conf.TracingEnabled,
		TracingType:    conf.TracingType,
	}
}

// fromCommonMysqlConfig converts a common_mysql.MysqlConfig to config.MysqlConfig.
func fromCommonMysqlConfig(conf *common_mysql.MysqlConfig) *config.MysqlConfig {
	if conf == nil {
		return nil
	}
	return &config.MysqlConfig{
		Protocol:        conf.Protocol,
		Address:         conf.Address,
		Username:        conf.Username,
		Password:        conf.Password,
		PasswordPath:    conf.PasswordPath,
		Database:        conf.Database,
		TLSCert:         conf.TLSCert,
		TLSKey:          conf.TLSKey,
		TLSCA:           conf.TLSCA,
		TLSServerName:   conf.TLSServerName,
		TLSConfig:       conf.TLSConfig,
		MaxOpenConns:    conf.MaxOpenConns,
		MaxIdleConns:    conf.MaxIdleConns,
		ConnMaxLifetime: conf.ConnMaxLifetime,
		SQLMode:         conf.SQLMode,
		Region:          conf.Region,
	}
}

// dialectForDriver returns the DialectHelper for the given driver name.
// Empty string defaults to "mysql".
func dialectForDriver(driver string) DialectHelper {
	switch driver {
	case "postgres":
		return postgresDialect{}
	case "", "mysql":
		return mysqlDialect{}
	default:
		// checkAndModifyConfig validates the driver before this is called,
		// so reaching here means a programming error.
		panic(fmt.Sprintf("unsupported database driver: %q", driver))
	}
}

func checkAndModifyConfig(conf *config.MysqlConfig) error {
	if conf.Driver != "" && conf.Driver != "mysql" && conf.Driver != "postgres" {
		return fmt.Errorf("unsupported database driver %q: valid values are \"mysql\" and \"postgres\"", conf.Driver)
	}

	if conf.PasswordPath != "" && conf.Password != "" {
		return errors.New("A MySQL password and a MySQL password file were provided - please specify only one")
	}

	// Check to see if the flag is populated
	// Check if file exists on disk
	// If file exists read contents
	if conf.PasswordPath != "" {
		fileContents, err := os.ReadFile(conf.PasswordPath)
		if err != nil {
			return err
		}
		conf.Password = strings.TrimSpace(string(fileContents))
	}

	if conf.TLSCA != "" {
		conf.TLSConfig = "custom"
		err := registerTLS(*conf)
		if err != nil {
			return fmt.Errorf("register TLS config for mysql: %w", err)
		}
	}
	return nil
}

// setupIAMAuthIfNeeded configures IAM authentication for RDS if the config
// indicates it should be used (no password provided but region is set).
func setupIAMAuthIfNeeded(conf *config.MysqlConfig, opts *common_mysql.DBOptions) error {
	if conf.Password != "" || conf.PasswordPath != "" || conf.Region == "" {
		return nil
	}

	// Parse host and port from address
	host, port, err := net.SplitHostPort(conf.Address)
	if err != nil {
		host = conf.Address
		port = "3306"
	}

	factory, err := rdsauth.NewConnectorFactory(conf, host, port)
	if err != nil {
		return fmt.Errorf("failed to create RDS IAM auth connector factory: %w", err)
	}
	opts.ConnectorFactory = factory
	return nil
}

func (ds *Datastore) MigrateTables(ctx context.Context) error {
	if ds.dialect.IsPostgres() {
		// First apply the baseline (no-op if schema already exists) and seed
		// migration history for migrations <= marker. Then run goose Up so
		// any newer migrations (added upstream after the baseline marker) get
		// applied.
		if err := ds.migratePGBaseline(ctx); err != nil {
			return err
		}
	}
	return tables.MigrationClient.Up(ds.writer(ctx).DB, "")
}

func (ds *Datastore) MigrateData(ctx context.Context) error {
	if ds.dialect.IsPostgres() {
		// PG baseline schema includes all data migrations (label seeds, etc.)
		return nil
	}
	return data.MigrationClient.Up(ds.writer(ctx).DB, "")
}

//go:embed pg_baseline_schema.sql
var pgBaselineSchemaSQL string

//go:embed pg_baseline_post.sql
var pgBaselinePostSQL string

// pgBaselineMarkerRe matches the `pg-baseline-up-to-migration: <ts>` header
// comment in pg_baseline_schema.sql. The timestamp records the highest
// migration version embedded in the baseline.
var pgBaselineMarkerRe = regexp.MustCompile(`(?m)^--\s*pg-baseline-up-to-migration:\s*(\d+)\s*$`)

// parsePGBaselineMarker returns the highest migration version embedded in the
// baseline. Returns 0 when no marker is present (older baselines), in which
// case drift detection is skipped and a warning is logged elsewhere.
func parsePGBaselineMarker(sql string) int64 {
	m := pgBaselineMarkerRe.FindStringSubmatch(sql)
	if m == nil {
		return 0
	}
	v, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// migratePGBaseline applies the PG baseline schema for fresh PostgreSQL databases
// and always runs idempotent post-baseline fixups (e.g., asserting object ownership).
//
// On a fresh apply it also seeds migration_status_tables with all migration
// versions <= the baseline marker, so MigrationStatus reports correctly and
// downstream code that queries the table sees the right history. On every
// startup it logs a warning if the running code carries migrations newer
// than the embedded baseline (silent drift would otherwise accumulate until
// a feature broke at runtime).
func (ds *Datastore) migratePGBaseline(ctx context.Context) error {
	marker := parsePGBaselineMarker(pgBaselineSchemaSQL)

	var exists bool
	err := ds.writer(ctx).GetContext(ctx, &exists,
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'hosts')`)
	if err != nil {
		return ctxerr.Wrap(ctx, err, "checking PG schema")
	}
	freshApply := false
	if exists {
		ds.logger.InfoContext(ctx, "PostgreSQL schema already exists, skipping baseline")
	} else {
		ds.logger.InfoContext(ctx, "Applying PostgreSQL baseline schema", "marker_version", marker)
		if _, err := ds.writer(ctx).ExecContext(ctx, pgBaselineSchemaSQL); err != nil {
			return ctxerr.Wrap(ctx, err, "applying PG baseline schema")
		}
		ds.logger.InfoContext(ctx, "PostgreSQL baseline schema applied successfully")
		freshApply = true
	}
	if _, err := ds.writer(ctx).ExecContext(ctx, pgBaselinePostSQL); err != nil {
		return ctxerr.Wrap(ctx, err, "applying PG post-baseline fixups")
	}
	if freshApply {
		if err := ds.seedPGMigrationHistory(ctx, marker); err != nil {
			return ctxerr.Wrap(ctx, err, "seeding PG migration history")
		}
	}
	ds.warnPGMigrationDrift(ctx, marker)
	return nil
}

// seedPGMigrationHistory populates migration_status_tables and migration_status_data
// with all known migration versions <= marker, so MigrationStatus does not falsely
// report the DB as empty after a fresh baseline apply. No-op when marker is 0
// (baseline has no marker — operator must regen) or when the target table already
// has rows (guards against double-seed and never touches existing DBs).
//
// The embedded PG baseline is generated from a production DB via `pg_dump
// --schema-only` then patched with the data-migration effects (builtin labels,
// etc.). All data migrations with version <= marker have therefore already
// produced their effects in the baseline data; we just need to record them as
// applied so future `fleet prepare db` runs don't try to re-run them.
func (ds *Datastore) seedPGMigrationHistory(ctx context.Context, marker int64) error {
	if marker == 0 {
		return nil
	}
	if err := ds.seedPGMigrationTable(ctx, marker, "migration_status_tables", tables.MigrationClient.Migrations); err != nil {
		return err
	}
	return ds.seedPGMigrationTable(ctx, marker, "migration_status_data", data.MigrationClient.Migrations)
}

// seedPGMigrationTableAllowed is the set of tracking tables this helper is
// allowed to write to. We string-concat tableName into a literal SQL
// statement, so this allowlist gates gosec's G202 concern and also prevents
// a future caller from accidentally writing to an arbitrary table.
var seedPGMigrationTableAllowed = map[string]struct{}{
	"migration_status_tables": {},
	"migration_status_data":   {},
}

func (ds *Datastore) seedPGMigrationTable(ctx context.Context, marker int64, tableName string, knownMigrations goose.Migrations) error {
	if _, ok := seedPGMigrationTableAllowed[tableName]; !ok {
		return ctxerr.New(ctx, "seedPGMigrationTable: refusing to write to disallowed table "+tableName)
	}
	var existing int
	if err := ds.writer(ctx).GetContext(ctx, &existing,
		`SELECT COUNT(*) FROM `+tableName+` WHERE is_applied`); err != nil {
		// Note: a partially-applied baseline can leave the tracking table
		// missing while `hosts` is also missing — caller sees this as an error
		// here rather than the more obvious "schema apply failed". Diagnose by
		// running the embedded baseline against an empty PG and checking which
		// statement errors first.
		return ctxerr.Wrap(ctx, err, "counting existing PG migration history in "+tableName)
	}
	if existing > 0 {
		return nil
	}
	versions := versionsAtOrBelow(knownMigrations, marker)
	if len(versions) == 0 {
		return nil
	}
	// Bulk insert with PG positional placeholders. The tracking tables have no
	// unique constraint on version_id (goose appends a row per up/down event),
	// so a plain INSERT is correct.
	//
	// versions is sorted ascending by versionsAtOrBelow → partitionMigrationVersions,
	// so PG assigns auto-increment ids in ascending version_id order. This
	// preserves id↔version_id alignment for any downstream consumer that
	// (incorrectly) infers "current version" from MAX(id). The dialect's
	// dbVersionQuery uses ORDER BY version_id DESC, id DESC for that reason
	// — even so, a defensive sort keeps the table tidy for human inspection
	// and protects against future query regressions.
	var b strings.Builder
	b.WriteString("INSERT INTO " + tableName + " (version_id, is_applied) VALUES ")
	args := make([]any, 0, len(versions))
	for i, v := range versions {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "($%d, true)", i+1)
		args = append(args, v)
	}
	if _, err := ds.writer(ctx).ExecContext(ctx, b.String(), args...); err != nil {
		return ctxerr.Wrap(ctx, err, "seeding "+tableName)
	}
	ds.logger.InfoContext(ctx, "Seeded PG migration history",
		"table", tableName, "rows", len(versions), "marker_version", marker)
	return nil
}

// warnPGMigrationDrift logs a loud warning when the running code has
// migrations newer than the embedded PG baseline. The PG path has no
// per-migration runner (migrations are MySQL DDL), so any drift means new
// code is running against an old schema until pg_baseline_schema.sql is
// regenerated.
func (ds *Datastore) warnPGMigrationDrift(ctx context.Context, marker int64) {
	if marker == 0 {
		ds.logger.WarnContext(ctx,
			"PostgreSQL baseline has no pg-baseline-up-to-migration marker; cannot detect migration drift",
			"remediation", "add the marker to server/datastore/mysql/pg_baseline_schema.sql header")
		return
	}
	pending := versionsAbove(tables.MigrationClient.Migrations, marker)
	if len(pending) == 0 {
		return
	}
	ds.logger.WarnContext(ctx,
		"PostgreSQL baseline is stale: code has migrations not present in the embedded baseline",
		"baseline_version", marker,
		"pending_count", len(pending),
		"oldest_pending", pending[0],
		"newest_pending", pending[len(pending)-1],
		"remediation", "regenerate pg_baseline_schema.sql (see file header) and bump the pg-baseline-up-to-migration marker",
	)
}

// partitionMigrationVersions splits the migration list at marker (inclusive
// of atOrBelow). Both returned slices are sorted ascending. One pass over the
// input, one sort of each side — used together in migratePGBaseline so the
// shared structure is intentional.
func partitionMigrationVersions(ms goose.Migrations, marker int64) (atOrBelow, above []int64) {
	atOrBelow = make([]int64, 0, len(ms))
	above = make([]int64, 0)
	for _, m := range ms {
		if m.Version <= marker {
			atOrBelow = append(atOrBelow, m.Version)
		} else {
			above = append(above, m.Version)
		}
	}
	slices.Sort(atOrBelow)
	slices.Sort(above)
	return atOrBelow, above
}

// versionsAtOrBelow / versionsAbove are thin wrappers around
// partitionMigrationVersions kept for readability at call sites — each caller
// only needs one half of the partition. The unit tests cover both halves.
func versionsAtOrBelow(ms goose.Migrations, marker int64) []int64 {
	atOrBelow, _ := partitionMigrationVersions(ms, marker)
	return atOrBelow
}

func versionsAbove(ms goose.Migrations, marker int64) []int64 {
	_, above := partitionMigrationVersions(ms, marker)
	return above
}

// loadMigrations manually loads the applied migrations in ascending
// order (goose doesn't provide such functionality).
//
// Returns two lists of version IDs (one for "table" and one for "data").
func (ds *Datastore) loadMigrations(
	ctx context.Context,
	writer *sql.DB,
	reader fleet.DBReader,
) (tableRecs []int64, dataRecs []int64, err error) {
	// We need to run the following to trigger the creation of the migration status tables.
	_, err = tables.MigrationClient.GetDBVersion(writer)
	if err != nil {
		return nil, nil, err
	}
	_, err = data.MigrationClient.GetDBVersion(writer)
	if err != nil {
		return nil, nil, err
	}
	// version_id > 0 to skip the bootstrap migration that creates the migration tables.
	if err := sqlx.SelectContext(ctx, reader, &tableRecs,
		"SELECT version_id FROM "+tables.MigrationClient.TableName+" WHERE version_id > 0 AND is_applied ORDER BY id ASC",
	); err != nil {
		return nil, nil, err
	}
	if err := sqlx.SelectContext(ctx, reader, &dataRecs,
		"SELECT version_id FROM "+data.MigrationClient.TableName+" WHERE version_id > 0 AND is_applied ORDER BY id ASC",
	); err != nil {
		return nil, nil, err
	}
	return tableRecs, dataRecs, nil
}

// MigrationStatus will return the current status of the migrations
// comparing the known migrations in code and the applied migrations in the database.
//
// It assumes some deployments may have performed migrations out of order.
func (ds *Datastore) MigrationStatus(ctx context.Context) (*fleet.MigrationStatus, error) {
	if tables.MigrationClient.Migrations == nil || data.MigrationClient.Migrations == nil {
		return nil, errors.New("unexpected nil migrations list")
	}
	// On a fresh PG install we must NOT call loadMigrations: it would invoke
	// goose's createVersionTable to bootstrap migration_status_tables, which
	// then collides with the CREATE TABLE for the same table in our embedded
	// pg_baseline_schema.sql when MigrateTables runs next. Detect "fresh DB"
	// by checking for the presence of the `hosts` table (always created by
	// the baseline) and short-circuit to NoMigrationsCompleted in that case
	// so prepare.go falls through to MigrateTables which applies the
	// baseline first.
	if ds.dialect.IsPostgres() {
		var hostsExists bool
		if err := ds.primary.GetContext(ctx, &hostsExists,
			`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'hosts')`); err != nil {
			return nil, ctxerr.Wrap(ctx, err, "checking PG schema")
		}
		if !hostsExists {
			return &fleet.MigrationStatus{StatusCode: fleet.NoMigrationsCompleted}, nil
		}
	}
	appliedTable, appliedData, err := ds.loadMigrations(ctx, ds.primary.DB, ds.replica)
	if err != nil {
		return nil, ctxerr.Wrap(ctx, err, "load migrations")
	}
	// This will only return a non-nil status if we detect the specific broken state from v4.73.2
	status := ds.CheckFleetv4732BadMigrations(appliedTable)
	if status != nil {
		return status, nil
	}
	return compareMigrations(
		tables.MigrationClient.Migrations,
		data.MigrationClient.Migrations,
		appliedTable,
		appliedData,
	), nil
}

// Checks for misnumbered migrations introduced in some released fleet v4.73.2 versions
func (ds *Datastore) CheckFleetv4732BadMigrations(appliedTable []int64) *fleet.MigrationStatus {
	if len(appliedTable) == 0 {
		return nil
	}
	// If the last 3 migrations are the "bad" 4.73.2 migrations and then the good 4.73.1 migration, in that order,
	// we are in the known-bad 4.73.2 state and should apply the fix
	if len(appliedTable) > 2 &&
		appliedTable[len(appliedTable)-1] == fleet4732BadMigrationID1 &&
		appliedTable[len(appliedTable)-2] == fleet4732BadMigrationID2 &&
		appliedTable[len(appliedTable)-3] == fleet4731GoodMigrationID {
		return &fleet.MigrationStatus{
			StatusCode: fleet.NeedsFleetv4732Fix,
		}
	}
	for _, v := range appliedTable {
		if v == fleet4732BadMigrationID1 || v == fleet4732BadMigrationID2 {
			return &fleet.MigrationStatus{
				StatusCode: fleet.UnknownFleetv4732State,
			}
		}
	}
	return nil
}

func (ds *Datastore) FixFleetv4732Migrations(ctx context.Context) error {
	// Update version ID of the bad migrations to the renumbered version IDs. Exactly 1 row should be affected
	// by each query
	stmt := `UPDATE ` + tables.MigrationClient.TableName + ` SET version_id = ? WHERE version_id = ?`
	return ds.withTx(ctx, func(tx sqlx.ExtContext) error {
		result, err := tx.ExecContext(ctx, stmt, fleet4732GoodMigrationID1, fleet4732BadMigrationID1)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected != 1 {
			return ctxerr.Errorf(ctx, "expected to affect 1 row for migration %d, affected %d", fleet4732BadMigrationID1, affected)
		}
		result, err = tx.ExecContext(ctx, stmt, fleet4732GoodMigrationID2, fleet4732BadMigrationID2)
		if err != nil {
			return err
		}
		affected, err = result.RowsAffected()
		if err != nil {
			return err
		}
		if affected != 1 {
			return ctxerr.Errorf(ctx, "expected to affect 1 row for migration %d, affected %d", fleet4732BadMigrationID2, affected)
		}
		return nil
	})
}

// It assumes some deployments may have performed migrations out of order.
func compareMigrations(knownTable goose.Migrations, knownData goose.Migrations, appliedTable, appliedData []int64) *fleet.MigrationStatus {
	if len(appliedTable) == 0 && len(appliedData) == 0 {
		return &fleet.MigrationStatus{
			StatusCode: fleet.NoMigrationsCompleted,
		}
	}

	missingTable, unknownTable, equalTable := compareVersions(
		getVersionsFromMigrations(knownTable),
		appliedTable,
		knownUnknownTableMigrations,
	)

	missingData, unknownData, equalData := compareVersions(
		getVersionsFromMigrations(knownData),
		appliedData,
		knownUnknownDataMigrations,
	)

	if equalData && equalTable {
		return &fleet.MigrationStatus{
			StatusCode: fleet.AllMigrationsCompleted,
		}
	}

	//
	// The following code assumes there cannot be migrations missing on
	// "table" and database being ahead on "data" (and vice-versa).
	//

	// Check for missing migrations first, as these are more important
	// to detect than the unknown migrations.
	if len(missingTable) > 0 || len(missingData) > 0 {
		return &fleet.MigrationStatus{
			StatusCode:   fleet.SomeMigrationsCompleted,
			MissingTable: missingTable,
			MissingData:  missingData,
		}
	}

	// len(unknownTable) > 0 || len(unknownData) > 0
	return &fleet.MigrationStatus{
		StatusCode:   fleet.UnknownMigrations,
		UnknownTable: unknownTable,
		UnknownData:  unknownData,
	}
}

var (
	knownUnknownTableMigrations = map[int64]struct{}{
		// This migration was introduced incorrectly in fleet-v4.4.0 and its
		// timestamp was changed in fleet-v4.4.1.
		20210924114500: {},
	}
	knownUnknownDataMigrations = map[int64]struct{}{
		// This migration was present in 2.0.0, and was removed on a subsequent release.
		// Was basically running `DELETE FROM packs WHERE deleted = 1`, (such `deleted`
		// column doesn't exist anymore).
		20171212182459: {},
		// Deleted in
		// https://github.com/fleetdm/fleet/commit/fd61dcab67f341c9e47fb6cb968171650c19a681
		20161223115449: {},
		20170309091824: {},
		20171027173700: {},
		20171212182458: {},
	}
)

func unknownUnknowns(in []int64, knownUnknowns map[int64]struct{}) []int64 {
	var result []int64
	for _, t := range in {
		if _, ok := knownUnknowns[t]; !ok {
			result = append(result, t)
		}
	}
	return result
}

// compareVersions returns any missing or extra elements in v2 with respect to v1
// (v1 or v2 need not be ordered).
func compareVersions(v1, v2 []int64, knownUnknowns map[int64]struct{}) (missing []int64, unknown []int64, equal bool) {
	v1s := make(map[int64]struct{})
	for _, m := range v1 {
		v1s[m] = struct{}{}
	}
	v2s := make(map[int64]struct{})
	for _, m := range v2 {
		v2s[m] = struct{}{}
	}
	for _, m := range v1 {
		if _, ok := v2s[m]; !ok {
			missing = append(missing, m)
		}
	}
	for _, m := range v2 {
		if _, ok := v1s[m]; !ok {
			unknown = append(unknown, m)
		}
	}
	unknown = unknownUnknowns(unknown, knownUnknowns)
	if len(missing) == 0 && len(unknown) == 0 {
		return nil, nil, true
	}
	return missing, unknown, false
}

func getVersionsFromMigrations(migrations goose.Migrations) []int64 {
	versions := make([]int64, len(migrations))
	for i := range migrations {
		versions[i] = migrations[i].Version
	}
	return versions
}

// HealthCheck returns an error if the MySQL backend is not healthy.
func (ds *Datastore) HealthCheck() error {
	// NOTE: does not receive a context as argument here, because the HealthCheck
	// interface potentially affects more than the datastore layer, and I'm not
	// sure we can safely identify and change them all at this moment.

	// Check that the primary is reachable and not in read-only mode.
	// After an AWS Aurora failover the old writer is demoted to a reader;
	// detecting this lets the health check fail so the orchestrator can restart Fleet.
	if ds.dialect.IsPostgres() {
		// PG: check if the server is in recovery (read-only replica)
		var inRecovery bool
		if err := ds.primary.QueryRowContext(context.Background(), "SELECT pg_is_in_recovery()").Scan(&inRecovery); err != nil {
			return err
		}
		if inRecovery {
			return errors.New("primary database is in recovery (read-only), possible failover detected")
		}
	} else {
		var readOnly int
		if err := ds.primary.QueryRowContext(context.Background(), "SELECT @@read_only").Scan(&readOnly); err != nil {
			return err
		}
		if readOnly == 1 {
			return errors.New("primary database is read-only, possible failover detected")
		}
	}

	if ds.readReplicaConfig != nil {
		var dst int
		if err := sqlx.GetContext(context.Background(), ds.replica, &dst, "select 1"); err != nil {
			return err
		}
	}
	return nil
}

func (ds *Datastore) closeStmts() error {
	ds.stmtCacheMu.Lock()
	defer ds.stmtCacheMu.Unlock()

	var err error
	for query, stmt := range ds.stmtCache {
		if errClose := stmt.Close(); errClose != nil {
			err = multierror.Append(err, errClose)
		}
		delete(ds.stmtCache, query)
	}
	return err
}

// Close frees resources associated with underlying mysql connection
func (ds *Datastore) Close() error {
	var err error
	if errStmt := ds.closeStmts(); errStmt != nil {
		err = multierror.Append(err, errStmt)
	}
	if errWriter := ds.primary.Close(); errWriter != nil {
		err = multierror.Append(err, errWriter)
	}
	if ds.readReplicaConfig != nil {
		if errRead := ds.replica.Close(); errRead != nil {
			err = multierror.Append(err, errRead)
		}
	}
	return err
}

// appendListOptionsToSelect will apply the given list options to ds and
// return the new select dataset.
//
// NOTE: This is a copy of appendListOptionsToSQL that uses the goqu package.
func appendListOptionsToSelect(ds *goqu.SelectDataset, opts fleet.ListOptions) *goqu.SelectDataset {
	ds = appendOrderByToSelect(ds, opts)
	ds = appendLimitOffsetToSelect(ds, opts)
	return ds
}

func appendOrderByToSelect(ds *goqu.SelectDataset, opts fleet.ListOptions) *goqu.SelectDataset {
	if opts.OrderKey != "" {
		ordersKeys := strings.Split(opts.OrderKey, ",")
		for _, key := range ordersKeys {
			sanitized := common_mysql.SanitizeColumn(key)
			if sanitized == "" {
				continue
			}

			var orderedExpr exp.OrderedExpression
			if opts.OrderDirection == fleet.OrderDescending {
				orderedExpr = goqu.L(sanitized).Desc()
			} else {
				orderedExpr = goqu.L(sanitized).Asc()
			}

			ds = ds.OrderAppend(orderedExpr)
		}
	}

	return ds
}

func appendLimitOffsetToSelect(ds *goqu.SelectDataset, opts fleet.ListOptions) *goqu.SelectDataset {
	perPage := opts.PerPage
	// If caller doesn't supply a limit apply a reasonably large default limit
	// to insure that an unbounded query with many results doesn't consume too
	// much memory or hang
	if perPage == 0 {
		perPage = fleet.DefaultPerPage
	}

	offset := perPage * opts.Page
	if offset > 0 {
		ds = ds.Offset(offset)
	}

	if opts.IncludeMetadata {
		perPage++
	}

	ds = ds.Limit(perPage)

	return ds
}

// sanitizeColumn is a facade that calls common_mysql.SanitizeColumn.
func sanitizeColumn(col string) string {
	return common_mysql.SanitizeColumn(col)
}

// appendListOptionsToSQLSecure is a facade that calls common_mysql.AppendListOptionsWithParamsSecure.
// The allowlist parameter maps user-facing order key names to actual SQL column expressions.
// This prevents SQL injection and information disclosure via arbitrary column sorting.
// See common_mysql.OrderKeyAllowlist for details.
func appendListOptionsToSQLSecure(sql string, opts *fleet.ListOptions, allowlist common_mysql.OrderKeyAllowlist) (string, []any, error) {
	return appendListOptionsWithCursorToSQLSecure(sql, nil, opts, allowlist)
}

// appendListOptionsWithCursorToSQLSecure is a facade that calls common_mysql.AppendListOptionsWithParamsSecure.
// NOTE: this method will mutate opts.PerPage if it is 0, setting it to the default value.
//
// The allowlist parameter maps user-facing order key names to actual SQL column expressions.
// This prevents SQL injection and information disclosure via arbitrary column sorting.
// See common_mysql.OrderKeyAllowlist for details.
func appendListOptionsWithCursorToSQLSecure(sql string, params []any, opts *fleet.ListOptions, allowlist common_mysql.OrderKeyAllowlist, textOrderKeys ...string) (string, []any, error) {
	if opts.PerPage == 0 {
		opts.PerPage = fleet.DefaultPerPage
	}
	return common_mysql.AppendListOptionsWithParamsSecure(sql, params, opts, allowlist, textOrderKeys...)
}

// whereFilterHostsByTeams returns the appropriate condition to use in the WHERE
// clause to render only the appropriate teams.
//
// filter provides the filtering parameters that should be used. hostKey is the
// name/alias of the hosts table to use in generating the SQL.
func (ds *Datastore) whereFilterHostsByTeams(filter fleet.TeamFilter, hostKey string) string {
	if filter.User == nil {
		// This is likely unintentional, however we would like to return no
		// results rather than panicking or returning some other error. At least
		// log.
		ds.logger.InfoContext(context.TODO(), "team filter missing user")
		return "FALSE"
	}

	defaultAllowClause := "TRUE"
	if filter.TeamID != nil {
		defaultAllowClause = fmt.Sprintf("%s.team_id = %d", hostKey, *filter.TeamID)
	}

	if filter.User.GlobalRole != nil {
		switch *filter.User.GlobalRole {
		case fleet.RoleAdmin, fleet.RoleMaintainer, fleet.RoleTechnician, fleet.RoleObserverPlus:
			return defaultAllowClause
		case fleet.RoleObserver:
			if filter.IncludeObserver {
				if filter.ObserverTeamID != nil {
					// Restrict global observer to only the specified team (e.g. the live query's own team).
					return fmt.Sprintf("%s.team_id = %d", hostKey, *filter.ObserverTeamID)
				}
				return defaultAllowClause
			}
			return "FALSE"
		default:
			// Fall through to specific teams
		}
	}

	// Collect matching teams
	var idStrs []string
	var teamIDSeen bool
	for _, team := range filter.User.Teams {
		if team.Role == fleet.RoleAdmin ||
			team.Role == fleet.RoleMaintainer ||
			team.Role == fleet.RoleTechnician ||
			team.Role == fleet.RoleObserverPlus {
			idStrs = append(idStrs, fmt.Sprint(team.ID))
			if filter.TeamID != nil && *filter.TeamID == team.ID {
				teamIDSeen = true
			}
		} else if team.Role == fleet.RoleObserver && filter.IncludeObserver {
			// When ObserverTeamID is set, restrict observer access to only that team.
			// This scopes observer_can_run to the query's own team, not all observed teams.
			if filter.ObserverTeamID == nil || *filter.ObserverTeamID == team.ID {
				idStrs = append(idStrs, fmt.Sprint(team.ID))
				if filter.TeamID != nil && *filter.TeamID == team.ID {
					teamIDSeen = true
				}
			}
		}
	}

	if len(idStrs) == 0 {
		// User has no global role and no teams allowed by includeObserver.
		return "FALSE"
	}

	if filter.TeamID != nil {
		if teamIDSeen {
			// all good, this user has the right to see the requested team
			return defaultAllowClause
		}
		return "FALSE"
	}

	return fmt.Sprintf("%s.team_id IN (%s)", hostKey, strings.Join(idStrs, ","))
}

// whereFilterTeamWithGlobalStats is the same as whereFilterHostsByTeams, it
// returns the appropriate condition to use in the WHERE clause to render only
// the appropriate teams, but is to be used when the team_id column uses "0" to
// mean "all teams including no team". This is the case e.g. for
// software_title_host_counts.
//
// filter provides the filtering parameters that should be used.
// filterTableAlias is the name/alias of the table to use in generating the
// SQL.
func (ds *Datastore) whereFilterTeamWithGlobalStats(filter fleet.TeamFilter, filterTableAlias string) string {
	globalFilter := fmt.Sprintf("%s.team_id = 0 AND %[1]s.global_stats = true", filterTableAlias)
	teamIDFilter := fmt.Sprintf("%s.team_id", filterTableAlias)
	return ds.whereFilterGlobalOrTeamIDByTeamsWithSqlFilter(filter, globalFilter, teamIDFilter)
}

func (ds *Datastore) whereFilterGlobalOrTeamIDByTeamsWithSqlFilter(
	filter fleet.TeamFilter, globalSqlFilter string, teamIDSqlFilter string,
) string {
	if filter.User == nil {
		// This is likely unintentional, however we would like to return no
		// results rather than panicking or returning some other error. At least
		// log.
		ds.logger.InfoContext(context.TODO(), "team filter missing user")
		return "FALSE"
	}

	defaultAllowClause := globalSqlFilter
	if filter.TeamID != nil {
		defaultAllowClause = fmt.Sprintf("%s = %d", teamIDSqlFilter, *filter.TeamID)
	}

	if filter.User.GlobalRole != nil {
		switch *filter.User.GlobalRole {
		case fleet.RoleAdmin, fleet.RoleMaintainer, fleet.RoleTechnician, fleet.RoleObserverPlus:
			return defaultAllowClause
		case fleet.RoleObserver:
			if filter.IncludeObserver {
				return defaultAllowClause
			}
			return "FALSE"
		default:
			// Fall through to specific teams
		}
	}

	// Collect matching teams
	var idStrs []string
	var teamIDSeen bool
	for _, team := range filter.User.Teams {
		if team.Role == fleet.RoleAdmin ||
			team.Role == fleet.RoleMaintainer ||
			team.Role == fleet.RoleTechnician ||
			team.Role == fleet.RoleObserverPlus ||
			(team.Role == fleet.RoleObserver && filter.IncludeObserver) {
			idStrs = append(idStrs, fmt.Sprint(team.ID))
			if filter.TeamID != nil && *filter.TeamID == team.ID {
				teamIDSeen = true
			}
		}
	}

	if len(idStrs) == 0 {
		// User has no global role and no teams allowed by includeObserver.
		return "FALSE"
	}

	if filter.TeamID != nil {
		if teamIDSeen {
			// all good, this user has the right to see the requested team
			return defaultAllowClause
		}
		return "FALSE"
	}

	return fmt.Sprintf("%s IN (%s)", teamIDSqlFilter, strings.Join(idStrs, ","))
}

// whereFilterTeams returns the appropriate condition to use in the WHERE
// clause to render only the appropriate teams.
//
// filter provides the filtering parameters that should be used. teamKey is the
// name/alias of the teams table to use in generating the SQL.
func (ds *Datastore) whereFilterTeams(filter fleet.TeamFilter, teamKey string) string {
	if filter.User == nil {
		// This is likely unintentional, however we would like to return no
		// results rather than panicking or returning some other error. At least
		// log.
		ds.logger.InfoContext(context.TODO(), "team filter missing user")
		return "FALSE"
	}

	if filter.User.GlobalRole != nil {
		switch *filter.User.GlobalRole {
		case fleet.RoleAdmin, fleet.RoleMaintainer, fleet.RoleTechnician, fleet.RoleGitOps, fleet.RoleObserverPlus:
			return "TRUE"
		case fleet.RoleObserver:
			if filter.IncludeObserver {
				return "TRUE"
			}
			return "FALSE"
		default:
			// Fall through to specific teams
		}
	}

	// Collect matching teams
	var idStrs []string
	for _, team := range filter.User.Teams {
		if team.Role == fleet.RoleAdmin ||
			team.Role == fleet.RoleMaintainer ||
			team.Role == fleet.RoleTechnician ||
			team.Role == fleet.RoleGitOps ||
			team.Role == fleet.RoleObserverPlus ||
			(team.Role == fleet.RoleObserver && filter.IncludeObserver) {
			idStrs = append(idStrs, fmt.Sprint(team.ID))
		}
	}

	if len(idStrs) == 0 {
		// User has no global role and no teams allowed by includeObserver.
		return "FALSE"
	}

	return fmt.Sprintf("%s.id IN (%s)", teamKey, strings.Join(idStrs, ","))
}

// whereOmitIDs returns the appropriate condition to use in the WHERE
// clause to omit the provided IDs from the selection.
func (ds *Datastore) whereOmitIDs(colName string, omit []uint) string {
	if len(omit) == 0 {
		return "TRUE"
	}

	var idStrs []string
	for _, id := range omit {
		idStrs = append(idStrs, fmt.Sprint(id))
	}

	return fmt.Sprintf("%s NOT IN (%s)", colName, strings.Join(idStrs, ","))
}

// registerTLS adds client certificate configuration to the mysql connection.
func registerTLS(conf config.MysqlConfig) error {
	tlsCfg := config.TLS{
		TLSCert:       conf.TLSCert,
		TLSKey:        conf.TLSKey,
		TLSCA:         conf.TLSCA,
		TLSServerName: conf.TLSServerName,
	}
	cfg, err := tlsCfg.ToTLSConfig()
	if err != nil {
		return err
	}
	if err := mysql.RegisterTLSConfig(conf.TLSConfig, cfg); err != nil {
		return fmt.Errorf("register mysql tls config: %w", err)
	}
	return nil
}

// isForeignKeyError checks if the provided error is a child foreign-key
// violation on either dialect: MySQL ER_NO_REFERENCED_ROW_2 (1452) or PG
// SQLSTATE 23503 (foreign_key_violation).
func isChildForeignKeyError(err error) bool {
	err = ctxerr.Cause(err)
	if mysqlErr, ok := err.(*mysql.MySQLError); ok {
		// https://dev.mysql.com/doc/refman/5.7/en/error-messages-server.html#error_er_no_referenced_row_2
		const ER_NO_REFERENCED_ROW_2 = 1452
		return mysqlErr.Number == ER_NO_REFERENCED_ROW_2
	}
	// PG: pgconn.PgError with SQLSTATE 23503.
	return pg.IsForeignKey(err)
}

type patternReplacer func(string) string

// likePattern returns a pattern to match m with LIKE.
func likePattern(m string) string {
	m = strings.ReplaceAll(m, "_", "\\_")
	m = strings.ReplaceAll(m, "%", "\\%")
	return "%" + m + "%"
}

// noneReplacer doesn't manipulate
func noneReplacer(m string) string {
	return m
}

// searchLike adds SQL and parameters for a "search" using LIKE syntax.
//
// The input columns must be sanitized if they are provided by the user.
func searchLike(sql string, params []interface{}, match string, columns ...string) (string, []interface{}) {
	return searchLikePattern(sql, params, match, likePattern, columns...)
}

func searchLikePattern(sql string, params []interface{}, match string, replacer patternReplacer, columns ...string) (string, []interface{}) {
	if len(columns) == 0 || len(match) == 0 {
		return sql, params
	}

	pattern := replacer(match)
	ors := make([]string, 0, len(columns))
	for _, column := range columns {
		ors = append(ors, column+" LIKE ?")
		params = append(params, pattern)
	}

	sql += " AND (" + strings.Join(ors, " OR ") + ")"
	return sql, params
}

/*
This regex matches any occurrence of a character from the ASCII character set followed by one or more characters that are not from the ASCII character set.
The first part `[[:ascii:]]` matches any character that is within the ASCII range (0 to 127 in the ASCII table),
while the second part `[^[:ascii:]]` matches any character that is not within the ASCII range.
So, when these two parts are combined with no space in between, the resulting regex matches any
sequence of characters where the first character is within the ASCII range and the following characters are not within the ASCII range.
*/
var (
	nonascii        = regexp.MustCompile(`(?P<ascii>[[:ascii:]])(?P<nonascii>[^[:ascii:]]+)`)
	nonacsiiReplace = regexp.MustCompile(`[^[:ascii:]]`)
)

// hostSearchLike searches hosts based on the given columns plus searching in hosts_emails. Note:
// the host from the `hosts` table must be aliased to `h` in `sql`.
func hostSearchLike(sql string, params []any, match string, columns ...string) (string, []any) {
	base, args := searchLike(sql, params, match, columns...)

	// Always search in host_emails table in addition to the provided columns,
	// so that any search query can surface results from human-host mapping information.
	if len(match) > 0 && len(columns) > 0 {
		// remove the closing paren and add the email condition to the list
		base = strings.TrimSuffix(base, ")") + " OR (" + ` EXISTS (SELECT 1 FROM host_emails he WHERE he.host_id = h.id AND he.email LIKE ?)))`
		args = append(args, likePattern(match))
	}
	return base, args
}

func hostSearchLikeAny(sql string, params []interface{}, match string, columns ...string) (string, []interface{}) {
	return searchLikePattern(sql, params, buildWildcardMatchPhrase(match), noneReplacer, columns...)
}

func buildWildcardMatchPhrase(matchQuery string) string {
	return replaceMatchAny(likePattern(matchQuery))
}

func hasNonASCIIRegex(s string) bool {
	return nonascii.MatchString(s)
}

func replaceMatchAny(s string) string {
	return nonacsiiReplace.ReplaceAllString(s, "_")
}

func (ds *Datastore) InnoDBStatus(ctx context.Context) (string, error) {
	// No InnoDB on PostgreSQL; report a placeholder rather than relying on
	// the driver to no-op the SHOW ENGINE statement (which broke scanning).
	if ds.dialect.IsPostgres() {
		return "n/a (PostgreSQL backend — no InnoDB engine status)", nil
	}
	status := struct {
		Type   string `db:"Type"`
		Name   string `db:"Name"`
		Status string `db:"Status"`
	}{}
	// using the writer even when doing a read to get the data from the main db node
	err := ds.writer(ctx).GetContext(ctx, &status, "show engine innodb status")
	if err != nil {
		// To read innodb tables, DB user must have PROCESS privilege
		// This can be set by DB admin like: GRANT PROCESS,SELECT ON *.* TO 'fleet'@'%';
		if isMySQLAccessDenied(err) {
			return "", &accessDeniedError{
				Message:     "getting innodb status: DB user must have global PROCESS and SELECT privilege",
				InternalErr: err,
			}
		}
		return "", ctxerr.Wrap(ctx, err, "getting innodb status")
	}
	return status.Status, nil
}

func (ds *Datastore) ProcessList(ctx context.Context) ([]fleet.MySQLProcess, error) {
	var processList []fleet.MySQLProcess
	// using the writer even when doing a read to get the data from the main db node
	err := ds.writer(ctx).SelectContext(ctx, &processList, "show processlist")
	if err != nil {
		return nil, ctxerr.Wrap(ctx, err, "Getting process list")
	}
	return processList, nil
}

// insertOnDuplicateDidInsertOrUpdate returns true if an INSERT ON DUPLICATE KEY
// UPDATE actually inserted or updated a row (vs no-op).
// MySQL: checks LastInsertId (non-zero on insert) AND RowsAffected (> 0).
// PostgreSQL: LastInsertId is not available, so just checks RowsAffected > 0.
func insertOnDuplicateDidInsertOrUpdate(res sql.Result) bool {
	// From mysql's documentation:
	//
	// With ON DUPLICATE KEY UPDATE, the affected-rows value per row is 1 if
	// the row is inserted as a new row, 2 if an existing row is updated, and
	// 0 if an existing row is set to its current values. If you specify the
	// CLIENT_FOUND_ROWS flag to the mysql_real_connect() C API function when
	// connecting to mysqld, the affected-rows value is 1 (not 0) if an
	// existing row is set to its current values.
	//
	// If a table contains an AUTO_INCREMENT column and INSERT ... ON DUPLICATE KEY UPDATE
	// inserts or updates a row, the LAST_INSERT_ID() function returns the AUTO_INCREMENT value.
	//
	// https://dev.mysql.com/doc/refman/8.4/en/insert-on-duplicate.html
	//
	// Note that connection string sets CLIENT_FOUND_ROWS (see
	// generateMysqlConnectionString in this package), so it does return 1 when
	// an existing row is set to its current values, but with a last inserted id
	// of 0.
	//
	// Also note that with our mysql driver, Result.LastInsertId and
	// Result.RowsAffected can never return an error, they are retrieved at the
	// time of the Exec call, and the result simply returns the integers it
	// already holds:
	// https://github.com/go-sql-driver/mysql/blob/bcc459a906419e2890a50fc2c99ea6dd927a88f2/result.go

	// PostgreSQL contract: the statement MUST be built with
	// dialect.OnDuplicateKeyGuarded so an identical re-upsert affects zero
	// rows. ON CONFLICT DO UPDATE otherwise rewrites the row unconditionally,
	// and for identity tables the rebind driver's RETURNING support makes
	// LastInsertId succeed with the row's ID — both branches below would then
	// report an unconditional true. With the guard: insert → aff=1 with a
	// non-zero returned ID; changed update → aff=1; identical re-upsert →
	// aff=0 and no returned row → false, matching MySQL.
	aff, _ := res.RowsAffected()
	lastID, err := res.LastInsertId()
	if err != nil {
		// PostgreSQL, non-identity table (no RETURNING) — RowsAffected alone
		// distinguishes the cases when the statement is guarded.
		return aff > 0
	}
	// MySQL: something was inserted (lastID != 0) AND row was found (aff > 0).
	// PG identity tables: the guard makes a no-op return zero rows, so
	// lastID == 0 and aff == 0.
	return lastID != 0 && aff > 0
}

type parameterizedStmt struct {
	Statement string
	Args      []interface{}
}

// optimisticGetOrInsert encodes an efficient pattern of looking up a row's ID
// for a unique key that is more likely to already exist (i.e. the insert
// should be infrequent, the read should succeed most of the time).
// It proceeds as follows:
//  1. Try to read the ID from the read replica.
//  2. If it does not exist, try to insert the row in the primary.
//  3. If it fails due to a duplicate key, try to read the ID again, this
//     time from the primary.
//
// The read statement must only SELECT the id column.
func (ds *Datastore) optimisticGetOrInsert(ctx context.Context, readStmt, insertStmt *parameterizedStmt) (id uint, err error) {
	return ds.optimisticGetOrInsertWithWriter(ctx, ds.writer(ctx), readStmt, insertStmt)
}

// optimisticGetOrInsertWithWriter is the same as optimisticGetOrInsert but it
// uses the provided writer to perform the insert or second read operations.
// This makes it possible to use this from inside a transaction.
func (ds *Datastore) optimisticGetOrInsertWithWriter(ctx context.Context, writer sqlx.ExtContext, readStmt, insertStmt *parameterizedStmt) (id uint, err error) { //nolint: gocritic // it's ok in this case to use ds.reader even if we receive an ExtContext
	readID := func(q sqlx.QueryerContext) (uint, error) {
		var id uint
		err := sqlx.GetContext(ctx, q, &id, readStmt.Statement, readStmt.Args...)
		return id, err
	}

	// 1. read from the read replica, as it is likely to already exist
	id, err = readID(ds.reader(ctx))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// this does not exist yet, try to insert it
			insertedID, err := insertAndGetIDTx(ctx, writer, ds.dialect, insertStmt.Statement, insertStmt.Args...)
			if err != nil {
				if ds.dialect.IsDuplicate(err) {
					// it might've been created between the select and the insert, read
					// again this time from the primary database connection.
					id, err := readID(writer)
					if err != nil {
						return 0, ctxerr.Wrap(ctx, err, "get id from writer")
					}
					return id, nil
				}
				return 0, ctxerr.Wrap(ctx, err, "insert")
			}
			return uint(insertedID), nil //nolint:gosec // dismiss G115
		}
		return 0, ctxerr.Wrap(ctx, err, "get id from reader")
	}
	return id, nil
}

// batchProcessDB abstracts the batch processing logic, for a given payload:
//
// - generateValueArgs will get called for each item, the expected return values are:
//   - a string containing the placeholders for each item in the batch
//   - a slice of arguments containing one item for each placeholder
//
// - executeBatch will get called on each batch to perform the operation in the db
//
// TODO(roberto): use this function in all the functions where we do ad-hoc
// batch processing.
func batchProcessDB[T any](
	payload []T,
	batchSize int,
	generateValueArgs func(T) (string, []any),
	executeBatch func(string, []any) error,
) error {
	if len(payload) == 0 {
		return nil
	}

	var (
		args       []any
		sb         strings.Builder
		batchCount int
	)

	resetBatch := func() {
		batchCount = 0
		args = args[:0]
		sb.Reset()
	}

	for _, item := range payload {
		valuePart, itemArgs := generateValueArgs(item)
		args = append(args, itemArgs...)
		sb.WriteString(valuePart)
		batchCount++

		if batchCount >= batchSize {
			if err := executeBatch(sb.String(), args); err != nil {
				return err
			}
			resetBatch()
		}
	}

	if batchCount > 0 {
		if err := executeBatch(sb.String(), args); err != nil {
			return err
		}
	}
	return nil
}
