package leveldb

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"io"

	"github.com/chrislusf/seaweedfs/weed/filer2"
	"github.com/chrislusf/seaweedfs/weed/glog"
	weed_util "github.com/chrislusf/seaweedfs/weed/util"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	leveldb_util "github.com/syndtr/goleveldb/leveldb/util"
)

func init() {
	filer2.Stores = append(filer2.Stores, &LevelDB2Store{})
}

// known theoretically 128 bit MD5 collision of 2 directories.
// (but really? please show some real examples)
type LevelDB2Store struct {
	db *leveldb.DB
}

func (store *LevelDB2Store) GetName() string {
	return "leveldb2"
}

func (store *LevelDB2Store) Initialize(configuration weed_util.Configuration) (err error) {
	dir := configuration.GetString("dir")
	return store.initialize(dir)
}

func (store *LevelDB2Store) initialize(dir string) (err error) {
	glog.Infof("filer store leveldb2 dir: %s", dir)
	if err := weed_util.TestFolderWritable(dir); err != nil {
		return fmt.Errorf("Check Level Folder %s Writable: %s", dir, err)
	}

	opts := &opt.Options{
		BlockCacheCapacity:            32 * 1024 * 1024, // default value is 8MiB
		WriteBuffer:                   16 * 1024 * 1024, // default value is 4MiB
		CompactionTableSizeMultiplier: 10,
	}

	if store.db, err = leveldb.OpenFile(dir, opts); err != nil {
		glog.Infof("filer store open dir %s: %v", dir, err)
		return
	}
	return
}

func (store *LevelDB2Store) BeginTransaction(ctx context.Context) (context.Context, error) {
	return ctx, nil
}
func (store *LevelDB2Store) CommitTransaction(ctx context.Context) error {
	return nil
}
func (store *LevelDB2Store) RollbackTransaction(ctx context.Context) error {
	return nil
}

func (store *LevelDB2Store) InsertEntry(ctx context.Context, entry *filer2.Entry) (err error) {
	key := genKey(entry.DirAndName())

	value, err := entry.EncodeAttributesAndChunks()
	if err != nil {
		return fmt.Errorf("encoding %s %+v: %v", entry.FullPath, entry.Attr, err)
	}

	err = store.db.Put(key, value, nil)

	if err != nil {
		return fmt.Errorf("persisting %s : %v", entry.FullPath, err)
	}

	// println("saved", entry.FullPath, "chunks", len(entry.Chunks))

	return nil
}

func (store *LevelDB2Store) UpdateEntry(ctx context.Context, entry *filer2.Entry) (err error) {

	return store.InsertEntry(ctx, entry)
}

func (store *LevelDB2Store) FindEntry(ctx context.Context, fullpath filer2.FullPath) (entry *filer2.Entry, err error) {
	key := genKey(fullpath.DirAndName())

	data, err := store.db.Get(key, nil)

	if err == leveldb.ErrNotFound {
		return nil, filer2.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get %s : %v", entry.FullPath, err)
	}

	entry = &filer2.Entry{
		FullPath: fullpath,
	}
	err = entry.DecodeAttributesAndChunks(data)
	if err != nil {
		return entry, fmt.Errorf("decode %s : %v", entry.FullPath, err)
	}

	// println("read", entry.FullPath, "chunks", len(entry.Chunks), "data", len(data), string(data))

	return entry, nil
}

func (store *LevelDB2Store) DeleteEntry(ctx context.Context, fullpath filer2.FullPath) (err error) {
	key := genKey(fullpath.DirAndName())

	err = store.db.Delete(key, nil)
	if err != nil {
		return fmt.Errorf("delete %s : %v", fullpath, err)
	}

	return nil
}

func (store *LevelDB2Store) ListDirectoryEntries(ctx context.Context, fullpath filer2.FullPath, startFileName string, inclusive bool,
	limit int) (entries []*filer2.Entry, err error) {

	directoryPrefix := genDirectoryKeyPrefix(fullpath, "")

	iter := store.db.NewIterator(&leveldb_util.Range{Start: genDirectoryKeyPrefix(fullpath, startFileName)}, nil)
	for iter.Next() {
		key := iter.Key()
		if !bytes.HasPrefix(key, directoryPrefix) {
			break
		}
		fileName := getNameFromKey(key)
		if fileName == "" {
			continue
		}
		if fileName == startFileName && !inclusive {
			continue
		}
		limit--
		if limit < 0 {
			break
		}
		entry := &filer2.Entry{
			FullPath: filer2.NewFullPath(string(fullpath), fileName),
		}
		if decodeErr := entry.DecodeAttributesAndChunks(iter.Value()); decodeErr != nil {
			err = decodeErr
			glog.V(0).Infof("list %s : %v", entry.FullPath, err)
			break
		}
		entries = append(entries, entry)
	}
	iter.Release()

	return entries, err
}

func genKey(dirPath, fileName string) (key []byte) {
	key = hashToBytes(dirPath)
	key = append(key, []byte(fileName)...)
	return key
}

func genDirectoryKeyPrefix(fullpath filer2.FullPath, startFileName string) (keyPrefix []byte) {
	keyPrefix = hashToBytes(string(fullpath))
	if len(startFileName) > 0 {
		keyPrefix = append(keyPrefix, []byte(startFileName)...)
	}
	return keyPrefix
}

func getNameFromKey(key []byte) string {

	return string(key[8:])

}

func hashToBytes(dir string) []byte {
	h := md5.New()
	io.WriteString(h, dir)

	b := h.Sum(nil)

	return b
}