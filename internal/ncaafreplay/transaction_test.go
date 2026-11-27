package ncaafreplay

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlog "gorm.io/gorm/logger"

	"github.com/robby-barton/stats-go/internal/database"
)

// A hermetic SQL driver verifies the PostgreSQL transaction protocol without
// a network, service, credentials, live database or external mocking module.
type proofConnector struct{ conn *proofConn }

func (c proofConnector) Connect(context.Context) (driver.Conn, error) { return c.conn, nil }
func (c proofConnector) Driver() driver.Driver                        { return proofDriver(c) }

type proofDriver struct{ conn *proofConn }

func (d proofDriver) Open(string) (driver.Conn, error) { return d.conn, nil }

type proofConn struct {
	mode, isolation          string
	begin                    driver.TxOptions
	rollback, commit, closed bool
	timeout                  bool
	rollbackErr              bool
	beginErr                 bool
}

func (c *proofConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (c *proofConn) Close() error              { c.closed = true; return nil }
func (c *proofConn) Begin() (driver.Tx, error) { return nil, errors.New("missing explicit tx options") }
func (c *proofConn) BeginTx(_ context.Context, opts driver.TxOptions) (driver.Tx, error) {
	c.begin = opts
	if c.beginErr {
		return nil, errors.New("begin failed")
	}
	return proofTx{c}, nil
}
func (c *proofConn) QueryContext(_ context.Context, q string, _ []driver.NamedValue) (driver.Rows, error) {
	switch q {
	case "SHOW transaction_read_only":
		return &proofRows{value: c.mode}, nil
	case "SHOW transaction_isolation":
		return &proofRows{value: c.isolation}, nil
	default:
		return nil, errors.New("unexpected query")
	}
}
func (c *proofConn) ExecContext(_ context.Context, q string, _ []driver.NamedValue) (driver.Result, error) {
	if q != "SET LOCAL statement_timeout = '10s'" {
		return nil, errors.New("unexpected exec")
	}
	c.timeout = true
	return driver.RowsAffected(0), nil
}

type proofTx struct{ conn *proofConn }

func (t proofTx) Commit() error { t.conn.commit = true; return errors.New("commits forbidden") }
func (t proofTx) Rollback() error {
	t.conn.rollback = true
	if t.conn.rollbackErr {
		return errors.New("rollback failure")
	}
	return nil
}

type proofRows struct {
	value string
	done  bool
}

func (r *proofRows) Columns() []string { return []string{"value"} }
func (r *proofRows) Close() error      { return nil }
func (r *proofRows) Next(values []driver.Value) error {
	if r.done {
		return io.EOF
	}
	values[0] = r.value
	r.done = true
	return nil
}

func TestReadOnlyTransactionProtocol(t *testing.T) {
	for _, tc := range []struct {
		name, mode, isolation            string
		visitErr, rollbackErr, wantVisit bool
	}{
		{"verified", "on", "repeatable read", false, false, true},
		{"writable", "off", "repeatable read", false, false, false},
		{"isolation", "on", "read committed", false, false, false},
		{"visit error", "on", "repeatable read", true, false, true},
		{"rollback error", "on", "repeatable read", false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn := &proofConn{mode: tc.mode, isolation: tc.isolation, rollbackErr: tc.rollbackErr}
			sqlDB := sql.OpenDB(proofConnector{conn})
			defer sqlDB.Close()
			db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}),
				&gorm.Config{DisableAutomaticPing: true,
					SkipDefaultTransaction: true,
					Logger:                 gormlog.Default.LogMode(gormlog.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			visited := false
			proof := TransactionEvidence{}
			err = VerifiedTransaction(context.Background(), db, &proof, func(*gorm.DB) error {
				visited = true
				if tc.visitErr {
					return errors.New("visit failed")
				}
				return nil
			})
			wantErr := !tc.wantVisit || tc.visitErr || tc.rollbackErr
			if (err != nil) != wantErr ||
				visited != tc.wantVisit ||
				!conn.rollback ||
				conn.commit ||
				!conn.begin.ReadOnly ||
				conn.begin.Isolation != driver.IsolationLevel(sql.LevelRepeatableRead) {
				t.Fatalf("err=%v visited=%v conn=%+v", err, visited, conn)
			}
			if proof.Rollback == tc.rollbackErr {
				t.Fatalf("wrong rollback proof: %+v", proof)
			}
			if visited && !conn.timeout {
				t.Fatal("application data read before timeout configured")
			}
			if visited && (!proof.ReadOnly || proof.Isolation != "repeatable read") {
				t.Fatalf("incomplete proof after visit: %+v", proof)
			}
		})
	}
}

func TestReadOnlyTransactionRejectsNonPostgresAndBadConfig(t *testing.T) {
	if err := VerifiedTransaction(context.Background(), nil, nil,
		func(*gorm.DB) error {
			t.Fatal("nil visitor executed")
			return nil
		}); err == nil {
		t.Fatal("nil DB allowed")
	}
	for _, tc := range []struct {
		name   string
		params *database.DBParams
	}{
		{name: "nil", params: nil},
		{name: "missing host", params: &database.DBParams{User: "x", DBName: "x", Port: 5432}},
		{name: "missing user", params: &database.DBParams{Host: "h", DBName: "x", Port: 5432}},
		{name: "missing dbname", params: &database.DBParams{Host: "h", User: "x", Port: 5432}},
		{name: "port zero", params: &database.DBParams{Host: "h", User: "x", DBName: "x"}},
		{name: "port too large", params: &database.DBParams{Host: "h", User: "x", DBName: "x", Port: 65536}},
	} {
		if err := WithReadOnlyProof(context.Background(), tc.params, nil,
			func(*gorm.DB) error {
				t.Fatalf("%s: visitor executed", tc.name)
				return nil
			}); err == nil {
			t.Fatalf("%s: invalid config accepted", tc.name)
		}
	}
}
