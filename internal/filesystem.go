package internal

import (
	"context"
	"fmt"
	"fuse-demo/internal/database"
	"os"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/uptrace/bun"
)

type SqlFS struct {
	fuse.RawFileSystem

	db  *bun.DB
	ctx context.Context
}

func attrFor(f database.File) fuse.Attr {
	nlink := uint32(1)
	if f.Mode&syscall.S_IFDIR != 0 {
		nlink = 2
	}
	return fuse.Attr{
		Ino:   f.ID,
		Mode:  f.Mode,
		Size:  uint64(f.Size),
		Nlink: nlink,
		Owner: fuse.Owner{
			Uid: uint32(os.Getuid()),
			Gid: uint32(os.Getgid()),
		},
	}
}

func (s *SqlFS) resolveFilepath(parentID uint64, name string) (string, error) {
	var filepath string
	if parentID == 1 { // root
		filepath = fmt.Sprintf("/%s", name)
	} else {
		parentFile, err := s.ResolveFile(fmt.Sprintf("%d", parentID))

		if err != fuse.OK {
			return "", fmt.Errorf("failed to resolve parent file: %v", err)
		}

		filepath = fmt.Sprintf("%s/%s", parentFile.Filepath, name)
	}

	return filepath, nil
}

func (s *SqlFS) resolveStorageFolder(id uint64) string {
	hex1 := fmt.Sprintf("%02x", (id/256)%256)
	hex2 := fmt.Sprintf("%02x", id%256)

	return fmt.Sprintf("storage/%s/%s", hex1, hex2)
}

func (s *SqlFS) Init(server *fuse.Server) {
	db, err := database.CreateDB("filesystem.db")
	if err != nil {
		fmt.Printf("Failed to create database: %v", err)
		panic(err)
	}

	s.db = db
	s.ctx = context.Background()
}

func (s *SqlFS) StatFs(cancel <-chan struct{}, header *fuse.InHeader, out *fuse.StatfsOut) fuse.Status {
	const blockSize = 4096
	const totalBlocks = 1 << 30 // ~4 TiB of reported capacity

	out.Bsize = blockSize
	out.Frsize = blockSize
	out.Blocks = totalBlocks
	out.Bfree = totalBlocks
	out.Bavail = totalBlocks
	out.Files = 1 << 20
	out.Ffree = 1 << 20
	out.NameLen = 255
	return fuse.OK
}

func (s *SqlFS) Lookup(cancel <-chan struct{}, header *fuse.InHeader, name string, out *fuse.EntryOut) (status fuse.Status) {

	file, err := s.GetChild(header.NodeId, name)
	if err != fuse.OK {
		return err
	}

	*out = fuse.EntryOut{
		NodeId: file.ID,
		Attr:   attrFor(*file),
	}

	return fuse.OK
}

func (s *SqlFS) GetAttr(cancel <-chan struct{}, input *fuse.GetAttrIn, out *fuse.AttrOut) (code fuse.Status) {
	file, err := s.ResolveFile(fmt.Sprintf("%d", input.NodeId))
	if err != fuse.OK {
		return err
	}

	out.Attr = attrFor(*file)

	return fuse.OK
}

func (s *SqlFS) ReadDirPlus(cancel <-chan struct{}, input *fuse.ReadIn, out *fuse.DirEntryList) fuse.Status {
	return s.readDir(input, out, true)
}

func (s *SqlFS) ReadDir(cancel <-chan struct{}, input *fuse.ReadIn, out *fuse.DirEntryList) fuse.Status {
	return s.readDir(input, out, false)
}

func (s *SqlFS) readDir(input *fuse.ReadIn, out *fuse.DirEntryList, plus bool) fuse.Status {
	files, err := s.ListChildren(input.NodeId)

	if err != fuse.OK {
		fmt.Printf("Failed to query database: %v", err)
		return fuse.EIO
	}

	if input.Offset >= uint64(len(files)) {
		return fuse.OK
	}
	files = files[input.Offset:]

	for _, file := range files {
		if plus {
			entryOut := out.AddDirLookupEntry(fuse.DirEntry{
				Name: file.Filename,
				Mode: file.Mode,
				Ino:  file.ID,
			})
			if entryOut == nil {
				break // buffer full; kernel will ask again from the last offset
			}

			entryOut.NodeId = file.ID
			entryOut.Attr = attrFor(file)
		} else {
			if !out.AddDirEntry(fuse.DirEntry{
				Name: file.Filename,
				Mode: file.Mode,
				Ino:  file.ID,
			}) {
				break // buffer full; kernel will ask again from the last offset
			}
		}
	}

	return fuse.OK
}

