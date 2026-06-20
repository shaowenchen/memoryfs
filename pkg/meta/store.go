package meta

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/shaowenchen/memoryfs/pkg/kv"
)

const (
	rootIno      = 1
	keyPrefix    = "memoryfs"
	inodeIndexKey = keyPrefix + ":inodeindex"
)

var (
	ErrNotFound    = errors.New("entry not found")
	ErrExists      = errors.New("file exists")
	ErrNotDir      = errors.New("not a directory")
	ErrIsDir       = errors.New("is a directory")
	ErrNotEmpty    = errors.New("directory not empty")
	ErrInvalidName = errors.New("invalid name")
)

// ChunkSize is the default file chunk size (4 MiB).
const ChunkSize = 4 << 20

// Attr holds inode metadata.
type Attr struct {
	Ino    uint64   `json:"ino"`
	Mode   uint32   `json:"mode"`
	Size   uint64   `json:"size"`
	UID    uint32   `json:"uid"`
	GID    uint32   `json:"gid"`
	Mtime    int64    `json:"mtime"`
	Nlink    uint32   `json:"nlink"`
	ExpireAt int64    `json:"expire_at,omitempty"`
	Target   string   `json:"target,omitempty"`
	Chunks []string `json:"chunks,omitempty"`
}

// Backend is the metadata storage interface.
type Backend interface {
	GetAttr(ctx context.Context, ino uint64) (*Attr, error)
	Lookup(ctx context.Context, parentIno uint64, name string) (*Attr, error)
	Readdir(ctx context.Context, parentIno uint64) (map[string]*Attr, error)
	Mkdir(ctx context.Context, parentIno uint64, name string, mode, uid, gid uint32) (*Attr, error)
	Create(ctx context.Context, parentIno uint64, name string, mode, uid, gid uint32) (*Attr, error)
	Symlink(ctx context.Context, parentIno uint64, name, target string, uid, gid uint32) (*Attr, error)
	Unlink(ctx context.Context, parentIno uint64, name string) (*Attr, error)
	Rmdir(ctx context.Context, parentIno uint64, name string) error
	Rename(ctx context.Context, oldParent, newParent uint64, oldName, newName string) error
	UpdateAttr(ctx context.Context, attr *Attr) error
	ListNodes(ctx context.Context) ([]string, error)
	ListInos(ctx context.Context) ([]uint64, error)
	PurgeInode(ctx context.Context, ino uint64) error
	Close() error
}

type inodeLink struct {
	Parent uint64 `json:"parent"`
	Name   string `json:"name"`
}

// LocalStore implements Backend on top of kv.KV.
type LocalStore struct {
	kv kv.KV
}

// NewLocalStore creates a metadata store backed by embedded KV.
func NewLocalStore(store kv.KV) (*LocalStore, error) {
	s := &LocalStore{kv: store}
	if err := s.initRoot(context.Background()); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *LocalStore) initRoot(ctx context.Context) error {
	exists, err := s.kv.Exists(inoKey(rootIno))
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if lc, ok := s.kv.(interface{ IsLeader() bool }); ok && !lc.IsLeader() {
		return nil
	}
	root := &Attr{
		Ino:   rootIno,
		Mode:  0o040755,
		UID:   uint32(syscallGetuid()),
		GID:   uint32(syscallGetgid()),
		Mtime: time.Now().Unix(),
		Nlink: 2,
	}
	data, err := json.Marshal(root)
	if err != nil {
		return err
	}
	if err := s.kv.Batch([]kv.Op{
		{Type: kv.OpSet, Key: inoKey(rootIno), Value: data},
		{Type: kv.OpIncr, Key: nextInoKey()},
		{Type: kv.OpSAdd, Key: inodeIndexKey, Member: strconv.FormatUint(rootIno, 10)},
	}); err != nil {
		if isNotLeaderErr(err) {
			return nil
		}
		return err
	}
	return nil
}

// IsNotLeaderErr reports raft write failures while this node is not leader.
func IsNotLeaderErr(err error) bool {
	return isNotLeaderErr(err)
}

func isNotLeaderErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "not leader")
}

// EnsureRoot creates the root inode when this node is the raft leader.
func (s *LocalStore) EnsureRoot(ctx context.Context) error {
	return s.initRoot(ctx)
}

