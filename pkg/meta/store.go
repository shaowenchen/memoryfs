package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	rootIno = 1
	keyPrefix = "memoryfs"
)

// ChunkSize is the default file chunk size (4 MiB).
const ChunkSize = 4 << 20

// Attr holds inode metadata stored in Redis.
type Attr struct {
	Ino    uint64   `json:"ino"`
	Mode   uint32   `json:"mode"`
	Size   uint64   `json:"size"`
	UID    uint32   `json:"uid"`
	GID    uint32   `json:"gid"`
	Mtime  int64    `json:"mtime"`
	Nlink  uint32   `json:"nlink"`
	Chunks []string `json:"chunks,omitempty"`
}

// Store manages filesystem metadata in Redis.
type Store struct {
	rdb *redis.Client
}

// NewStore creates a metadata store backed by Redis.
func NewStore(redisAddr string) (*Store, error) {
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	s := &Store{rdb: rdb}
	if err := s.initRoot(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) initRoot(ctx context.Context) error {
	key := inoKey(rootIno)
	exists, err := s.rdb.Exists(ctx, key).Result()
	if err != nil {
		return err
	}
	if exists > 0 {
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
	if err := s.setAttr(ctx, root); err != nil {
		return err
	}
	return s.rdb.Set(ctx, nextInoKey(), rootIno, 0).Err()
}

func inoKey(ino uint64) string {
	return fmt.Sprintf("%s:ino:%d", keyPrefix, ino)
}

func direntKey(parentIno uint64) string {
	return fmt.Sprintf("%s:dirent:%d", keyPrefix, parentIno)
}

func nextInoKey() string {
	return keyPrefix + ":next_ino"
}

func (s *Store) setAttr(ctx context.Context, attr *Attr) error {
	data, err := json.Marshal(attr)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, inoKey(attr.Ino), data, 0).Err()
}

// GetAttr returns metadata for an inode.
func (s *Store) GetAttr(ctx context.Context, ino uint64) (*Attr, error) {
	data, err := s.rdb.Get(ctx, inoKey(ino)).Bytes()
	if err == redis.Nil {
		return nil, fmt.Errorf("inode %d not found", ino)
	}
	if err != nil {
		return nil, err
	}
	var attr Attr
	if err := json.Unmarshal(data, &attr); err != nil {
		return nil, err
	}
	return &attr, nil
}

// Lookup finds a child inode by name under parentIno.
func (s *Store) Lookup(ctx context.Context, parentIno uint64, name string) (*Attr, error) {
	childIno, err := s.rdb.HGet(ctx, direntKey(parentIno), name).Uint64()
	if err == redis.Nil {
		return nil, fmt.Errorf("entry not found")
	}
	if err != nil {
		return nil, err
	}
	return s.GetAttr(ctx, childIno)
}

// Readdir lists directory entries under parentIno.
func (s *Store) Readdir(ctx context.Context, parentIno uint64) (map[string]*Attr, error) {
	entries, err := s.rdb.HGetAll(ctx, direntKey(parentIno)).Result()
	if err != nil {
		return nil, err
	}
	result := make(map[string]*Attr, len(entries))
	for name, inoStr := range entries {
		var ino uint64
		if _, err := fmt.Sscanf(inoStr, "%d", &ino); err != nil {
			return nil, err
		}
		attr, err := s.GetAttr(ctx, ino)
		if err != nil {
			return nil, err
		}
		result[name] = attr
	}
	return result, nil
}

func (s *Store) allocIno(ctx context.Context) (uint64, error) {
	return s.rdb.Incr(ctx, nextInoKey()).Uint64()
}

// Mkdir creates a directory under parentIno.
func (s *Store) Mkdir(ctx context.Context, parentIno uint64, name string, mode uint32, uid, gid uint32) (*Attr, error) {
	if err := s.checkName(name); err != nil {
		return nil, err
	}
	exists, err := s.rdb.HExists(ctx, direntKey(parentIno), name).Result()
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("file exists")
	}
	ino, err := s.allocIno(ctx)
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
	pipe := s.rdb.Pipeline()
	pipe.HSet(ctx, direntKey(parentIno), name, ino)
	data, _ := json.Marshal(attr)
	pipe.Set(ctx, inoKey(ino), data, 0)
	pipe.HSet(ctx, direntKey(ino), ".", ino)
	pipe.HSet(ctx, direntKey(ino), "..", parentIno)
	_, err = pipe.Exec(ctx)
	if err != nil {
		return nil, err
	}
	return attr, nil
}

