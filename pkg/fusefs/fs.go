package fusefs

import (
	"context"
	"errors"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"

	"github.com/shaowenchen/memoryfs/pkg/meta"
	"github.com/shaowenchen/memoryfs/pkg/mountlog"
	"github.com/shaowenchen/memoryfs/pkg/storage"
)

// MemoryFS is the root FUSE node.
type MemoryFS struct {
	fs.Inode
	store     meta.Backend
	chunks    *storage.ChunkStore
	uid       uint32
	gid       uint32
	sizeBytes uint64
	capacityBytes func(context.Context) uint64
	usedBytes     func(context.Context) uint64
}

// NewRoot creates the root filesystem node.
func NewRoot(store meta.Backend, chunks *storage.ChunkStore, uid, gid uint32, capacityBytes, usedBytes func(context.Context) uint64) *MemoryFS {
	return &MemoryFS{
		store:         store,
		chunks:        chunks,
		uid:           uid,
		gid:           gid,
		capacityBytes: capacityBytes,
		usedBytes:     usedBytes,
	}
}

var (
	_ fs.NodeLookuper     = (*MemoryFS)(nil)
	_ fs.NodeGetattrer    = (*MemoryFS)(nil)
	_ fs.NodeReaddirer    = (*MemoryFS)(nil)
	_ fs.NodeMkdirer      = (*MemoryFS)(nil)
	_ fs.NodeCreater      = (*MemoryFS)(nil)
	_ fs.NodeUnlinker     = (*MemoryFS)(nil)
	_ fs.NodeRmdirer      = (*MemoryFS)(nil)
	_ fs.NodeRenamer      = (*MemoryFS)(nil)
	_ fs.NodeSymlinker    = (*MemoryFS)(nil)
	_ fs.NodeStatfser     = (*MemoryFS)(nil)
)

func (m *MemoryFS) rootIno() uint64 { return meta.RootIno() }

func (m *MemoryFS) Statfs(ctx context.Context, out *fuse.StatfsOut) syscall.Errno {
	const blockSize = 4096
	total := m.sizeBytes
	if m.capacityBytes != nil {
		if t := m.capacityBytes(ctx); t > 0 {
			total = t
		}
	}
	used := uint64(0)
	if m.usedBytes != nil {
		used = m.usedBytes(ctx)
	}
	if total < used {
		total = used
	}
	free := total - used
	out.Bsize = blockSize
	out.Frsize = blockSize
	out.Blocks = total / blockSize
	out.Bfree = free / blockSize
	out.Bavail = free / blockSize
	out.NameLen = 255
	return 0
}

func (m *MemoryFS) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	attr, err := m.store.GetAttr(ctx, m.rootIno())
	if err != nil {
		return syscall.ENOENT
	}
	fillAttr(out, attr)
	return 0
}

func (m *MemoryFS) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	if name == "." || name == ".." {
		fillEntryOut(out, m.store, ctx, m.rootIno())
		return m.EmbeddedInode(), 0
	}
	attr, err := m.store.Lookup(ctx, m.rootIno(), name)
	if err != nil {
		return nil, syscall.ENOENT
	}
	return m.newChild(ctx, attr, out)
}

func (m *MemoryFS) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	entries, err := m.store.Readdir(ctx, m.rootIno())
	if err != nil {
		return nil, syscall.EIO
	}
	var items []fuse.DirEntry
	items = append(items, fuse.DirEntry{Name: ".", Ino: m.rootIno(), Mode: fuse.S_IFDIR})
	items = append(items, fuse.DirEntry{Name: "..", Ino: m.rootIno(), Mode: fuse.S_IFDIR})
	for name, attr := range entries {
		items = append(items, fuse.DirEntry{Name: name, Ino: attr.Ino, Mode: attr.Mode & 0o7777})
	}
	return fs.NewListDirStream(items), 0
}

func (m *MemoryFS) Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	attr, err := m.store.Mkdir(ctx, m.rootIno(), name, mode, m.uid, m.gid)
	if err != nil {
		return nil, mapMetaErr(err)
	}
	return m.newChild(ctx, attr, out)
}

func (m *MemoryFS) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (node *fs.Inode, fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	mountlog.Infof("MemoryFS Create start: name=%s flags=0x%x mode=0%o", name, flags, mode)
	attr, err := m.store.Create(ctx, m.rootIno(), name, mode, m.uid, m.gid)
	if err != nil {
		mountlog.Errorf("MemoryFS Create failed: name=%s err=%v", name, err)
		return nil, nil, 0, mapMetaErr(err)
	}
	mountlog.Infof("MemoryFS Create success: name=%s newIno=%d", name, attr.Ino)
	inode, errno := m.newChild(ctx, attr, out)
	return inode, nil, fuse.FOPEN_KEEP_CACHE, errno
}