func (s *SqlFS) Mkdir(cancel <-chan struct{}, input *fuse.MkdirIn, name string, out *fuse.EntryOut) (code fuse.Status) {

	filepath, err := s.resolveFilepath(input.NodeId, name)
	if err != nil {
		fmt.Printf("Failed to resolve filepath: %v", err)
		return fuse.EIO
	}

	newFile := &database.File{
		Filename: name,
		ParentID: input.NodeId,
		// MkdirIn.Mode carries only permission bits; the kernel masks off the
		// type bits and rejects the reply (EIO) unless we report S_IFDIR back.
		Mode:     input.Mode | syscall.S_IFDIR,
		Size:     0,
		Filepath: filepath,
	}

	_, dbErr := s.db.NewInsert().
		Model(newFile).
		Returning("id").
		Exec(s.ctx)

	if dbErr != nil {
		fmt.Printf("Failed to insert into database: %v", dbErr)
		return fuse.EIO
	}

	out.NodeId = newFile.ID
	out.Attr = attrFor(*newFile)

	return fuse.OK
}

func (s *SqlFS) Create(cancel <-chan struct{}, input *fuse.CreateIn, name string, out *fuse.CreateOut) (code fuse.Status) {
	filepath, err := s.resolveFilepath(input.NodeId, name)
	if err != nil {
		fmt.Printf("Failed to resolve filepath: %v", err)
		return fuse.EIO
	}

	newFile := &database.File{
		Filename: name,
		ParentID: input.NodeId,
		Mode:     input.Mode,
		Size:     0,
		Filepath: filepath,
	}

	dbErr := s.CreateFile(newFile)
	if dbErr != fuse.OK {
		fmt.Printf("Failed to insert into database: %v", dbErr)
		return fuse.EIO
	}

	storageFolder := s.resolveStorageFolder(newFile.ID)
	if err := os.MkdirAll(storageFolder, 0755); err != nil {
		fmt.Printf("Failed to create directories for file storage: %v", err)
		return fuse.EIO
	}

	f, err := os.Create(fmt.Sprintf("%s/%016x", storageFolder, newFile.ID))
	if err != nil {
		fmt.Printf("Failed to create file: %v", dbErr)
		return fuse.EIO
	}
	defer f.Close()

	out.NodeId = newFile.ID
	out.Attr = fuse.Attr{
		Mode:  newFile.Mode,
		Size:  uint64(newFile.Size),
		Mtime: 0, // Set appropriate modification time
		Ctime: 0, // Set appropriate change time
		Atime: 0, // Set appropriate access time
	}

	return fuse.OK
}

func (s *SqlFS) Rmdir(cancel <-chan struct{}, header *fuse.InHeader, name string) (code fuse.Status) {
	file, err := s.GetChild(header.NodeId, name)
	if err != fuse.OK {
		return err
	}

	if file.Mode&syscall.S_IFDIR == 0 {
		return fuse.ENOTDIR
	}

	// Check if directory is empty
	children, listErr := s.ListChildren(file.ID)
	if listErr != fuse.OK {
		fmt.Printf("Failed to list children: %v", listErr)
		return fuse.EIO
	}

	if len(children) > 0 {
		return fuse.Status(syscall.ENOTEMPTY)
	}

	dbErr := s.DeleteFile(file.ID)

	if dbErr != fuse.OK {
		fmt.Printf("Failed to delete from database: %v", dbErr)
		return fuse.EIO
	}

	return fuse.OK
}

func (s *SqlFS) Unlink(cancel <-chan struct{}, header *fuse.InHeader, name string) (code fuse.Status) {
	file, err := s.GetChild(header.NodeId, name)
	if err != fuse.OK {
		return err
	}

	if file.Mode&syscall.S_IFDIR != 0 {
		return fuse.EISDIR
	}

	storageFolder := s.resolveStorageFolder(file.ID)
	filePath := fmt.Sprintf("%s/%016x", storageFolder, file.ID)
	if err := os.Remove(filePath); err != nil {
		fmt.Printf("Failed to remove file from storage: %v", err)
		return fuse.EIO
	}

	dbErr := s.DeleteFile(file.ID)
	if dbErr != fuse.OK {
		fmt.Printf("Failed to delete from database: %v", dbErr)
		return fuse.EIO
	}

	return fuse.OK
}

func (s *SqlFS) Rename(cancel <-chan struct{}, input *fuse.RenameIn, oldName string, newName string) (code fuse.Status) {

	fmt.Println("Rename called with oldName:", oldName, "newName:", newName)
	fmt.Println("src/dest inodes", input.NodeId, input.Newdir)

	// Desitnation should not already exist
	_, err := s.GetChild(input.Newdir, newName)
	if err == fuse.OK {
		fmt.Printf("Destination file already exists: %s in directory %d", newName, input.Newdir)
		return fuse.Status(syscall.EEXIST)
	}

	// Get the file to be renamed
	file, err := s.GetChild(input.NodeId, oldName)
	if err != fuse.OK {
		fmt.Printf("Failed to get file for renaming: %v", err)
		return err
	}

	// build the new full filepath
	newFilepath, pathErr := s.resolveFilepath(input.Newdir, newName)
	if pathErr != nil {
		fmt.Printf("Failed to resolve new filepath: %v", pathErr)
		return fuse.EIO
	}

	return s.RenameFile(file.ID, newName, newFilepath, input.Newdir)
}

// TODO
// Open
// Read
// Write
// Release

func NewSqlFS() *SqlFS {
	return &SqlFS{RawFileSystem: fuse.NewDefaultRawFileSystem()}
}
