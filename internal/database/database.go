package database

import (
	"context"
	"database/sql"
	"fmt"
	"syscall"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func CreateDB(filename string) (*bun.DB, error) {
	ctx := context.Background()
	sqldb, err := sql.Open(sqliteshim.ShimName, "file:"+filename+"?cache=shared")
	if err != nil {
		return nil, err
	}

	db := bun.NewDB(sqldb, sqlitedialect.New())

	_, err = db.NewCreateTable().
		Model((*File)(nil)).
		IfNotExists().
		Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("create table: %w", err)
	}

	rootFile := &File{
		ID:       1,
		Filename: "root",
		ParentID: 0,
		Mode:     syscall.S_IFDIR | 0755,
	}

	_, err = db.NewInsert().
		Model(rootFile).
		On("CONFLICT (id) DO NOTHING").
		Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("insert root: %w", err)
	}

	fmt.Println("Database initialized successfully")

	return db, nil
}