func (m *MemoryFS) Symlink(ctx context.Context, target, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	attr, err := m.store.Symlink(ctx, m.rootIno(), name, target, m.uid, m.gid)
	if err != nil {
		return nil, mapMetaErr(err)
	}
	return m.newChild(ctx, attr, out)
}

func (m *MemoryFS) Unlink(ctx context.Context, name string) syscall.Errno {
	attr, err := m.store.Unlink(ctx, m.rootIno(), name)
	if err != nil {
		return syscall.ENOENT
	}
	m.chunks.DeleteChunks(ctx, attr)
	return 0
}

func (m *MemoryFS) Rmdir(ctx context.Context, name string) syscall.Errno {
	if err := m.store.Rmdir(ctx, m.rootIno(), name); err != nil {
		return mapMetaErr(err)
	}
	return 0
}

func (m *MemoryFS) Rename(ctx context.Context, name string, newParent fs.InodeEmbedder, newName string, flags uint32) syscall.Errno {
	np, ok := newParent.(*MemoryFS)
	if !ok {
		if dir, ok := newParent.(*Dir); ok {
			if err := m.store.Rename(ctx, m.rootIno(), dir.ino, name, newName); err != nil {
				return mapMetaErr(err)
			}
			return 0
		}
		return syscall.EXDEV
	}
	if err := m.store.Rename(ctx, m.rootIno(), np.rootIno(), name, newName); err != nil {
		return mapMetaErr(err)
	}
	return 0
}

func (m *MemoryFS) newChild(ctx context.Context, attr *meta.Attr, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	return newChildNode(m.NewInode, ctx, m.store, m.chunks, m.uid, m.gid, attr, out)
}

// Dir represents a subdirectory.
type Dir struct {
	fs.Inode
	store  meta.Backend
	chunks *storage.ChunkStore
	ino    uint64
	uid    uint32
	gid    uint32
}

var (
	_ fs.NodeLookuper  = (*Dir)(nil)
	_ fs.NodeGetattrer = (*Dir)(nil)
	_ fs.NodeReaddirer = (*Dir)(nil)
	_ fs.NodeMkdirer   = (*Dir)(nil)
	_ fs.NodeCreater   = (*Dir)(nil)
	_ fs.NodeUnlinker  = (*Dir)(nil)
	_ fs.NodeRmdirer   = (*Dir)(nil)
	_ fs.NodeRenamer   = (*Dir)(nil)
	_ fs.NodeSymlinker = (*Dir)(nil)
)

func (d *Dir) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	attr, err := d.store.GetAttr(ctx, d.ino)
	if err != nil {
		return syscall.ENOENT
	}
	fillAttr(out, attr)
	return 0
}

func (d *Dir) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	if name == "." {
		fillEntryOut(out, d.store, ctx, d.ino)
		return d.EmbeddedInode(), 0
	}
	if name == ".." {
		if parent, err := d.store.Lookup(ctx, d.ino, ".."); err == nil {
			fillEntryOut(out, d.store, ctx, parent.Ino)
			return d.NewInode(ctx, &Dir{store: d.store, chunks: d.chunks, ino: parent.Ino, uid: d.uid, gid: d.gid},
				fs.StableAttr{Ino: parent.Ino, Mode: fuse.S_IFDIR}), 0
		}
	}
	attr, err := d.store.Lookup(ctx, d.ino, name)
	if err != nil {
		return nil, syscall.ENOENT
	}
	return d.newChild(ctx, attr, out)
}

func (d *Dir) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	entries, err := d.store.Readdir(ctx, d.ino)
	if err != nil {
		return nil, syscall.EIO
	}
	var items []fuse.DirEntry
	items = append(items, fuse.DirEntry{Name: ".", Ino: d.ino, Mode: fuse.S_IFDIR})
	parentIno := d.ino
	if p, err := d.store.Lookup(ctx, d.ino, ".."); err == nil {
		parentIno = p.Ino
	}
	items = append(items, fuse.DirEntry{Name: "..", Ino: parentIno, Mode: fuse.S_IFDIR})
	for name, attr := range entries {
		if name == "." || name == ".." {
			continue
		}
		items = append(items, fuse.DirEntry{Name: name, Ino: attr.Ino, Mode: attr.Mode & 0o7777})
	}
	return fs.NewListDirStream(items), 0
}

func (d *Dir) Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	attr, err := d.store.Mkdir(ctx, d.ino, name, mode, d.uid, d.gid)
	if err != nil {
		return nil, mapMetaErr(err)
	}
	return d.newChild(ctx, attr, out)
}