func inoKey(ino uint64) string          { return fmt.Sprintf("%s:ino:%d", keyPrefix, ino) }
func direntKey(parentIno uint64) string  { return fmt.Sprintf("%s:dirent:%d", keyPrefix, parentIno) }
func nextInoKey() string                 { return keyPrefix + ":next_ino" }
func inodeLinkKey(ino uint64) string     { return fmt.Sprintf("%s:ilink:%d", keyPrefix, ino) }

func encodeInodeLink(parentIno uint64, name string) ([]byte, error) {
	return json.Marshal(inodeLink{Parent: parentIno, Name: name})
}

func (s *LocalStore) GetAttr(_ context.Context, ino uint64) (*Attr, error) {
	data, err := s.kv.Get(inoKey(ino))
	if err != nil {
		return nil, ErrNotFound
	}
	var attr Attr
	if err := json.Unmarshal(data, &attr); err != nil {
		return nil, err
	}
	return &attr, nil
}

func (s *LocalStore) Lookup(_ context.Context, parentIno uint64, name string) (*Attr, error) {
	data, err := s.kv.HGet(direntKey(parentIno), name)
	if err != nil {
		return nil, ErrNotFound
	}
	ino, err := strconv.ParseUint(string(data), 10, 64)
	if err != nil {
		return nil, err
	}
	return s.GetAttr(context.Background(), ino)
}

func (s *LocalStore) Readdir(_ context.Context, parentIno uint64) (map[string]*Attr, error) {
	entries, err := s.kv.HGetAll(direntKey(parentIno))
	if err != nil {
		return nil, err
	}
	result := make(map[string]*Attr, len(entries))
	for name, inoBytes := range entries {
		ino, err := strconv.ParseUint(string(inoBytes), 10, 64)
		if err != nil {
			return nil, err
		}
		attr, err := s.GetAttr(context.Background(), ino)
		if err != nil {
			return nil, err
		}
		result[name] = attr
	}
	return result, nil
}

func (s *LocalStore) allocIno() (uint64, error) {
	return s.kv.Incr(nextInoKey())
}

func (s *LocalStore) Mkdir(_ context.Context, parentIno uint64, name string, mode, uid, gid uint32) (*Attr, error) {
	if err := checkName(name); err != nil {
		return nil, err
	}
	exists, err := s.kv.HExists(direntKey(parentIno), name)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrExists
	}
	ino, err := s.allocIno()
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	attr := &Attr{
		Ino:   ino,
		Mode:  mode | 0o040000,
		UID:   uid,
		GID:   gid,
		Mtime: now,
		Nlink: 2,
	}
	data, err := json.Marshal(attr)
	if err != nil {
		return nil, err
	}
	linkData, err := encodeInodeLink(parentIno, name)
	if err != nil {
		return nil, err
	}
	err = s.kv.Batch([]kv.Op{
		{Type: kv.OpHSet, Key: direntKey(parentIno), Field: name, Value: []byte(strconv.FormatUint(ino, 10))},
		{Type: kv.OpSet, Key: inoKey(ino), Value: data},
		{Type: kv.OpHSet, Key: direntKey(ino), Field: ".", Value: []byte(strconv.FormatUint(ino, 10))},
		{Type: kv.OpHSet, Key: direntKey(ino), Field: "..", Value: []byte(strconv.FormatUint(parentIno, 10))},
		{Type: kv.OpSAdd, Key: inodeIndexKey, Member: strconv.FormatUint(ino, 10)},
		{Type: kv.OpSet, Key: inodeLinkKey(ino), Value: linkData},
	})
	if err != nil {
		return nil, err
	}
	return attr, nil
}

func (s *LocalStore) Create(_ context.Context, parentIno uint64, name string, mode, uid, gid uint32) (*Attr, error) {
	if err := checkName(name); err != nil {
		return nil, err
	}
	exists, err := s.kv.HExists(direntKey(parentIno), name)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrExists
	}
	ino, err := s.allocIno()
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	attr := &Attr{
		Ino:   ino,
		Mode:  mode | 0o0100000,
		UID:   uid,
		GID:   gid,
		Mtime: now,
		Nlink: 1,
	}
	data, err := json.Marshal(attr)
	if err != nil {
		return nil, err
	}
	linkData, err := encodeInodeLink(parentIno, name)
	if err != nil {
		return nil, err
	}
	err = s.kv.Batch([]kv.Op{
		{Type: kv.OpHSet, Key: direntKey(parentIno), Field: name, Value: []byte(strconv.FormatUint(ino, 10))},
		{Type: kv.OpSet, Key: inoKey(ino), Value: data},
		{Type: kv.OpSAdd, Key: inodeIndexKey, Member: strconv.FormatUint(ino, 10)},
		{Type: kv.OpSet, Key: inodeLinkKey(ino), Value: linkData},
	})
	if err != nil {
		return nil, err
	}
	return attr, nil
}

