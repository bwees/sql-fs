package database

import "github.com/uptrace/bun"

type File struct {
	bun.BaseModel `bun:"table:file,alias:f"`

	ID       uint64 `bun:",pk,autoincrement"`
	ParentID uint64 `bun:",notnull"`

	Filename string `bun:",notnull"`
	Filepath string `bun:",unique"`

	// Mode holds the full mode incl. file-type bits (e.g. S_IFDIR|0755 or S_IFREG|0644).
	Mode uint32 `bun:",notnull"`

	Size int64 `bun:",notnull"`
}