func (d *Dir) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (node *fs.Inode, fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	mountlog.Infof("Dir Create start: parentIno=%d name=%s flags=0x%x mode=0%o", d.ino, name, flags, mode)
	attr, err := d.store.Create(ctx, d.ino, name, mode, d.uid, d.gid)
	if err != nil {
		mountlog.Errorf("Dir Create failed: parentIno=%d name=%s err=%v", d.ino, name, err)
		return nil, nil, 0, mapMetaErr(err)
	}
	mountlog.Infof("Dir Create success: parentIno=%d name=%s newIno=%d", d.ino, name, attr.Ino)
	inode, errno := d.newChild(ctx, attr, out)
	return inode, nil, fuse.FOPEN_KEEP_CACHE, errno
}

func (d *Dir) Symlink(ctx context.Context, target, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	attr, err := d.store.Symlink(ctx, d.ino, name, target, d.uid, d.gid)
	if err != nil {
		return nil, mapMetaErr(err)
	}
	return d.newChild(ctx, attr, out)
}

func (d *Dir) Unlink(ctx context.Context, name string) syscall.Errno {
	attr, err := d.store.Unlink(ctx, d.ino, name)
	if err != nil {
		return syscall.ENOENT
	}
	d.chunks.DeleteChunks(ctx, attr)
	return 0
}

func (d *Dir) Rmdir(ctx context.Context, name string) syscall.Errno {
	if err := d.store.Rmdir(ctx, d.ino, name); err != nil {
		return mapMetaErr(err)
	}
	return 0
}

func (d *Dir) Rename(ctx context.Context, name string, newParent fs.InodeEmbedder, newName string, flags uint32) syscall.Errno {
	var newParentIno uint64
	switch np := newParent.(type) {
	case *MemoryFS:
		newParentIno = np.rootIno()
	case *Dir:
		newParentIno = np.ino
	default:
		return syscall.EXDEV
	}
	if err := d.store.Rename(ctx, d.ino, newParentIno, name, newName); err != nil {
		return mapMetaErr(err)
	}
	return 0
}

func (d *Dir) newChild(ctx context.Context, attr *meta.Attr, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	return newChildNode(d.NewInode, ctx, d.store, d.chunks, d.uid, d.gid, attr, out)
}

// File represents a regular file.
type File struct {
	fs.Inode
	store  meta.Backend
	chunks *storage.ChunkStore
	ino    uint64
}

var (
	_ fs.NodeGetattrer = (*File)(nil)
	_ fs.NodeOpener    = (*File)(nil)
	_ fs.NodeReader    = (*File)(nil)
	_ fs.NodeWriter    = (*File)(nil)
	_ fs.NodeSetattrer = (*File)(nil)
	_ fs.NodeFsyncer   = (*File)(nil)
	_ fs.NodeReleaser  = (*File)(nil)
)

func (f *File) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	attr, err := f.store.GetAttr(ctx, f.ino)
	if err != nil {
		return syscall.ENOENT
	}
	fillAttr(out, attr)
	return 0
}

func (f *File) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	mountlog.Infof("fuse open ino=%d flags=0x%x", f.ino, flags)
	return nil, fuse.FOPEN_KEEP_CACHE, 0
}

func (f *File) Read(ctx context.Context, fh fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	attr, err := f.store.GetAttr(ctx, f.ino)
	if err != nil {
		return nil, syscall.ENOENT
	}
	n, err := f.chunks.Read(ctx, attr, dest, off)
	if err != nil {
		mountlog.Errorf("fuse read ino=%d off=%d len=%d: %v", f.ino, off, len(dest), err)
		return nil, syscall.EIO
	}
	mountlog.Debugf("fuse read ino=%d off=%d got=%d", f.ino, off, n)
	return fuse.ReadResultData(dest[:n]), 0
}

func (f *File) Write(ctx context.Context, fh fs.FileHandle, data []byte, off int64) (uint32, syscall.Errno) {
	attr, err := f.store.GetAttr(ctx, f.ino)
	if err != nil {
		return 0, syscall.ENOENT
	}
	oldSize := attr.Size
	if err := f.chunks.Write(ctx, attr, data, off); err != nil {
		mountlog.Errorf("fuse write ino=%d off=%d len=%d: %v", f.ino, off, len(data), err)
		return 0, syscall.EIO
	}
	if attr.Size > oldSize {
		_ = f.store.UpdateAttr(ctx, attr)
	}
	mountlog.Debugf("fuse write ino=%d off=%d len=%d size=%d", f.ino, off, len(data), attr.Size)
	return uint32(len(data)), 0
}