func (s *LocalStore) Symlink(_ context.Context, parentIno uint64, name, target string, uid, gid uint32) (*Attr, error) {
	if err := checkName(name); err != nil {
		return nil, err
	}
	exists, err := s.kv.HExists(direntKey(parentIno), name)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrExists
	}
	ino, err := s.allocIno()
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	attr := &Attr{
		Ino:    ino,
		Mode:   0o0120777,
		UID:    uid,
		GID:    gid,
		Mtime:  now,
		Nlink:  1,
		Target: target,
		Size:   uint64(len(target)),
	}
	data, err := json.Marshal(attr)
	if err != nil {
		return nil, err
	}
	linkData, err := encodeInodeLink(parentIno, name)
	if err != nil {
		return nil, err
	}
	err = s.kv.Batch([]kv.Op{
		{Type: kv.OpHSet, Key: direntKey(parentIno), Field: name, Value: []byte(strconv.FormatUint(ino, 10))},
		{Type: kv.OpSet, Key: inoKey(ino), Value: data},
		{Type: kv.OpSAdd, Key: inodeIndexKey, Member: strconv.FormatUint(ino, 10)},
		{Type: kv.OpSet, Key: inodeLinkKey(ino), Value: linkData},
	})
	if err != nil {
		return nil, err
	}
	return attr, nil
}

func (s *LocalStore) Unlink(_ context.Context, parentIno uint64, name string) (*Attr, error) {
	data, err := s.kv.HGet(direntKey(parentIno), name)
	if err != nil {
		return nil, ErrNotFound
	}
	ino, err := strconv.ParseUint(string(data), 10, 64)
	if err != nil {
		return nil, err
	}
	attr, err := s.GetAttr(context.Background(), ino)
	if err != nil {
		return nil, err
	}
	if attr.Mode&0o170000 == 0o040000 {
		return nil, ErrIsDir
	}
	err = s.kv.Batch([]kv.Op{
		{Type: kv.OpHDel, Key: direntKey(parentIno), Field: name},
		{Type: kv.OpDel, Key: inoKey(ino)},
		{Type: kv.OpSRem, Key: inodeIndexKey, Member: strconv.FormatUint(ino, 10)},
		{Type: kv.OpDel, Key: inodeLinkKey(ino)},
	})
	if err != nil {
		return nil, err
	}
	return attr, nil
}

func (s *LocalStore) Rmdir(_ context.Context, parentIno uint64, name string) error {
	data, err := s.kv.HGet(direntKey(parentIno), name)
	if err != nil {
		return ErrNotFound
	}
	ino, err := strconv.ParseUint(string(data), 10, 64)
	if err != nil {
		return err
	}
	attr, err := s.GetAttr(context.Background(), ino)
	if err != nil {
		return err
	}
	if attr.Mode&0o170000 != 0o040000 {
		return ErrNotDir
	}
	count, err := s.kv.HLen(direntKey(ino))
	if err != nil {
		return err
	}
	if count > 2 {
		return ErrNotEmpty
	}
	return s.kv.Batch([]kv.Op{
		{Type: kv.OpHDel, Key: direntKey(parentIno), Field: name},
		{Type: kv.OpDel, Key: inoKey(ino)},
		{Type: kv.OpDel, Key: direntKey(ino)},
		{Type: kv.OpSRem, Key: inodeIndexKey, Member: strconv.FormatUint(ino, 10)},
		{Type: kv.OpDel, Key: inodeLinkKey(ino)},
	})
}

