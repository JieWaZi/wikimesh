package index

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
	// sqlite-vec 通过空导入注册 vec0 虚表模块，VSearch 的 chunk 近邻召回依赖它。
	_ "modernc.org/sqlite/vec"
)

// DB 管理 SQLite 连接。
// 设计成“一条写连接 + 一个读连接池”：写入统一走 WriteTx 串行化，
// 读取走只读连接池。配合 WAL 模式后，查询和索引写入不容易互相阻塞。
type DB struct {
	write     *sql.DB    // write 是唯一写连接，所有写事务都通过它执行。
	read      *sql.DB    // read 是只读连接池，供 Search/VSearch/Query 使用。
	writeMu   sync.Mutex // writeMu 保证任意时刻只有一个写事务。
	closeOnce sync.Once  // closeOnce 保证 Close 可以被重复调用。
}

// Open 打开数据库并创建检索需要的表。
// path 的父目录不存在时会自动创建，方便 CLI 直接写到 .wikimesh/wiki.db。
func Open(path string) (*DB, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("index.Open: create parent dir: %w", err)
		}
	}

	writeDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("index.Open: %w", err)
	}
	writeDB.SetMaxOpenConns(1)

	// WAL 允许读写并发；busy_timeout 避免短暂锁冲突立刻失败。
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
		"PRAGMA synchronous=NORMAL",
	} {
		if _, err := writeDB.Exec(pragma); err != nil {
			writeDB.Close()
			return nil, fmt.Errorf("index.Open: %s: %w", pragma, err)
		}
	}

	readDB, err := sql.Open("sqlite", path+"?mode=ro")
	if err != nil {
		writeDB.Close()
		return nil, fmt.Errorf("index.Open: read pool: %w", err)
	}
	readDB.SetMaxOpenConns(4)
	for _, pragma := range []string{
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := readDB.Exec(pragma); err != nil {
			writeDB.Close()
			readDB.Close()
			return nil, fmt.Errorf("index.Open: read %s: %w", pragma, err)
		}
	}

	db := &DB{write: writeDB, read: readDB}
	if err := db.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("index.Open: migrate: %w", err)
	}
	return db, nil
}

// WriteDB 返回写连接。
// 只有已经持有 WriteTx 写锁的调用方才应该直接使用它。
func (db *DB) WriteDB() *sql.DB {
	return db.write
}

// ReadDB 返回只读连接池。
func (db *DB) ReadDB() *sql.DB {
	return db.read
}

// WriteTx 在串行事务中执行 fn。
// 用它把文档状态、FTS、chunk、vector 的写入放在同一个事务里，失败时一起回滚。
func (db *DB) WriteTx(fn func(tx *sql.Tx) error) error {
	db.writeMu.Lock()
	defer db.writeMu.Unlock()

	tx, err := db.write.Begin()
	if err != nil {
		return fmt.Errorf("index.WriteTx: begin: %w", err)
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Close 关闭读写连接。
func (db *DB) Close() error {
	var closeErr error
	db.closeOnce.Do(func() {
		var errs []error
		if err := db.read.Close(); err != nil {
			errs = append(errs, err)
		}
		if err := db.write.Close(); err != nil {
			errs = append(errs, err)
		}
		if len(errs) > 0 {
			closeErr = fmt.Errorf("index.Close: %v", errs)
		}
	})
	return closeErr
}

// migrate 创建检索核心表。
// 这里不做复杂历史迁移：当前模块是新库，只保留 collection 检索需要的最小 schema。
func (db *DB) migrate() error {
	return db.WriteTx(func(tx *sql.Tx) error {
		_, err := tx.Exec(`
-- schema_version 记录当前 schema 版本，后续需要变更表结构时可扩展。
CREATE TABLE IF NOT EXISTS schema_version (
	version INTEGER NOT NULL
);

-- entries 是文档级 FTS5 表，用于 Search 的 BM25 检索。
-- id 是文档 ID；content 是全文；tags 当前记录 collection；article_path 是相对路径。
CREATE VIRTUAL TABLE IF NOT EXISTS entries USING fts5(
	id,
	content,
	tags,
	article_path,
	tokenize='porter unicode61'
);

-- documents_fts 是 Search 的主全文索引。
-- filepath/title/body 分字段保存，查询时可以给标题更高 BM25 权重。
-- rowid 由 collection_documents.rowid 提供，方便 FTS 命中后直接回表读取元信息。
CREATE VIRTUAL TABLE IF NOT EXISTS documents_fts USING fts5(
	filepath,
	title,
	body,
	tokenize='porter unicode61'
);

-- vec_entries 是文档级向量表，保留给未来文档级向量检索。
CREATE TABLE IF NOT EXISTS vec_entries (
	id TEXT PRIMARY KEY,
	embedding BLOB NOT NULL,
	dimensions INTEGER NOT NULL
);

-- chunks_meta 保存 chunk 元信息和原文片段。
-- doc_id 关联文档；chunk_index 保留文档内顺序；heading 用于展示命中来源。
CREATE TABLE IF NOT EXISTS chunks_meta (
	chunk_id TEXT PRIMARY KEY,
	doc_id TEXT NOT NULL,
	chunk_index INTEGER NOT NULL,
	heading TEXT,
	content TEXT NOT NULL,
	start_offset INTEGER,
	end_offset INTEGER
);
CREATE INDEX IF NOT EXISTS idx_chunks_doc ON chunks_meta(doc_id);

-- chunks_fts 是 chunk 级 FTS5 表，用于更细粒度的问答召回。
CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
	chunk_id UNINDEXED,
	heading,
	content,
	tokenize='porter unicode61'
);

-- vec_chunks 保存 chunk 级 embedding 元数据和 BLOB 原文。
-- sqlite-vec 虚表负责近邻召回；这里的 BLOB 用于重算对外稳定的 cosine 分。
CREATE TABLE IF NOT EXISTS vec_chunks (
	chunk_id TEXT PRIMARY KEY,
	doc_id TEXT NOT NULL,
	embedding BLOB NOT NULL,
	dimensions INTEGER NOT NULL,
	model TEXT,
	embed_fingerprint TEXT
);
CREATE INDEX IF NOT EXISTS idx_vec_chunks_doc ON vec_chunks(doc_id);
`)
		if err != nil {
			return err
		}
		// 旧库可能已经有 vec_chunks，但缺少模型和指纹字段；这里做轻量迁移。
		if err := ensureColumn(tx, "vec_chunks", "model", "TEXT"); err != nil {
			return err
		}
		return ensureColumn(tx, "vec_chunks", "embed_fingerprint", "TEXT")
	})
}

// ensureColumn 为旧库补齐新增列。
// table/column/definition 只由内部 migration 传入，不接收用户输入。
func ensureColumn(tx *sql.Tx, table string, column string, definition string) error {
	rows, err := tx.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if name == column {
			return rows.Err()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = tx.Exec("ALTER TABLE " + table + " ADD COLUMN " + column + " " + definition)
	return err
}
