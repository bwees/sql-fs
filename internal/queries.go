package internal

import (
	"fmt"
	"fuse-demo/internal/database"

	"github.com/hanwen/go-fuse/v2/fuse"
)

func (s *SqlFS) ResolveFile(id string) (*database.File, fuse.Status) {
	var file []database.File
	err := s.db.NewSelect().
		Model(&file).
		Where("id = ?", id).
		Scan(s.ctx)

	if err != nil {
		return nil, fuse.EIO
	}

	if len(file) == 0 {
		return nil, fuse.ENOENT
	}

	return &file[0], fuse.OK
}

func (s *SqlFS) GetChild(parentID uint64, name string) (*database.File, fuse.Status) {
	var file []database.File
	err := s.db.NewSelect().
		Model(&file).
		Where("parent_id = ? AND filename = ?", parentID, name).
		Scan(s.ctx)

	if err != nil {
		return nil, fuse.EIO
	}

	if len(file) == 0 {
		return nil, fuse.ENOENT
	}

	return &file[0], fuse.OK
}

func (s *SqlFS) ListChildren(parentID uint64) ([]database.File, fuse.Status) {
	var files []database.File
	err := s.db.NewSelect().
		Model(&files).
		Where("parent_id = ?", parentID).
		Scan(s.ctx)

	if err != nil {
		return nil, fuse.EIO
	}

	return files, fuse.OK
}

func (s *SqlFS) RenameFile(id uint64, name string, filepath string, parentID uint64) fuse.Status {
	_, err := s.db.NewUpdate().
		Model((*database.File)(nil)).
		Set("filename = ?", name).
		Set("filepath = ?", filepath).
		Set("parent_id = ?", parentID).
		Where("id = ?", id).
		Exec(s.ctx)

	if err != nil {
		fmt.Printf("Failed to rename file in database: %v", err)
		return fuse.EIO
	}

	return fuse.OK
}

func (s *SqlFS) DeleteFile(id uint64) fuse.Status {
	_, err := s.db.NewDelete().
		Model((*database.File)(nil)).
		Where("id = ?", id).
		Exec(s.ctx)

	if err != nil {
		fmt.Printf("Failed to delete file from database: %v", err)
		return fuse.EIO
	}

	return fuse.OK
}