func (s *LocalStore) Rename(_ context.Context, oldParent, newParent uint64, oldName, newName string) error {
	if err := checkName(newName); err != nil {
		return err
	}
	_, err := s.kv.HGet(direntKey(oldParent), oldName)
	if err != nil {
		return ErrNotFound
	}
	exists, err := s.kv.HExists(direntKey(newParent), newName)
	if err != nil {
		return err
	}
	if exists {
		return ErrExists
	}
	child, err := s.kv.HGet(direntKey(oldParent), oldName)
	if err != nil {
		return err
	}
	ino, err := strconv.ParseUint(string(child), 10, 64)
	if err != nil {
		return err
	}
	linkData, err := encodeInodeLink(newParent, newName)
	if err != nil {
		return err
	}
	return s.kv.Batch([]kv.Op{
		{Type: kv.OpHDel, Key: direntKey(oldParent), Field: oldName},
		{Type: kv.OpHSet, Key: direntKey(newParent), Field: newName, Value: child},
		{Type: kv.OpSet, Key: inodeLinkKey(ino), Value: linkData},
	})
}

func (s *LocalStore) UpdateAttr(_ context.Context, attr *Attr) error {
	data, err := json.Marshal(attr)
	if err != nil {
		return err
	}
	return s.kv.Set(inoKey(attr.Ino), data)
}

func (s *LocalStore) ListNodes(_ context.Context) ([]string, error) {
	fields, err := s.kv.HGetAll(raftnodeNodesKey())
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(fields))
	for _, v := range fields {
		out = append(out, string(v))
	}
	return out, nil
}

func (s *LocalStore) ListInos(_ context.Context) ([]uint64, error) {
	members, err := s.kv.SMembers(inodeIndexKey)
	if err != nil {
		return nil, err
	}
	out := make([]uint64, 0, len(members))
	for _, member := range members {
		ino, err := strconv.ParseUint(member, 10, 64)
		if err != nil {
			continue
		}
		out = append(out, ino)
	}
	return out, nil
}

func (s *LocalStore) PurgeInode(_ context.Context, ino uint64) error {
	if ino == rootIno {
		return fmt.Errorf("cannot purge root inode")
	}
	ops := []kv.Op{
		{Type: kv.OpDel, Key: inoKey(ino)},
		{Type: kv.OpSRem, Key: inodeIndexKey, Member: strconv.FormatUint(ino, 10)},
		{Type: kv.OpDel, Key: inodeLinkKey(ino)},
	}
	if data, err := s.kv.Get(inodeLinkKey(ino)); err == nil {
		var link inodeLink
		if err := json.Unmarshal(data, &link); err == nil {
			ops = append([]kv.Op{
				{Type: kv.OpHDel, Key: direntKey(link.Parent), Field: link.Name},
			}, ops...)
		}
	}
	return s.kv.Batch(ops)
}

func (s *LocalStore) Close() error { return s.kv.Close() }

// ResolvePath resolves an absolute path to an inode.
func (s *LocalStore) ResolvePath(ctx context.Context, p string) (*Attr, error) {
	p = path.Clean("/" + strings.TrimPrefix(p, "/"))
	if p == "/" {
		return s.GetAttr(ctx, rootIno)
	}
	parts := strings.Split(strings.TrimPrefix(p, "/"), "/")
	ino := uint64(rootIno)
	for _, part := range parts {
		if part == "" {
			continue
		}
		attr, err := s.Lookup(ctx, ino, part)
		if err != nil {
			return nil, err
		}
		ino = attr.Ino
	}
	return s.GetAttr(ctx, ino)
}

// RootIno returns the root inode number.
func RootIno() uint64 { return rootIno }

// ChunkID generates a chunk identifier.
func ChunkID(ino uint64, index int) string {
	return fmt.Sprintf("%d_%d", ino, index)
}

func checkName(name string) error {
	if name == "" || name == "." || name == ".." {
		return ErrInvalidName
	}
	return nil
}

func raftnodeNodesKey() string { return "memoryfs:nodes" }

// MapError converts store errors to human-readable messages for HTTP API.
func MapError(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, ErrExists):
		return "file exists"
	case errors.Is(err, ErrNotFound):
		return "entry not found"
	case errors.Is(err, ErrNotEmpty):
		return "directory not empty"
	case errors.Is(err, ErrIsDir):
		return "is a directory"
	case errors.Is(err, ErrNotDir):
		return "not a directory"
	default:
		return err.Error()
	}
}
