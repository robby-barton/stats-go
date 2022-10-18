package database

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

type DB struct {
	db *sql.DB
}

type DBParams struct {
	Host string
	Port int64
	User string
	Password string
	DBName string
	SSLMode string
}

func NewDatabase(params *DBParams) (*DB, error) {
	connInfo := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		params.Host,
		params.Port,
		params.User,
		params.Password,
		params.DBName,
		params.SSLMode,
	)
	db, err := sql.Open("postgres", connInfo)
	if err != nil {
		return nil, err
	}

	err = db.Ping()
	if err != nil {
		return nil, err
	}

	return &DB{
		db: db,
	}, nil
}

func (db *DB) Close() {
	db.db.Close()
}

func (db *DB) Query(queryString string, args ...any) ([]map[string]interface{}, error) {
	rows, err := db.db.Query(queryString, args...)
	if err != nil {
		return nil, err
	}

	var columns []string
	columns, err = rows.Columns()
	if err != nil {
		return nil, err
	}

	colNum := len(columns)
	var results []map[string]interface{}

	for rows.Next() {
		r := make([]interface{}, colNum)
		for i := range r {
			r[i] = &r[i]
		}

		err = rows.Scan(r...)
		if err != nil {
			return nil, err
		}

		var row = map[string]interface{}{}
		for i := range r {
			row[columns[i]] = r[i]
		}

		results = append(results, row)
	}

	return results, nil
}
