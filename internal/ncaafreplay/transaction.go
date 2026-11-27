// Package ncaafreplay drives a narrow, read-only NCAAF parity check: it
// compares the production ranking baseline (the unblended, default path) with
// an independently loaded explicit-cutoff replay over explicitly listed
// windows, inside a verified read-only repeatable-read PostgreSQL transaction
// that never commits.
package ncaafreplay

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"net/url"
	"strconv"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlog "gorm.io/gorm/logger"

	"github.com/robby-barton/stats-go/internal/database"
)

const statementTimeout = "10s"

// TransactionEvidence records the verified read-only transaction guarantees.
// A report is only trustworthy when every field is set.
type TransactionEvidence struct {
	ReadOnly  bool   `json:"verified_read_only"`
	Isolation string `json:"verified_isolation"`
	Rollback  bool   `json:"rollback_completed"`
}

// WithReadOnlyProof opens a fresh PostgreSQL connection from explicit params,
// runs visit inside a verified read-only repeatable-read transaction that is
// always rolled back, and closes the connection. The error never carries
// connection details.
func WithReadOnlyProof(ctx context.Context, params *database.DBParams, proof *TransactionEvidence,
	visit func(*gorm.DB) error,
) (err error) {
	if params == nil || params.Host == "" || params.User == "" || params.DBName == "" ||
		params.Port < 1 || params.Port > 65535 {
		return errors.New("replay requires explicit PostgreSQL configuration; SQLite fallback is disabled")
	}
	dsn := &url.URL{Scheme: "postgres", Host: net.JoinHostPort(params.Host, strconv.FormatInt(params.Port, 10)),
		User: url.UserPassword(params.User, params.Password), Path: "/" + params.DBName}
	query := dsn.Query()
	if params.SSLMode != "" {
		query.Set("sslmode", params.SSLMode)
	}
	query.Set("connect_timeout", "5")
	dsn.RawQuery = query.Encode()
	db, err := gorm.Open(postgres.Open(dsn.String()), &gorm.Config{
		SkipDefaultTransaction: true, DisableAutomaticPing: true, Logger: gormlog.Default.LogMode(gormlog.Silent),
	})
	if err != nil {
		return errors.New("PostgreSQL connection failed (connection details suppressed)")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return errors.New("PostgreSQL connection unavailable")
	}
	defer func() {
		if closeErr := sqlDB.Close(); closeErr != nil {
			err = errors.Join(err, errors.New("PostgreSQL connection closure failed"))
		}
	}()
	return VerifiedTransaction(ctx, db, proof, visit)
}

// VerifiedTransaction begins a read-only repeatable-read transaction, proves
// the server-side mode before any application read, sets a statement timeout,
// and rolls back in every path. There is no commit path.
func VerifiedTransaction(ctx context.Context, db *gorm.DB, proof *TransactionEvidence,
	visit func(*gorm.DB) error,
) (err error) {
	if db == nil || db.Name() != "postgres" {
		return errors.New("read-only replay requires PostgreSQL; SQLite fallback disabled")
	}
	tx := db.WithContext(ctx).Begin(&sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if tx.Error != nil {
		return errors.New("cannot start PostgreSQL read-only transaction")
	}
	defer func() {
		if rollbackErr := tx.Rollback().Error; rollbackErr != nil {
			err = errors.Join(err, errors.New("read-only transaction rollback failed"))
		} else if proof != nil {
			proof.Rollback = true
		}
	}()
	var mode string
	if err := tx.Raw("SHOW transaction_read_only").Scan(&mode).Error; err != nil || mode != "on" {
		return errors.New("PostgreSQL read-only verification failed; no application data read")
	}
	if proof != nil {
		proof.ReadOnly = true
	}
	var isolation string
	if err := tx.Raw("SHOW transaction_isolation").Scan(&isolation).Error; err != nil || isolation != "repeatable read" {
		return errors.New("PostgreSQL repeatable-read verification failed; no application data read")
	}
	if proof != nil {
		proof.Isolation = isolation
	}
	if err := tx.Exec("SET LOCAL statement_timeout = '" + statementTimeout + "'").Error; err != nil {
		return errors.New("cannot set read-only replay statement timeout")
	}
	return visit(tx)
}
