package qmd

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/JieWaZi/wikimesh/pkg/qmd/internal/extract"
	"github.com/JieWaZi/wikimesh/pkg/qmd/internal/index"
)

// UpdateCollection 扫描 collection 并刷新文档级 FTS 与 chunk 索引。
// 它会先执行 collection 的 Update 命令，再按 include/ignore 规则扫描文件；向量生成由 EmbedCollection 单独负责。
func (s *Store) UpdateCollection(ctx context.Context, name string, opts UpdateOptions) (*UpdateResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c, err := s.getCollection(name)
	if err != nil {
		return nil, err
	}
	if err := runCollectionUpdateCommand(ctx, c); err != nil {
		return nil, err
	}
	files, err := scanCollection(c)
	if err != nil {
		return nil, err
	}

	result := &UpdateResult{Scanned: len(files)}
	seen := make(map[string]bool, len(files))
	for i, file := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		seen[file.relPath] = true
		indexed, embedded, err := s.indexFile(c, file, opts)
		if err != nil {
			return nil, err
		}
		if indexed {
			result.Indexed++
			result.Embedded += embedded
		} else {
			result.Skipped++
		}
		if opts.Progress != nil {
			opts.Progress(UpdateProgress{Current: i + 1, Total: len(files), CurrentPath: file.relPath})
		}
	}

	removed, err := s.markMissingInactive(c.Name, seen)
	if err != nil {
		return nil, err
	}
	result.Removed = removed
	return result, nil
}

func runCollectionUpdateCommand(ctx context.Context, c Collection) error {
	command := strings.TrimSpace(c.Update)
	if command == "" {
		return nil
	}
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = c.Path
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("collection update command failed for %s: %w: %s", c.Name, err, strings.TrimSpace(string(output)))
	}
	return nil
}

type collectionFile struct {
	relPath string
	absPath string
	info    fs.FileInfo
}

type docMeta struct {
	id         string
	collection string
	relPath    string
	absPath    string
	title      string
	hash       string
	// embeddingModel 是上次写入向量时使用的模型指纹。
	// 文档内容不变但模型变化时，需要靠它触发向量重建。
	embeddingModel string
}

