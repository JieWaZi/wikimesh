package qmd

import (
	"database/sql"

	"github.com/JieWaZi/wikimesh/pkg/qmd/internal/embed"
	"github.com/JieWaZi/wikimesh/pkg/qmd/internal/index"
)

const defaultChunkSize = 900
const defaultChunkOverlap = 0.15
const defaultVSearchMinScore = 0.3
const defaultCollectionPattern = "**/*.md"
const rrfK = 60.0

const embeddingFingerprintProbeQuery = "__qmd_embedding_query_probe__"
const embeddingFingerprintProbeTitle = "__qmd_embedding_title_probe__"
const embeddingFingerprintProbeDoc = "__qmd_embedding_document_probe__"

// Store 是 collection 文档库的主入口。
// 它负责协调 collection 元数据、文档状态表、FTS 表、chunk 表和向量表。
type Store struct {
	// db 是底层 SQLite 连接管理器，内部使用单写多读模式。
	db *index.DB

	// memStore 是文档级 FTS5 索引封装。
	memStore *index.DocumentStore

	// chunkStore 是 chunk 级 FTS5 索引封装。
	chunkStore *index.ChunkStore

	// vecStore 是向量存储和 cosine 检索封装。
	vecStore *index.VectorStore

	// 这是可选的向量模型实例；为空时 search 仍可用，vsearch 不可用。
	embedder Embedder

	// queryExpander 是可选的查询扩展器；只影响 VSearch/Query 的语义检索召回。
	queryExpander QueryExpander

	// queryReranker 是可选的 query 重排器；为空时 Query 使用 qmd 的 no-rerank RRF 路径。
	queryReranker QueryReranker

	// chunkSize 是每个 chunk 的最大近似 token 数。
	chunkSize int

	// chunkOverlap 是相邻 chunk 的重叠比例，默认 0.15。
	chunkOverlap float64
}

// NewStore 打开数据库并初始化 collection 需要的表。
func NewStore(cfg Config) (*Store, error) {
	dbPath := cfg.DBPath
	if dbPath == "" {
		dbPath = "wikimesh.db"
	}
	db, err := index.Open(dbPath)
	if err != nil {
		return nil, err
	}

	s := &Store{
		db:         db,
		memStore:   index.NewDocumentStore(db),
		chunkStore: index.NewChunkStore(db),
		vecStore:   index.NewVectorStore(db),
		chunkSize:  cfg.ChunkSize,
	}
	s.queryExpander = cfg.QueryExpander
	s.queryReranker = cfg.QueryReranker
	if s.chunkSize <= 0 {
		s.chunkSize = defaultChunkSize
	}
	s.chunkOverlap = cfg.ChunkOverlap
	if s.chunkOverlap <= 0 {
		s.chunkOverlap = defaultChunkOverlap
	}
	if cfg.Embedder != nil {
		s.embedder = cfg.Embedder
	} else {
		s.embedder = embed.NewFromConfig(cfg.Embedding.internalConfig())
	}
	if err := s.ensureSchema(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close 关闭数据库连接。
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// ensureSchema 创建 collection 层自己的业务表。
// FTS、chunk 和向量表由底层 storage migration 创建；这里补充 collection 元数据和文档状态。
func (s *Store) ensureSchema() error {
	return s.db.WriteTx(func(tx *sql.Tx) error {
		_, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS collections (
	name TEXT PRIMARY KEY,
	path TEXT NOT NULL,
	pattern TEXT NOT NULL DEFAULT '',
	include_globs TEXT,
	ignore_globs TEXT,
	update_command TEXT NOT NULL DEFAULT '',
	include_by_default INTEGER NOT NULL DEFAULT 1,
	context_json TEXT NOT NULL DEFAULT '{}',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS collection_config (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS collection_documents (
	id TEXT PRIMARY KEY,
	collection TEXT NOT NULL REFERENCES collections(name) ON DELETE CASCADE,
	rel_path TEXT NOT NULL,
	abs_path TEXT NOT NULL,
	title TEXT,
	hash TEXT NOT NULL,
	embedding_model TEXT NOT NULL DEFAULT '',
	mtime TEXT,
	size_bytes INTEGER NOT NULL DEFAULT 0,
	active INTEGER NOT NULL DEFAULT 1,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	UNIQUE(collection, rel_path)
);

CREATE INDEX IF NOT EXISTS idx_collection_documents_collection
	ON collection_documents(collection, active);
`)
		if err != nil {
			return err
		}
		for _, col := range []struct {
			name string
			def  string
		}{
			{name: "pattern", def: "''"},
			{name: "update_command", def: "''"},
			{name: "context_json", def: "'{}'"},
		} {
			if err := ensureTextColumn(tx, "collections", col.name, col.def); err != nil {
				return err
			}
		}
		if err := ensureIntegerColumn(tx, "collections", "include_by_default", "1"); err != nil {
			return err
		}
		// 旧测试库或已有库可能已经创建过 collection_documents。
		// 这里补齐模型指纹列，确保切换 embedding 模型后能自动重建向量。
		return ensureTextColumn(tx, "collection_documents", "embedding_model", "''")
	})
}