// Create creates a regular file under parentIno.
func (s *Store) Create(ctx context.Context, parentIno uint64, name string, mode uint32, uid, gid uint32) (*Attr, error) {
	if err := s.checkName(name); err != nil {
		return nil, err
	}
	exists, err := s.rdb.HExists(ctx, direntKey(parentIno), name).Result()
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("file exists")
	}
	ino, err := s.allocIno(ctx)
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
	pipe := s.rdb.Pipeline()
	pipe.HSet(ctx, direntKey(parentIno), name, ino)
	data, _ := json.Marshal(attr)
	pipe.Set(ctx, inoKey(ino), data, 0)
	_, err = pipe.Exec(ctx)
	if err != nil {
		return nil, err
	}
	return attr, nil
}

// Unlink removes a file entry from parentIno.
func (s *Store) Unlink(ctx context.Context, parentIno uint64, name string) (*Attr, error) {
	childIno, err := s.rdb.HGet(ctx, direntKey(parentIno), name).Uint64()
	if err == redis.Nil {
		return nil, fmt.Errorf("entry not found")
	}
	if err != nil {
		return nil, err
	}
	attr, err := s.GetAttr(ctx, childIno)
	if err != nil {
		return nil, err
	}
	if attr.Mode&0o170000 == 0o040000 {
		return nil, fmt.Errorf("is a directory")
	}
	pipe := s.rdb.Pipeline()
	pipe.HDel(ctx, direntKey(parentIno), name)
	pipe.Del(ctx, inoKey(childIno))
	_, err = pipe.Exec(ctx)
	if err != nil {
		return nil, err
	}
	return attr, nil
}

// Rmdir removes an empty directory.
func (s *Store) Rmdir(ctx context.Context, parentIno uint64, name string) error {
	childIno, err := s.rdb.HGet(ctx, direntKey(parentIno), name).Uint64()
	if err == redis.Nil {
		return fmt.Errorf("entry not found")
	}
	if err != nil {
		return err
	}
	attr, err := s.GetAttr(ctx, childIno)
	if err != nil {
		return err
	}
	if attr.Mode&0o170000 != 0o040000 {
		return fmt.Errorf("not a directory")
	}
	count, err := s.rdb.HLen(ctx, direntKey(childIno)).Result()
	if err != nil {
		return err
	}
	if count > 2 {
		return fmt.Errorf("directory not empty")
	}
	pipe := s.rdb.Pipeline()
	pipe.HDel(ctx, direntKey(parentIno), name)
	pipe.Del(ctx, inoKey(childIno))
	pipe.Del(ctx, direntKey(childIno))
	_, err = pipe.Exec(ctx)
	return err
}

// Rename moves/renames an entry.
func (s *Store) Rename(ctx context.Context, oldParent, newParent uint64, oldName, newName string) error {
	if err := s.checkName(newName); err != nil {
		return err
	}
	childIno, err := s.rdb.HGet(ctx, direntKey(oldParent), oldName).Uint64()
	if err == redis.Nil {
		return fmt.Errorf("entry not found")
	}
	if err != nil {
		return err
	}
	exists, err := s.rdb.HExists(ctx, direntKey(newParent), newName).Result()
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("file exists")
	}
	pipe := s.rdb.Pipeline()
	pipe.HDel(ctx, direntKey(oldParent), oldName)
	pipe.HSet(ctx, direntKey(newParent), newName, childIno)
	_, err = pipe.Exec(ctx)
	return err
}

// UpdateAttr saves modified inode metadata.
func (s *Store) UpdateAttr(ctx context.Context, attr *Attr) error {
	return s.setAttr(ctx, attr)
}

// ResolvePath resolves an absolute path to an inode (for debugging).
func (s *Store) ResolvePath(ctx context.Context, p string) (*Attr, error) {
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

func (s *Store) checkName(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("invalid name")
	}
	return nil
}

// Close closes the Redis connection.
func (s *Store) Close() error {
	return s.rdb.Close()
}

// RegisterWorker adds a worker URL to the cluster registry.
func (s *Store) RegisterWorker(ctx context.Context, url string) error {
	return s.rdb.SAdd(ctx, keyPrefix+":workers", url).Err()
}

// ListWorkers returns registered worker URLs.
func (s *Store) ListWorkers(ctx context.Context) ([]string, error) {
	return s.rdb.SMembers(ctx, keyPrefix+":workers").Result()
}

// ChunkID generates a chunk identifier for an inode and chunk index.
func ChunkID(ino uint64, index int) string {
	return fmt.Sprintf("%d_%d", ino, index)
}
