package fusefs

import (
	"context"
	"log"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"

	"github.com/shaowenchen/memoryfs/pkg/meta"
	"github.com/shaowenchen/memoryfs/pkg/storage"
)

// MemoryFS is the root FUSE node backed by Redis metadata and worker storage.
type MemoryFS struct {
	fs.Inode
	store  *meta.Store
	chunks *storage.ChunkStore
	uid    uint32
	gid    uint32
}

// NewRoot creates the root filesystem node.
func NewRoot(store *meta.Store, chunks *storage.ChunkStore, uid, gid uint32) *MemoryFS {
	return &MemoryFS{
		store:  store,
		chunks: chunks,
		uid:    uid,
		gid:    gid,
	}
}

var (
	_ fs.NodeLookuper   = (*MemoryFS)(nil)
	_ fs.NodeGetattrer  = (*MemoryFS)(nil)
	_ fs.NodeReaddirer  = (*MemoryFS)(nil)
	_ fs.NodeMkdirer    = (*MemoryFS)(nil)
	_ fs.NodeCreater    = (*MemoryFS)(nil)
	_ fs.NodeUnlinker   = (*MemoryFS)(nil)
	_ fs.NodeRmdirer    = (*MemoryFS)(nil)
	_ fs.NodeRenamer    = (*MemoryFS)(nil)
)

func (m *MemoryFS) rootIno() uint64 { return meta.RootIno() }

func (m *MemoryFS) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	attr, err := m.store.GetAttr(ctx, m.rootIno())
	if err != nil {
		return syscall.ENOENT
	}
	fillAttr(out, attr)
	return 0
}

func (m *MemoryFS) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	if name == "." {
		fillEntryOut(out, m.store, ctx, m.rootIno())
		return m.EmbeddedInode(), 0
	}
	if name == ".." {
		fillEntryOut(out, m.store, ctx, m.rootIno())
		return m.EmbeddedInode(), 0
	}
	attr, err := m.store.Lookup(ctx, m.rootIno(), name)
	if err != nil {
		return nil, syscall.ENOENT
	}
	return m.newChild(ctx, name, attr, out)
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
		if err.Error() == "file exists" {
			return nil, syscall.EEXIST
		}
		return nil, syscall.EIO
	}
	return m.newChild(ctx, name, attr, out)
}

func (m *MemoryFS) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (node *fs.Inode, fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	attr, err := m.store.Create(ctx, m.rootIno(), name, mode, m.uid, m.gid)
	if err != nil {
		if err.Error() == "file exists" {
			return nil, nil, 0, syscall.EEXIST
		}
		return nil, nil, 0, syscall.EIO
	}
	inode, errno := m.newChild(ctx, name, attr, out)
	return inode, nil, fuse.FOPEN_KEEP_CACHE, errno
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
		if err.Error() == "directory not empty" {
			return syscall.ENOTEMPTY
		}
		return syscall.ENOENT
	}
	return 0
}

func (m *MemoryFS) Rename(ctx context.Context, name string, newParent fs.InodeEmbedder, newName string, flags uint32) syscall.Errno {
	np, ok := newParent.(*MemoryFS)
	if !ok {
		if dir, ok := newParent.(*Dir); ok {
			if err := m.store.Rename(ctx, m.rootIno(), dir.ino, name, newName); err != nil {
				if err.Error() == "file exists" {
					return syscall.EEXIST
				}
				return syscall.EIO
			}
			return 0
		}
		return syscall.EXDEV
	}
	if err := m.store.Rename(ctx, m.rootIno(), np.rootIno(), name, newName); err != nil {
		if err.Error() == "file exists" {
			return syscall.EEXIST
		}
		return syscall.EIO
	}
	return 0
}

func (m *MemoryFS) newChild(ctx context.Context, name string, attr *meta.Attr, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	stable := fs.StableAttr{Ino: attr.Ino, Mode: attr.Mode & 0o7777}
	var child fs.InodeEmbedder
	if attr.Mode&0o170000 == 0o040000 {
		child = &Dir{store: m.store, chunks: m.chunks, ino: attr.Ino, uid: m.uid, gid: m.gid}
	} else {
		child = &File{store: m.store, chunks: m.chunks, ino: attr.Ino}
	}
	inode := m.NewInode(ctx, child, stable)
	fillEntryOut(out, m.store, ctx, attr.Ino)
	return inode, 0
}

