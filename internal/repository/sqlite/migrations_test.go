package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	glebarezsqlite "github.com/glebarez/sqlite"
	mysqlDriver "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestIsMySQLDuplicateIndexError(t *testing.T) {
	if !isMySQLDuplicateIndexError(&mysqlDriver.MySQLError{Number: 1061, Message: "Duplicate key name"}) {
		t.Fatalf("expected true for ER_DUP_KEYNAME(1061)")
	}
	if isMySQLDuplicateIndexError(&mysqlDriver.MySQLError{Number: 1146, Message: "Table doesn't exist"}) {
		t.Fatalf("expected false for non-duplicate mysql error")
	}
	if !isMySQLDuplicateIndexError(errors.New("Error 1061: Duplicate key name 'idx_proxy_requests_provider_id'")) {
		t.Fatalf("expected true for duplicate key name string match fallback")
	}
	if isMySQLDuplicateIndexError(errors.New("some other error")) {
		t.Fatalf("expected false for unrelated error")
	}
}

func TestIsMySQLMissingIndexError(t *testing.T) {
	if !isMySQLMissingIndexError(&mysqlDriver.MySQLError{Number: 1091, Message: "Can't DROP"}) {
		t.Fatalf("expected true for ER_CANT_DROP_FIELD_OR_KEY(1091)")
	}
	if !isMySQLMissingIndexError(errors.New("Error 1091: Can't DROP 'idx_x'; check that column/key exists")) {
		t.Fatalf("expected true for missing index string match fallback")
	}
	if isMySQLMissingIndexError(errors.New("some other error")) {
		t.Fatalf("expected false for unrelated error")
	}
}

func TestMySQLSupportsInstantAddColumn(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{version: "8.0.11", want: false},
		{version: "8.0.12", want: true},
		{version: "8.0.12-log", want: true},
		{version: "8.4.1", want: true},
		{version: "9.0.0", want: true},
		{version: "5.7.44-log", want: false},
		{version: "10.11.6-MariaDB", want: false},
		{version: "8.0", want: false},
		{version: "8.x.12", want: false},
		{version: "invalid", want: false},
	}

	for _, tt := range tests {
		if got := mysqlSupportsInstantAddColumn(tt.version); got != tt.want {
			t.Errorf("mysqlSupportsInstantAddColumn(%q) = %v, want %v", tt.version, got, tt.want)
		}
	}
}