func (f *File) Fsync(ctx context.Context, fh fs.FileHandle, flags uint32) syscall.Errno {
	if err := f.chunks.FlushFile(ctx, f.ino); err != nil {
		mountlog.Errorf("fuse fsync ino=%d: %v", f.ino, err)
		return syscall.EIO
	}
	return 0
}

func (f *File) Release(ctx context.Context, fh fs.FileHandle) syscall.Errno {
	if err := f.chunks.FlushFile(ctx, f.ino); err != nil {
		mountlog.Warnf("fuse release flush ino=%d: %v", f.ino, err)
	}
	return 0
}

func (f *File) Setattr(ctx context.Context, fh fs.FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	attr, err := f.store.GetAttr(ctx, f.ino)
	if err != nil {
		return syscall.ENOENT
	}
	if mode, ok := in.GetMode(); ok {
		attr.Mode = (attr.Mode & 0o170000) | (mode & 0o7777)
	}
	if size, ok := in.GetSize(); ok {
		if err := f.chunks.Truncate(ctx, attr, size); err != nil {
			return syscall.EIO
		}
	}
	attr.Mtime = time.Now().Unix()
	if err := f.store.UpdateAttr(ctx, attr); err != nil {
		return syscall.EIO
	}
	fillAttr(out, attr)
	return 0
}

// SymlinkNode represents a symbolic link.
type SymlinkNode struct {
	fs.Inode
	store  meta.Backend
	ino    uint64
	target string
}

var (
	_ fs.NodeGetattrer    = (*SymlinkNode)(nil)
	_ fs.NodeReadlinker   = (*SymlinkNode)(nil)
)

func (s *SymlinkNode) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	attr, err := s.store.GetAttr(ctx, s.ino)
	if err != nil {
		return syscall.ENOENT
	}
	fillAttr(out, attr)
	return 0
}

func (s *SymlinkNode) Readlink(ctx context.Context) ([]byte, syscall.Errno) {
	attr, err := s.store.GetAttr(ctx, s.ino)
	if err != nil {
		return nil, syscall.ENOENT
	}
	return []byte(attr.Target), 0
}

func newChildNode(newInode func(context.Context, fs.InodeEmbedder, fs.StableAttr) *fs.Inode, ctx context.Context, store meta.Backend, chunks *storage.ChunkStore, uid, gid uint32, attr *meta.Attr, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	stable := fs.StableAttr{Ino: attr.Ino, Mode: attr.Mode & 0o7777}
	var child fs.InodeEmbedder
	switch attr.Mode & 0o170000 {
	case 0o040000:
		child = &Dir{store: store, chunks: chunks, ino: attr.Ino, uid: uid, gid: gid}
	case 0o0120000:
		child = &SymlinkNode{store: store, ino: attr.Ino, target: attr.Target}
	default:
		child = &File{store: store, chunks: chunks, ino: attr.Ino}
	}
	inode := newInode(ctx, child, stable)
	fillEntryOut(out, store, ctx, attr.Ino)
	return inode, 0
}

func mapMetaErr(err error) syscall.Errno {
	switch {
	case errors.Is(err, meta.ErrExists):
		return syscall.EEXIST
	case errors.Is(err, meta.ErrNotFound):
		return syscall.ENOENT
	case errors.Is(err, meta.ErrNotEmpty):
		return syscall.ENOTEMPTY
	default:
		return syscall.EIO
	}
}

func fillAttr(out *fuse.AttrOut, attr *meta.Attr) {
	out.Ino = attr.Ino
	out.Mode = attr.Mode
	out.Size = attr.Size
	out.Uid = attr.UID
	out.Gid = attr.GID
	out.Nlink = attr.Nlink
	out.Mtime = uint64(attr.Mtime)
	out.Ctime = uint64(attr.Mtime)
	out.Atime = uint64(attr.Mtime)
	out.Blocks = (attr.Size + 511) / 512
}

func fillEntryAttr(out *fuse.Attr, attr *meta.Attr) {
	out.Ino = attr.Ino
	out.Mode = attr.Mode
	out.Size = attr.Size
	out.Uid = attr.UID
	out.Gid = attr.GID
	out.Nlink = attr.Nlink
	out.Mtime = uint64(attr.Mtime)
	out.Ctime = uint64(attr.Mtime)
	out.Atime = uint64(attr.Mtime)
	out.Blocks = (attr.Size + 511) / 512
}

func fillEntryOut(out *fuse.EntryOut, store meta.Backend, ctx context.Context, ino uint64) {
	attr, err := store.GetAttr(ctx, ino)
	if err != nil {
		return
	}
	fillEntryAttr(&out.Attr, attr)
}
