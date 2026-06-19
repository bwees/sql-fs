# sqlfs

PoC of a filesystem backed by SQL as an index for a large collection of files. The goal is to be able to quickly query and access files based on their metadata, without having to rely on the underlying filesystem's directory structure.

The implementation uses FUSE to create a virtual filesystem that interacts with a SQL database. The database stores metadata about the files, such as their names, paths, sizes, and modification times. The filesystem allows users to navigate through the virtual directory structure and access files based on their metadata.

TODO:
- Implement file reading and writing
- Add support for file permissions and ownership
- Tests
- Mount by inode instead of path