func (s *Store) indexFile(c Collection, file collectionFile, opts UpdateOptions) (bool, int, error) {
	data, err := os.ReadFile(file.absPath)
	if err != nil {
		return false, 0, err
	}
	hash := contentHash(data)
	docID := documentID(c.Name, file.relPath)
	existing, err := s.getDocByCollectionPath(c.Name, file.relPath)
	if err != nil {
		return false, 0, err
	}
	if existing != nil && existing.hash == hash && !opts.RebuildVectors {
		return false, 0, nil
	}

	content, err := extract.Extract(file.absPath, "auto")
	if err != nil {
		return false, 0, err
	}
	title := extractTitle(content.Text, file.relPath)
	// chunk 使用少量重叠上下文，避免答案刚好落在切片边界时向量召回断裂。
	chunks := extract.ChunkTextWithOverlap(content.Text, s.chunkSize, s.chunkOverlap)
	now := time.Now().UTC().Format(time.RFC3339)
	embedded := 0

	// 先删除旧索引再插入新索引，避免 FTS5 和向量表里留下过期 chunk。
	if err := s.db.WriteTx(func(tx *sql.Tx) error {
		if err := s.chunkStore.DeleteDocChunks(tx, docID); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM entries WHERE id=?`, docID); err != nil {
			return err
		}
		if _, err := tx.Exec(`
INSERT INTO collection_documents
	(id, collection, rel_path, abs_path, title, hash, embedding_model, mtime, size_bytes, active, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)
ON CONFLICT(collection, rel_path) DO UPDATE SET
	id=excluded.id,
	abs_path=excluded.abs_path,
	title=excluded.title,
	hash=excluded.hash,
	embedding_model=excluded.embedding_model,
	mtime=excluded.mtime,
	size_bytes=excluded.size_bytes,
	active=1,
	updated_at=excluded.updated_at
`, docID, c.Name, file.relPath, file.absPath, title, hash, s.embeddingFingerprint(), file.info.ModTime().UTC().Format(time.RFC3339), file.info.Size(), now, now); err != nil {
			return err
		}
		rowID, err := collectionDocumentRowID(tx, c.Name, file.relPath)
		if err != nil {
			return err
		}
		// 写入 Search 专用 FTS：filepath/title/body 分列参与 BM25，标题字段权重更高。
		if err := s.indexSearchDocument(tx, searchFTSRecord{
			RowID:    rowID,
			FilePath: searchFilePath(c.Name, file.relPath),
			Title:    title,
			Body:     content.Text,
		}); err != nil {
			return err
		}
		if _, err := tx.Exec(
			`INSERT INTO entries (id, content, tags, article_path) VALUES (?, ?, ?, ?)`,
			docID, content.Text, c.Name, file.relPath,
		); err != nil {
			return err
		}

		chunkEntries := make([]index.ChunkEntry, 0, len(chunks))
		for _, ch := range chunks {
			if strings.TrimSpace(ch.Text) == "" {
				continue
			}
			chunkEntries = append(chunkEntries, index.ChunkEntry{
				ChunkID:     fmt.Sprintf("%s:c%d", docID, ch.Index),
				ChunkIndex:  ch.Index,
				Heading:     ch.Heading,
				Content:     ch.Text,
				StartOffset: ch.StartOffset,
				EndOffset:   ch.EndOffset,
			})
		}
		return s.chunkStore.IndexChunks(tx, docID, chunkEntries)
	}); err != nil {
		return false, 0, err
	}

	return true, embedded, nil
}

func (s *Store) embedDocumentChunks(docID string, title string, chunks []extract.Chunk) (int, error) {
	type chunkVector struct {
		chunkID string
		vector  []float32
	}
	var vectorsToWrite []chunkVector
	for _, ch := range chunks {
		text := strings.TrimSpace(ch.Text)
		if text == "" {
			continue
		}
		// 向量计算可能访问外部 API 或本地模型，不能放在 SQLite 写事务里，
		// 否则一次慢请求会长时间占用全局写锁。
		vec, err := s.embedder.Embed(s.formatDocumentEmbeddingInput(title, text))
		if err != nil {
			return 0, err
		}
		vectorsToWrite = append(vectorsToWrite, chunkVector{
			chunkID: fmt.Sprintf("%s:c%d", docID, ch.Index),
			vector:  vec,
		})
	}
	err := s.db.WriteTx(func(tx *sql.Tx) error {
		model := s.embeddingModelName()
		fingerprint := s.embeddingFingerprint()
		for _, item := range vectorsToWrite {
			if err := s.vecStore.UpsertChunk(tx, item.chunkID, docID, item.vector, model, fingerprint); err != nil {
				return err
			}
		}
		return nil
	})
	return len(vectorsToWrite), err
}

func (s *Store) markMissingInactive(collection string, seen map[string]bool) (int, error) {
	rows, err := s.db.ReadDB().Query(`SELECT id, rel_path FROM collection_documents WHERE collection=? AND active=1`, collection)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var missing []docMeta
	for rows.Next() {
		var m docMeta
		if err := rows.Scan(&m.id, &m.relPath); err != nil {
			return 0, err
		}
		if !seen[m.relPath] {
			missing = append(missing, m)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	for _, m := range missing {
		if err := s.db.WriteTx(func(tx *sql.Tx) error {
			if err := s.chunkStore.DeleteDocChunks(tx, m.id); err != nil {
				return err
			}
			if _, err := tx.Exec(`DELETE FROM entries WHERE id=?`, m.id); err != nil {
				return err
			}
			if _, err := tx.Exec(`DELETE FROM documents_fts WHERE rowid IN (SELECT rowid FROM collection_documents WHERE id=?)`, m.id); err != nil {
				return err
			}
			_, err := tx.Exec(`UPDATE collection_documents SET active=0, updated_at=? WHERE id=?`, time.Now().UTC().Format(time.RFC3339), m.id)
			return err
		}); err != nil {
			return 0, err
		}
	}
	return len(missing), nil
}