// Dir represents a subdirectory.
type Dir struct {
	fs.Inode
	store  *meta.Store
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
		parentIno, _ := d.store.Lookup(ctx, d.ino, "..")
		if parentIno != nil {
			fillEntryOut(out, d.store, ctx, parentIno.Ino)
			return d.NewInode(ctx, &Dir{store: d.store, chunks: d.chunks, ino: parentIno.Ino, uid: d.uid, gid: d.gid},
				fs.StableAttr{Ino: parentIno.Ino, Mode: fuse.S_IFDIR}), 0
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
		if err.Error() == "file exists" {
			return nil, syscall.EEXIST
		}
		return nil, syscall.EIO
	}
	return d.newChild(ctx, attr, out)
}

func (d *Dir) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (node *fs.Inode, fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	attr, err := d.store.Create(ctx, d.ino, name, mode, d.uid, d.gid)
	if err != nil {
		if err.Error() == "file exists" {
			return nil, nil, 0, syscall.EEXIST
		}
		return nil, nil, 0, syscall.EIO
	}
	inode, errno := d.newChild(ctx, attr, out)
	return inode, nil, fuse.FOPEN_KEEP_CACHE, errno
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
		if err.Error() == "directory not empty" {
			return syscall.ENOTEMPTY
		}
		return syscall.ENOENT
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
		if err.Error() == "file exists" {
			return syscall.EEXIST
		}
		return syscall.EIO
	}
	return 0
}

func (d *Dir) newChild(ctx context.Context, attr *meta.Attr, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	stable := fs.StableAttr{Ino: attr.Ino, Mode: attr.Mode & 0o7777}
	var child fs.InodeEmbedder
	if attr.Mode&0o170000 == 0o040000 {
		child = &Dir{store: d.store, chunks: d.chunks, ino: attr.Ino, uid: d.uid, gid: d.gid}
	} else {
		child = &File{store: d.store, chunks: d.chunks, ino: attr.Ino}
	}
	inode := d.NewInode(ctx, child, stable)
	fillEntryOut(out, d.store, ctx, attr.Ino)
	return inode, 0
}

// File represents a regular file.
type File struct {
	fs.Inode
	store  *meta.Store
	chunks *storage.ChunkStore
	ino    uint64
}

var (
	_ fs.NodeGetattrer = (*File)(nil)
	_ fs.NodeReader    = (*File)(nil)
	_ fs.NodeWriter    = (*File)(nil)
	_ fs.NodeSetattrer = (*File)(nil)
)

func (f *File) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	attr, err := f.store.GetAttr(ctx, f.ino)
	if err != nil {
		return syscall.ENOENT
	}
	fillAttr(out, attr)
	return 0
}

func (f *File) Read(ctx context.Context, fh fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	attr, err := f.store.GetAttr(ctx, f.ino)
	if err != nil {
		return nil, syscall.ENOENT
	}
	n, err := f.chunks.Read(ctx, attr, dest, off)
	if err != nil {
		log.Printf("read error ino=%d off=%d: %v", f.ino, off, err)
		return nil, syscall.EIO
	}
	return fuse.ReadResultData(dest[:n]), 0
}

func (f *File) Write(ctx context.Context, fh fs.FileHandle, data []byte, off int64) (uint32, syscall.Errno) {
	attr, err := f.store.GetAttr(ctx, f.ino)
	if err != nil {
		return 0, syscall.ENOENT
	}
	if err := f.chunks.Write(ctx, attr, data, off); err != nil {
		log.Printf("write error ino=%d off=%d: %v", f.ino, off, err)
		return 0, syscall.EIO
	}
	attr.Mtime = time.Now().Unix()
	if err := f.store.UpdateAttr(ctx, attr); err != nil {
		return 0, syscall.EIO
	}
	return uint32(len(data)), 0
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

func fillEntryOut(out *fuse.EntryOut, store *meta.Store, ctx context.Context, ino uint64) {
	attr, err := store.GetAttr(ctx, ino)
	if err != nil {
		return
	}
	fillEntryAttr(&out.Attr, attr)
}