func TestIsDuplicateColumnError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "mysql duplicate field", err: &mysqlDriver.MySQLError{Number: 1060, Message: "Duplicate column name"}, want: true},
		{name: "mysql unrelated", err: &mysqlDriver.MySQLError{Number: 1146, Message: "Table doesn't exist"}, want: false},
		{name: "sqlite duplicate", err: errors.New("duplicate column name: reasoning_effort"), want: true},
		{name: "postgres duplicate", err: errors.New(`column "reasoning_effort" already exists`), want: true},
		{name: "unrelated", err: errors.New("connection refused"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDuplicateColumnError(tt.err); got != tt.want {
				t.Fatalf("isDuplicateColumnError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestReasoningEffortColumnIsExcludedFromAutoMigrate(t *testing.T) {
	db := openRawSQLiteDB(t)
	if err := db.AutoMigrate(&ProxyRequest{}); err != nil {
		t.Fatalf("AutoMigrate ProxyRequest: %v", err)
	}
	if db.Migrator().HasColumn(&ProxyRequest{}, "reasoning_effort") {
		t.Fatal("reasoning_effort must be added by guarded migration v18, not AutoMigrate")
	}
}

func TestReasoningEffortMigrationIsLatestAndUnique(t *testing.T) {
	lastVersion := 0
	v18Count := 0
	for _, migration := range migrations {
		if migration.Version <= lastVersion {
			t.Fatalf("migrations are not strictly ordered: v%d follows v%d", migration.Version, lastVersion)
		}
		lastVersion = migration.Version
		if migration.Version == 18 {
			v18Count++
		}
	}
	if lastVersion != 18 {
		t.Fatalf("latest migration = v%d, want v18", lastVersion)
	}
	if v18Count != 1 {
		t.Fatalf("migration v18 registered %d times, want once", v18Count)
	}
}

func TestProxyRequestsExceedReasoningMigrationThreshold(t *testing.T) {
	db := openRawSQLiteDB(t)
	if err := db.AutoMigrate(&ProxyRequest{}); err != nil {
		t.Fatalf("AutoMigrate ProxyRequest: %v", err)
	}

	insert := func(index int) {
		t.Helper()
		model := &ProxyRequest{TenantID: 1, RequestID: fmt.Sprintf("threshold-%d", index)}
		if err := db.Omit("reasoning_effort").Create(model).Error; err != nil {
			t.Fatalf("insert threshold fixture %d: %v", index, err)
		}
	}

	for i := 1; i <= 3; i++ {
		insert(i)
	}
	exceeds, err := proxyRequestsExceedReasoningMigrationThreshold(db, 3)
	if err != nil {
		t.Fatalf("probe exact threshold: %v", err)
	}
	if exceeds {
		t.Fatal("exactly threshold rows must not be treated as exceeding the threshold")
	}

	insert(4)
	exceeds, err = proxyRequestsExceedReasoningMigrationThreshold(db, 3)
	if err != nil {
		t.Fatalf("probe above threshold: %v", err)
	}
	if !exceeds {
		t.Fatal("threshold+1 rows must be treated as exceeding the threshold")
	}
}

func TestProxyRequestsExceedReasoningMigrationThresholdReturnsProbeError(t *testing.T) {
	db := openRawSQLiteDB(t)
	if _, err := proxyRequestsExceedReasoningMigrationThreshold(db, 1); err == nil {
		t.Fatal("expected missing proxy_requests table to return an error")
	}
}

func TestReasoningEffortMigrationPreservesHistoricalNullAndIsIdempotent(t *testing.T) {
	db := openRawSQLiteDB(t)
	if err := db.AutoMigrate(&ProxyRequest{}); err != nil {
		t.Fatalf("AutoMigrate ProxyRequest: %v", err)
	}
	historical := &ProxyRequest{TenantID: 1, RequestID: "historical-before-v18"}
	if err := db.Omit("reasoning_effort").Create(historical).Error; err != nil {
		t.Fatalf("insert historical request: %v", err)
	}

	migration := findMigrationByVersion(t, 18)
	if err := migration.Up(db); err != nil {
		t.Fatalf("run migration v18: %v", err)
	}
	if !db.Migrator().HasColumn(&ProxyRequest{}, "reasoning_effort") {
		t.Fatal("migration v18 did not add reasoning_effort")
	}

	var effort sql.NullString
	if err := db.Raw("SELECT reasoning_effort FROM proxy_requests WHERE id = ?", historical.ID).Row().Scan(&effort); err != nil {
		t.Fatalf("read historical reasoning_effort: %v", err)
	}
	if effort.Valid {
		t.Fatalf("historical reasoning_effort = %q, want SQL NULL", effort.String)
	}

	if err := migration.Up(db); err != nil {
		t.Fatalf("rerun migration v18: %v", err)
	}
	var count int64
	if err := db.Model(&ProxyRequest{}).Count(&count).Error; err != nil {
		t.Fatalf("count requests after idempotent migration: %v", err)
	}
	if count != 1 {
		t.Fatalf("request count after idempotent migration = %d, want 1", count)
	}
}

func TestReasoningEffortMigrationMySQL(t *testing.T) {
	t.Run("column already exists", func(t *testing.T) {
		db, mock := openMockMySQLDB(t)
		expectMySQLReasoningColumnLookup(mock, 1)

		if err := runReasoningEffortColumnMigration(db); err != nil {
			t.Fatalf("run migration: %v", err)
		}
		assertSQLMockExpectations(t, mock)
	})

	t.Run("non-mysql duplicate column is idempotent", func(t *testing.T) {
		db, mock := openMockSQLDB(t, "sqlite")
		expectMySQLReasoningColumnLookup(mock, 0)
		mock.ExpectExec(regexp.QuoteMeta(reasoningEffortColumnDDL)).
			WillReturnError(errors.New("duplicate column name: reasoning_effort"))

		if err := runReasoningEffortColumnMigration(db); err != nil {
			t.Fatalf("run migration: %v", err)
		}
		assertSQLMockExpectations(t, mock)
	})

	t.Run("non-mysql add column error is returned", func(t *testing.T) {
		db, mock := openMockSQLDB(t, "sqlite")
		expectMySQLReasoningColumnLookup(mock, 0)
		wantErr := errors.New("add column failed")
		mock.ExpectExec(regexp.QuoteMeta(reasoningEffortColumnDDL)).WillReturnError(wantErr)

		if err := runReasoningEffortColumnMigration(db); !errors.Is(err, wantErr) {
			t.Fatalf("run migration error = %v, want %v", err, wantErr)
		}
		assertSQLMockExpectations(t, mock)
	})

	t.Run("mysql 8 uses instant add column", func(t *testing.T) {
		db, mock := openMockMySQLDB(t)
		expectMySQLReasoningColumnLookup(mock, 0)
		mock.ExpectQuery(regexp.QuoteMeta("SELECT VERSION()")).
			WillReturnRows(sqlmock.NewRows([]string{"VERSION()"}).AddRow("8.0.36"))
		mock.ExpectExec(regexp.QuoteMeta(reasoningEffortColumnDDL + ", ALGORITHM=INSTANT")).
			WillReturnResult(sqlmock.NewResult(0, 0))

		if err := runReasoningEffortColumnMigration(db); err != nil {
			t.Fatalf("run migration: %v", err)
		}
		assertSQLMockExpectations(t, mock)
	})

	t.Run("version query failure uses guarded fallback", func(t *testing.T) {
		db, mock := openMockMySQLDB(t)
		expectMySQLReasoningColumnLookup(mock, 0)
		mock.ExpectQuery(regexp.QuoteMeta("SELECT VERSION()")).
			WillReturnError(errors.New("version unavailable"))
		expectReasoningThresholdProbe(mock, false)
		mock.ExpectExec(regexp.QuoteMeta(reasoningEffortColumnDDL)).
			WillReturnResult(sqlmock.NewResult(0, 0))

		if err := runReasoningEffortColumnMigration(db); err != nil {
			t.Fatalf("run migration: %v", err)
		}
		assertSQLMockExpectations(t, mock)
	})

	t.Run("mysql 5.7 small table uses guarded add column", func(t *testing.T) {
		db, mock := openMockMySQLDB(t)
		expectMySQLReasoningColumnLookup(mock, 0)
		mock.ExpectQuery(regexp.QuoteMeta("SELECT VERSION()")).
			WillReturnRows(sqlmock.NewRows([]string{"VERSION()"}).AddRow("5.7.44-log"))
		expectReasoningThresholdProbe(mock, false)
		mock.ExpectExec(regexp.QuoteMeta(reasoningEffortColumnDDL)).
			WillReturnResult(sqlmock.NewResult(0, 0))

		if err := runReasoningEffortColumnMigration(db); err != nil {
			t.Fatalf("run migration: %v", err)
		}
		assertSQLMockExpectations(t, mock)
	})

	t.Run("guarded add duplicate column is idempotent", func(t *testing.T) {
		db, mock := openMockMySQLDB(t)
		expectMySQLReasoningColumnLookup(mock, 0)
		mock.ExpectQuery(regexp.QuoteMeta("SELECT VERSION()")).
			WillReturnRows(sqlmock.NewRows([]string{"VERSION()"}).AddRow("5.7.44-log"))
		expectReasoningThresholdProbe(mock, false)
		mock.ExpectExec(regexp.QuoteMeta(reasoningEffortColumnDDL)).
			WillReturnError(&mysqlDriver.MySQLError{Number: 1060, Message: "Duplicate column name 'reasoning_effort'"})

		if err := runReasoningEffortColumnMigration(db); err != nil {
			t.Fatalf("run migration: %v", err)
		}
		assertSQLMockExpectations(t, mock)
	})

	t.Run("guarded add error is returned", func(t *testing.T) {
		db, mock := openMockMySQLDB(t)
		expectMySQLReasoningColumnLookup(mock, 0)
		mock.ExpectQuery(regexp.QuoteMeta("SELECT VERSION()")).
			WillReturnRows(sqlmock.NewRows([]string{"VERSION()"}).AddRow("5.7.44-log"))
		expectReasoningThresholdProbe(mock, false)
		wantErr := errors.New("blocking alter failed")
		mock.ExpectExec(regexp.QuoteMeta(reasoningEffortColumnDDL)).WillReturnError(wantErr)

		if err := runReasoningEffortColumnMigration(db); !errors.Is(err, wantErr) {
			t.Fatalf("run migration error = %v, want %v", err, wantErr)
		}
		assertSQLMockExpectations(t, mock)
	})

	t.Run("instant failure falls back for a small table", func(t *testing.T) {
		db, mock := openMockMySQLDB(t)
		expectMySQLReasoningColumnLookup(mock, 0)
		mock.ExpectQuery(regexp.QuoteMeta("SELECT VERSION()")).
			WillReturnRows(sqlmock.NewRows([]string{"VERSION()"}).AddRow("8.0.36"))
		mock.ExpectExec(regexp.QuoteMeta(reasoningEffortColumnDDL + ", ALGORITHM=INSTANT")).
			WillReturnError(&mysqlDriver.MySQLError{Number: 1846, Message: "ALGORITHM=INSTANT is not supported"})
		expectReasoningThresholdProbe(mock, false)
		mock.ExpectExec(regexp.QuoteMeta(reasoningEffortColumnDDL)).
			WillReturnResult(sqlmock.NewResult(0, 0))

		if err := runReasoningEffortColumnMigration(db); err != nil {
			t.Fatalf("run migration: %v", err)
		}
		assertSQLMockExpectations(t, mock)
	})

	t.Run("large table skips blocking fallback", func(t *testing.T) {
		db, mock := openMockMySQLDB(t)
		expectMySQLReasoningColumnLookup(mock, 0)
		mock.ExpectQuery(regexp.QuoteMeta("SELECT VERSION()")).
			WillReturnRows(sqlmock.NewRows([]string{"VERSION()"}).AddRow("5.7.44-log"))
		expectReasoningThresholdProbe(mock, true)

		if err := runReasoningEffortColumnMigration(db); err != nil {
			t.Fatalf("run migration: %v", err)
		}
		assertSQLMockExpectations(t, mock)
	})

	t.Run("probe failure safely skips fallback", func(t *testing.T) {
		db, mock := openMockMySQLDB(t)
		expectMySQLReasoningColumnLookup(mock, 0)
		mock.ExpectQuery(regexp.QuoteMeta("SELECT VERSION()")).
			WillReturnRows(sqlmock.NewRows([]string{"VERSION()"}).AddRow("5.7.44-log"))
		mock.ExpectQuery(regexp.QuoteMeta("SELECT 1 FROM proxy_requests ORDER BY id LIMIT 1 OFFSET ?")).
			WithArgs(detailCleanupIndexRowThreshold).
			WillReturnError(errors.New("probe failed"))

		if err := runReasoningEffortColumnMigration(db); err != nil {
			t.Fatalf("run migration: %v", err)
		}
		assertSQLMockExpectations(t, mock)
	})

	t.Run("concurrent duplicate column is idempotent", func(t *testing.T) {
		db, mock := openMockMySQLDB(t)
		expectMySQLReasoningColumnLookup(mock, 0)
		mock.ExpectQuery(regexp.QuoteMeta("SELECT VERSION()")).
			WillReturnRows(sqlmock.NewRows([]string{"VERSION()"}).AddRow("8.0.36"))
		mock.ExpectExec(regexp.QuoteMeta(reasoningEffortColumnDDL + ", ALGORITHM=INSTANT")).
			WillReturnError(&mysqlDriver.MySQLError{Number: 1060, Message: "Duplicate column name 'reasoning_effort'"})

		if err := runReasoningEffortColumnMigration(db); err != nil {
			t.Fatalf("run migration: %v", err)
		}
		assertSQLMockExpectations(t, mock)
	})
}

func openMockMySQLDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	return openMockSQLDB(t, "mysql")
}

type testNamedDialector struct {
	gorm.Dialector
	name string
}

func (d testNamedDialector) Name() string {
	return d.name
}

func openMockSQLDB(t *testing.T, dialectorName string) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock: %v", err)
	}
	t.Cleanup(func() {
		_ = rawDB.Close()
	})

	var dialector gorm.Dialector = gormmysql.New(gormmysql.Config{
		Conn:                      rawDB,
		SkipInitializeWithVersion: true,
	})
	if dialectorName != "mysql" {
		dialector = testNamedDialector{Dialector: dialector, name: dialectorName}
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		DisableAutomaticPing: true,
		Logger:               logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open mock GORM MySQL database: %v", err)
	}
	return db, mock
}

func expectMySQLReasoningColumnLookup(mock sqlmock.Sqlmock, count int64) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT DATABASE()")).
		WillReturnRows(sqlmock.NewRows([]string{"DATABASE()"}).AddRow("maxx"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT SCHEMA_NAME from Information_schema.SCHEMATA where SCHEMA_NAME LIKE ? ORDER BY SCHEMA_NAME=? DESC,SCHEMA_NAME limit 1")).
		WithArgs("maxx%", "maxx").
		WillReturnRows(sqlmock.NewRows([]string{"SCHEMA_NAME"}).AddRow("maxx"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM INFORMATION_SCHEMA.columns WHERE table_schema = ? AND table_name = ? AND column_name = ?")).
		WithArgs("maxx", "proxy_requests", "reasoning_effort").
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(count))
}

func expectReasoningThresholdProbe(mock sqlmock.Sqlmock, exceeds bool) {
	rows := sqlmock.NewRows([]string{"1"})
	if exceeds {
		rows.AddRow(1)
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT 1 FROM proxy_requests ORDER BY id LIMIT 1 OFFSET ?")).
		WithArgs(detailCleanupIndexRowThreshold).
		WillReturnRows(rows)
}

func assertSQLMockExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestDedupeCodexQuotaIdentityRows(t *testing.T) {
	gormDB := openRawSQLiteDB(t)
	prepareCodexQuotaDedupeFixture(t, gormDB)

	if err := dedupeCodexQuotaIdentityRows(gormDB); err != nil {
		t.Fatalf("dedupe identities: %v", err)
	}

	assertCodexQuotaFixtureCounts(t, gormDB)
}

func TestCodexQuotaIdentityMigrationV8Up(t *testing.T) {
	gormDB := openRawSQLiteDB(t)
	prepareCodexQuotaMigrationFixture(t, gormDB)

	migration := findMigrationByVersion(t, 8)
	if err := migration.Up(gormDB); err != nil {
		t.Fatalf("run migration v8 up: %v", err)
	}

	assertCodexQuotaPostMigrationState(t, gormDB)
	assertIndexExists(t, gormDB, "idx_codex_quotas_tenant_identity", true)
	assertIndexExists(t, gormDB, "idx_codex_quotas_email", false)
	assertIndexMissing(t, gormDB, "idx_codex_quotas_tenant_email")
}

func TestCodexQuotaIdentityMigrationV8UpPreservesDeletedHistory(t *testing.T) {
	gormDB := openRawSQLiteDB(t)
	prepareCodexQuotaMigrationFixtureWithDeletedHistory(t, gormDB)

	migration := findMigrationByVersion(t, 8)
	if err := migration.Up(gormDB); err != nil {
		t.Fatalf("run migration v8 up with deleted history: %v", err)
	}

	var activeCount int64
	if err := gormDB.Raw(`
		SELECT COUNT(*)
		FROM codex_quotas
		WHERE tenant_id = 1
		  AND identity_key = 'account:acct-1'
		  AND deleted_at = 0
	`).Scan(&activeCount).Error; err != nil {
		t.Fatalf("count active rows: %v", err)
	}
	if activeCount != 1 {
		t.Fatalf("expected one active row to remain, got %d", activeCount)
	}

	var deletedCount int64
	if err := gormDB.Raw(`
		SELECT COUNT(*)
		FROM codex_quotas
		WHERE tenant_id = 1
		  AND identity_key = 'account:acct-1'
		  AND deleted_at != 0
	`).Scan(&deletedCount).Error; err != nil {
		t.Fatalf("count deleted rows: %v", err)
	}
	if deletedCount != 1 {
		t.Fatalf("expected deleted historical row to be preserved, got %d", deletedCount)
	}
}

func TestCodexQuotaIdentityMigrationV8UpHandlesPreexistingIdentityIndex(t *testing.T) {
	gormDB := openRawSQLiteDB(t)
	prepareCodexQuotaMigrationFixtureWithPreexistingIdentityIndex(t, gormDB)

	migration := findMigrationByVersion(t, 8)
	if err := migration.Up(gormDB); err != nil {
		t.Fatalf("run migration v8 up with preexisting identity index: %v", err)
	}

	var count int64
	if err := gormDB.Raw(`
		SELECT COUNT(*)
		FROM codex_quotas
		WHERE tenant_id = 1
		  AND identity_key = 'account:e94ce011-80f4-490b-b285-d3109db72b0e'
		  AND deleted_at = 0
	`).Scan(&count).Error; err != nil {
		t.Fatalf("count migrated rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected preexisting index collision rows to collapse to 1, got %d", count)
	}
	assertIndexExists(t, gormDB, "idx_codex_quotas_tenant_identity", true)
	assertIndexExists(t, gormDB, "idx_codex_quotas_email", false)
	assertIndexMissing(t, gormDB, "idx_codex_quotas_tenant_email")
}

func TestCodexQuotaIdentityMigrationV8DownReturnsIrreversibleError(t *testing.T) {
	gormDB := openRawSQLiteDB(t)
	prepareCodexQuotaMigrationFixture(t, gormDB)

	migration := findMigrationByVersion(t, 8)
	err := migration.Down(gormDB)
	if err == nil {
		t.Fatal("expected irreversible down migration error")
	}
	if !strings.Contains(err.Error(), "idx_codex_quotas_tenant_email") {
		t.Fatalf("expected error to mention idx_codex_quotas_tenant_email, got %q", err)
	}
	if !strings.Contains(err.Error(), "identity/email") {
		t.Fatalf("expected error to mention CodexQuota identity/email situation, got %q", err)
	}
}

func TestSessionCleanupMigrationV11UpReordersCleanupIndex(t *testing.T) {
	gormDB := openRawSQLiteDB(t)
	prepareSessionMigrationFixture(t, gormDB)

	migration := findMigrationByVersion(t, 11)
	if err := migration.Up(gormDB); err != nil {
		t.Fatalf("run migration v11 up: %v", err)
	}

	assertIndexMissing(t, gormDB, "idx_sessions_deleted_updated_at")
	assertSessionIndexColumns(t, gormDB, "idx_sessions_updated_deleted_at", []string{"updated_at", "deleted_at"})
}

func TestSessionCleanupMigrationV11DownRestoresLegacyIndexOrder(t *testing.T) {
	gormDB := openRawSQLiteDB(t)
	prepareSessionMigrationFixture(t, gormDB)

	migration := findMigrationByVersion(t, 11)
	if err := migration.Up(gormDB); err != nil {
		t.Fatalf("run migration v11 up: %v", err)
	}
	if err := migration.Down(gormDB); err != nil {
		t.Fatalf("run migration v11 down: %v", err)
	}

	assertIndexMissing(t, gormDB, "idx_sessions_updated_deleted_at")
	assertSessionIndexColumns(t, gormDB, "idx_sessions_deleted_updated_at", []string{"deleted_at", "updated_at"})
}

func TestCodexQuotaIdentityBackfillSQL_MySQLUsesAccountAwareConcat(t *testing.T) {
	sql := codexQuotaIdentityBackfillSQL("mysql")
	if !strings.Contains(sql, "CONCAT('account:'") {
		t.Fatalf("expected mysql backfill SQL to use CONCAT for account identities, got %q", sql)
	}
	if !strings.Contains(sql, "CONCAT('email:'") {
		t.Fatalf("expected mysql backfill SQL to use CONCAT for email identities, got %q", sql)
	}
	if strings.Contains(sql, "||") {
		t.Fatalf("expected mysql backfill SQL not to use sqlite concatenation, got %q", sql)
	}
}

func TestCodexQuotaIdentityBackfillSQL_SQLiteUsesPipeConcatenation(t *testing.T) {
	sql := codexQuotaIdentityBackfillSQL("sqlite")
	if !strings.Contains(sql, "'account:' || TRIM(account_id)") {
		t.Fatalf("expected sqlite backfill SQL to use pipe concatenation for account identities, got %q", sql)
	}
	if !strings.Contains(sql, "'email:' || TRIM(email)") {
		t.Fatalf("expected sqlite backfill SQL to use pipe concatenation for email identities, got %q", sql)
	}
	if strings.Contains(sql, "CONCAT(") {
		t.Fatalf("expected sqlite backfill SQL not to use mysql CONCAT, got %q", sql)
	}
}

func TestCodexQuotaIdentityDedupeSQL_UsesDialectSpecificDeleteSyntax(t *testing.T) {
	mysqlSQL := codexQuotaIdentityDedupeSQL("mysql")
	if !strings.Contains(mysqlSQL, "DELETE doomed") || !strings.Contains(mysqlSQL, "JOIN codex_quotas AS keeper") {
		t.Fatalf("expected mysql dedupe SQL to use delete-join syntax, got %q", mysqlSQL)
	}

	sqliteSQL := codexQuotaIdentityDedupeSQL("sqlite")
	if !strings.Contains(sqliteSQL, "DELETE FROM codex_quotas") || !strings.Contains(sqliteSQL, "WHERE id IN") {
		t.Fatalf("expected sqlite dedupe SQL to use delete-by-subquery syntax, got %q", sqliteSQL)
	}
}

func openRawSQLiteDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "migrations-test.db")
	gormDB, err := gorm.Open(glebarezsqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open raw sqlite db: %v", err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatalf("get raw sqlite sql.DB: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	return gormDB
}

func prepareCodexQuotaDedupeFixture(t *testing.T, gormDB *gorm.DB) {
	t.Helper()
	if err := gormDB.Exec(`DROP TABLE IF EXISTS codex_quotas`).Error; err != nil {
		t.Fatalf("drop table: %v", err)
	}
	if err := gormDB.Exec(`
		CREATE TABLE codex_quotas (
			id TEXT PRIMARY KEY,
			created_at INTEGER DEFAULT 0,
			updated_at INTEGER DEFAULT 0,
			deleted_at INTEGER DEFAULT 0,
			tenant_id INTEGER NOT NULL,
			identity_key TEXT,
			email TEXT,
			account_id TEXT,
			plan_type TEXT,
			is_forbidden INTEGER DEFAULT 0,
			primary_window TEXT,
			secondary_window TEXT,
			code_review_window TEXT
		)
	`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	inserts := []string{
		`INSERT INTO codex_quotas (id, tenant_id, identity_key, email, account_id, updated_at) VALUES ('row-1', 1, 'account:acct-1', 'first@example.com', 'acct-1', 100)`,
		`INSERT INTO codex_quotas (id, tenant_id, identity_key, email, account_id, updated_at) VALUES ('row-2', 1, 'account:acct-1', 'second@example.com', 'acct-1', 200)`,
		`INSERT INTO codex_quotas (id, tenant_id, identity_key, email, account_id, updated_at) VALUES ('row-3', 1, 'account:acct-2', 'third@example.com', 'acct-2', 150)`,
		`INSERT INTO codex_quotas (id, tenant_id, identity_key, email, account_id, updated_at) VALUES ('row-4', 2, 'account:acct-1', 'other-tenant@example.com', 'acct-1', 120)`,
		`INSERT INTO codex_quotas (id, tenant_id, identity_key, email, account_id, updated_at) VALUES ('row-5', 1, NULL, 'legacy@example.com', '', 90)`,
	}
	for _, sql := range inserts {
		if err := gormDB.Exec(sql).Error; err != nil {
			t.Fatalf("insert fixture: %v", err)
		}
	}
}

func prepareCodexQuotaMigrationFixture(t *testing.T, gormDB *gorm.DB) {
	t.Helper()
	if err := gormDB.Exec(`DROP TABLE IF EXISTS codex_quotas`).Error; err != nil {
		t.Fatalf("drop table: %v", err)
	}
	if err := gormDB.Exec(`
		CREATE TABLE codex_quotas (
			id TEXT PRIMARY KEY,
			created_at INTEGER DEFAULT 0,
			updated_at INTEGER DEFAULT 0,
			deleted_at INTEGER DEFAULT 0,
			tenant_id INTEGER NOT NULL,
			identity_key TEXT,
			email TEXT,
			account_id TEXT,
			plan_type TEXT,
			is_forbidden INTEGER DEFAULT 0,
			primary_window TEXT,
			secondary_window TEXT,
			code_review_window TEXT
		)
	`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	if err := gormDB.Exec(`CREATE UNIQUE INDEX idx_codex_quotas_tenant_email ON codex_quotas(tenant_id, email)`).Error; err != nil {
		t.Fatalf("create old unique index: %v", err)
	}
	inserts := []string{
		`INSERT INTO codex_quotas (id, tenant_id, identity_key, email, account_id, updated_at) VALUES ('row-1', 1, NULL, 'first@example.com', 'acct-1', 100)`,
		`INSERT INTO codex_quotas (id, tenant_id, identity_key, email, account_id, updated_at) VALUES ('row-2', 1, NULL, 'second@example.com', 'acct-1', 200)`,
		`INSERT INTO codex_quotas (id, tenant_id, identity_key, email, account_id, updated_at) VALUES ('row-3', 1, NULL, 'third@example.com', 'acct-2', 150)`,
		`INSERT INTO codex_quotas (id, tenant_id, identity_key, email, account_id, updated_at) VALUES ('row-4', 2, NULL, 'other-tenant@example.com', 'acct-1', 120)`,
		`INSERT INTO codex_quotas (id, tenant_id, identity_key, email, account_id, updated_at) VALUES ('row-5', 1, NULL, 'legacy@example.com', '', 90)`,
	}
	for _, sql := range inserts {
		if err := gormDB.Exec(sql).Error; err != nil {
			t.Fatalf("insert fixture: %v", err)
		}
	}
}

func prepareSessionMigrationFixture(t *testing.T, gormDB *gorm.DB) {
	t.Helper()
	if err := gormDB.Exec(`DROP TABLE IF EXISTS sessions`).Error; err != nil {
		t.Fatalf("drop sessions table: %v", err)
	}
	if err := gormDB.Exec(`
		CREATE TABLE sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at INTEGER DEFAULT 0,
			updated_at INTEGER DEFAULT 0,
			deleted_at INTEGER DEFAULT 0,
			tenant_id INTEGER NOT NULL,
			session_id TEXT NOT NULL,
			client_type TEXT NOT NULL,
			project_id INTEGER DEFAULT 0,
			rejected_at INTEGER DEFAULT 0
		)
	`).Error; err != nil {
		t.Fatalf("create sessions table: %v", err)
	}
	if err := gormDB.Exec(`CREATE INDEX idx_sessions_deleted_updated_at ON sessions(deleted_at, updated_at)`).Error; err != nil {
		t.Fatalf("create legacy session index: %v", err)
	}
}

func prepareCodexQuotaMigrationFixtureWithDeletedHistory(t *testing.T, gormDB *gorm.DB) {
	t.Helper()
	if err := gormDB.Exec(`DROP TABLE IF EXISTS codex_quotas`).Error; err != nil {
		t.Fatalf("drop table: %v", err)
	}
	if err := gormDB.Exec(`
		CREATE TABLE codex_quotas (
			id TEXT PRIMARY KEY,
			created_at INTEGER DEFAULT 0,
			updated_at INTEGER DEFAULT 0,
			deleted_at INTEGER DEFAULT 0,
			tenant_id INTEGER NOT NULL,
			identity_key TEXT,
			email TEXT,
			account_id TEXT,
			plan_type TEXT,
			is_forbidden INTEGER DEFAULT 0,
			primary_window TEXT,
			secondary_window TEXT,
			code_review_window TEXT
		)
	`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	if err := gormDB.Exec(`CREATE UNIQUE INDEX idx_codex_quotas_tenant_email ON codex_quotas(tenant_id, email)`).Error; err != nil {
		t.Fatalf("create old unique index: %v", err)
	}
	inserts := []string{
		`INSERT INTO codex_quotas (id, tenant_id, identity_key, email, account_id, deleted_at, updated_at) VALUES ('row-deleted', 1, NULL, 'old@example.com', 'acct-1', 111, 100)`,
		`INSERT INTO codex_quotas (id, tenant_id, identity_key, email, account_id, deleted_at, updated_at) VALUES ('row-active', 1, NULL, 'current@example.com', 'acct-1', 0, 200)`,
		`INSERT INTO codex_quotas (id, tenant_id, identity_key, email, account_id, deleted_at, updated_at) VALUES ('row-other', 1, NULL, 'other@example.com', 'acct-2', 0, 150)`,
	}
	for _, sql := range inserts {
		if err := gormDB.Exec(sql).Error; err != nil {
			t.Fatalf("insert fixture: %v", err)
		}
	}
}

func prepareCodexQuotaMigrationFixtureWithPreexistingIdentityIndex(t *testing.T, gormDB *gorm.DB) {
	t.Helper()
	if err := gormDB.Exec(`DROP TABLE IF EXISTS codex_quotas`).Error; err != nil {
		t.Fatalf("drop table: %v", err)
	}
	if err := gormDB.Exec(`
		CREATE TABLE codex_quotas (
			id TEXT PRIMARY KEY,
			created_at INTEGER DEFAULT 0,
			updated_at INTEGER DEFAULT 0,
			deleted_at INTEGER DEFAULT 0,
			tenant_id INTEGER NOT NULL,
			identity_key TEXT,
			email TEXT,
			account_id TEXT,
			plan_type TEXT,
			is_forbidden INTEGER DEFAULT 0,
			primary_window TEXT,
			secondary_window TEXT,
			code_review_window TEXT
		)
	`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	if err := gormDB.Exec(`CREATE UNIQUE INDEX idx_codex_quotas_tenant_identity ON codex_quotas(tenant_id, identity_key)`).Error; err != nil {
		t.Fatalf("create legacy identity index: %v", err)
	}
	if err := gormDB.Exec(`CREATE UNIQUE INDEX idx_codex_quotas_email ON codex_quotas(email)`).Error; err != nil {
		t.Fatalf("create broken email index: %v", err)
	}
	if err := gormDB.Exec(`CREATE UNIQUE INDEX idx_codex_quotas_tenant_email ON codex_quotas(tenant_id, email)`).Error; err != nil {
		t.Fatalf("create legacy tenant email index: %v", err)
	}
	inserts := []string{
		`INSERT INTO codex_quotas (id, tenant_id, identity_key, email, account_id, updated_at) VALUES ('row-8', 1, NULL, 'cnc6n2io9xvfev2mtm5t6hu8@example.com', 'e94ce011-80f4-490b-b285-d3109db72b0e', 1773456445476)`,
		`INSERT INTO codex_quotas (id, tenant_id, identity_key, email, account_id, updated_at) VALUES ('row-11', 1, NULL, 'fcew8ua8r6u6zekrwqfi60nl@example.com', 'e94ce011-80f4-490b-b285-d3109db72b0e', 1773456444987)`,
		`INSERT INTO codex_quotas (id, tenant_id, identity_key, email, account_id, updated_at) VALUES ('row-7', 1, NULL, 'puckxqnzu6ktt7k4bcevlw06@example.com', 'e94ce011-80f4-490b-b285-d3109db72b0e', 1773456443639)`,
		`INSERT INTO codex_quotas (id, tenant_id, identity_key, email, account_id, updated_at) VALUES ('row-9', 1, NULL, 'n7e87dj5hxv2c2m4u0e6l5zo@example.com', 'e94ce011-80f4-490b-b285-d3109db72b0e', 1773456443366)`,
	}
	for _, sql := range inserts {
		if err := gormDB.Exec(sql).Error; err != nil {
			t.Fatalf("insert fixture: %v", err)
		}
	}
}

func assertCodexQuotaFixtureCounts(t *testing.T, gormDB *gorm.DB) {
	t.Helper()
	var duplicateCount int64
	if err := gormDB.Raw(`SELECT COUNT(*) FROM codex_quotas WHERE tenant_id = 1 AND identity_key = 'account:acct-1'`).Scan(&duplicateCount).Error; err != nil {
		t.Fatalf("count duplicate rows: %v", err)
	}
	if duplicateCount != 1 {
		t.Fatalf("expected duplicate identity rows to collapse to 1, got %d", duplicateCount)
	}

	var keptEmail string
	if err := gormDB.Raw(`SELECT email FROM codex_quotas WHERE tenant_id = 1 AND identity_key = 'account:acct-1' LIMIT 1`).Scan(&keptEmail).Error; err != nil {
		t.Fatalf("fetch kept email: %v", err)
	}
	if keptEmail != "second@example.com" {
		t.Fatalf("expected newest updated_at row to be kept, got %q", keptEmail)
	}

	var tenant2Count int64
	if err := gormDB.Raw(`SELECT COUNT(*) FROM codex_quotas WHERE tenant_id = 2 AND identity_key = 'account:acct-1'`).Scan(&tenant2Count).Error; err != nil {
		t.Fatalf("count tenant 2 rows: %v", err)
	}
	if tenant2Count != 1 {
		t.Fatalf("expected tenant 2 row to be preserved, got %d", tenant2Count)
	}

	var nullIdentityCount int64
	if err := gormDB.Raw(`SELECT COUNT(*) FROM codex_quotas WHERE tenant_id = 1 AND identity_key IS NULL`).Scan(&nullIdentityCount).Error; err != nil {
		t.Fatalf("count null identity rows: %v", err)
	}
	if nullIdentityCount != 1 {
		t.Fatalf("expected null identity rows to be preserved, got %d", nullIdentityCount)
	}
}

func assertCodexQuotaPostMigrationState(t *testing.T, gormDB *gorm.DB) {
	t.Helper()
	var duplicateCount int64
	if err := gormDB.Raw(`SELECT COUNT(*) FROM codex_quotas WHERE tenant_id = 1 AND identity_key = 'account:acct-1'`).Scan(&duplicateCount).Error; err != nil {
		t.Fatalf("count duplicate rows: %v", err)
	}
	if duplicateCount != 1 {
		t.Fatalf("expected migrated duplicate identity rows to collapse to 1, got %d", duplicateCount)
	}

	var keptEmail string
	if err := gormDB.Raw(`SELECT email FROM codex_quotas WHERE tenant_id = 1 AND identity_key = 'account:acct-1' LIMIT 1`).Scan(&keptEmail).Error; err != nil {
		t.Fatalf("fetch kept email: %v", err)
	}
	if keptEmail != "second@example.com" {
		t.Fatalf("expected newest migrated row to be kept, got %q", keptEmail)
	}

	var tenant2Count int64
	if err := gormDB.Raw(`SELECT COUNT(*) FROM codex_quotas WHERE tenant_id = 2 AND identity_key = 'account:acct-1'`).Scan(&tenant2Count).Error; err != nil {
		t.Fatalf("count tenant 2 rows: %v", err)
	}
	if tenant2Count != 1 {
		t.Fatalf("expected tenant 2 migrated row to be preserved, got %d", tenant2Count)
	}

	var legacyEmailCount int64
	if err := gormDB.Raw(`SELECT COUNT(*) FROM codex_quotas WHERE tenant_id = 1 AND identity_key = 'email:legacy@example.com'`).Scan(&legacyEmailCount).Error; err != nil {
		t.Fatalf("count legacy email rows: %v", err)
	}
	if legacyEmailCount != 1 {
		t.Fatalf("expected legacy null identity row to backfill to email identity, got %d", legacyEmailCount)
	}
}

func findMigrationByVersion(t *testing.T, version int) Migration {
	t.Helper()
	for _, migration := range migrations {
		if migration.Version == version {
			return migration
		}
	}
	t.Fatalf("migration v%d not found", version)
	return Migration{}
}

func assertIndexExists(t *testing.T, gormDB *gorm.DB, name string, wantUnique bool) {
	t.Helper()
	var rows []struct {
		Name   string `gorm:"column:name"`
		Unique int    `gorm:"column:unique"`
	}
	if err := gormDB.Raw(`PRAGMA index_list('codex_quotas')`).Scan(&rows).Error; err != nil {
		t.Fatalf("list indexes: %v", err)
	}
	for _, row := range rows {
		if row.Name == name {
			if (row.Unique == 1) != wantUnique {
				t.Fatalf("index %s unique=%v, want %v", name, row.Unique == 1, wantUnique)
			}
			return
		}
	}
	t.Fatalf("expected index %s to exist; got %v", name, rows)
}

func assertIndexMissing(t *testing.T, gormDB *gorm.DB, name string) {
	t.Helper()
	var count int64
	query := fmt.Sprintf("SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='%s'", name)
	if err := gormDB.Raw(query).Scan(&count).Error; err != nil {
		t.Fatalf("check index missing: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected index %s to be missing, got count %d", name, count)
	}
}

func assertSessionIndexColumns(t *testing.T, gormDB *gorm.DB, name string, wantColumns []string) {
	t.Helper()

	type indexListRow struct {
		Name string `gorm:"column:name"`
	}
	var indexes []indexListRow
	if err := gormDB.Raw(`PRAGMA index_list('sessions')`).Scan(&indexes).Error; err != nil {
		t.Fatalf("list session indexes: %v", err)
	}

	found := false
	for _, index := range indexes {
		if index.Name == name {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected session index %s to exist; got %v", name, indexes)
	}

	type indexInfoRow struct {
		Name string `gorm:"column:name"`
	}
	var columns []indexInfoRow
	if err := gormDB.Raw(fmt.Sprintf("PRAGMA index_info('%s')", name)).Scan(&columns).Error; err != nil {
		t.Fatalf("get session index columns: %v", err)
	}

	gotColumns := make([]string, 0, len(columns))
	for _, column := range columns {
		gotColumns = append(gotColumns, column.Name)
	}
	if !slices.Equal(gotColumns, wantColumns) {
		t.Fatalf("session index %s columns = %v, want %v", name, gotColumns, wantColumns)
	}
}
