package qmd

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type indexedEmbedDoc struct {
	id         string
	collection string
	relPath    string
	title      string
}

type indexedEmbedChunk struct {
	id      string
	index   int
	content string
}

// EmbedCollection 为已建立文档/chunk 索引的 collection 生成向量。
// 调用方应先执行 UpdateCollection；Force 为 false 时会跳过已有当前 embedding 指纹的文档。
func (s *Store) EmbedCollection(ctx context.Context, collection string, opts EmbedOptions) (*EmbedResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.embedder == nil {
		return nil, fmt.Errorf("embedding provider is not configured")
	}
	if strings.TrimSpace(collection) != "" {
		if _, err := s.getCollection(collection); err != nil {
			return nil, err
		}
	}

	docs, err := s.pendingEmbedDocs(collection)
	if err != nil {
		return nil, err
	}
	result := &EmbedResult{Scanned: len(docs)}
	for i, doc := range docs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		chunks, err := s.indexedChunks(doc.id)
		if err != nil {
			return nil, err
		}
		fresh, err := s.docHasFreshVectors(doc.id, len(chunks))
		if err != nil {
			return nil, err
		}
		if fresh && !opts.Force {
			result.Skipped++
			if opts.Progress != nil {
				opts.Progress(EmbedProgress{Current: i + 1, Total: len(docs), Embedded: result.Embedded, CurrentPath: doc.collection + "/" + doc.relPath})
			}
			continue
		}
		if opts.Force {
			if err := s.vecStore.DeleteDocChunkVectors(doc.id); err != nil {
				return nil, err
			}
		}
		embedded, err := s.embedIndexedChunks(doc, chunks)
		if err != nil {
			return nil, err
		}
		result.Embedded += embedded
		if opts.Progress != nil {
			opts.Progress(EmbedProgress{Current: i + 1, Total: len(docs), Embedded: result.Embedded, CurrentPath: doc.collection + "/" + doc.relPath})
		}
	}
	return result, nil
}

func (s *Store) pendingEmbedDocs(collection string) ([]indexedEmbedDoc, error) {
	query := `
SELECT id, collection, rel_path, title
FROM collection_documents
WHERE active=1
`
	var args []any
	if strings.TrimSpace(collection) != "" {
		query += " AND collection=?"
		args = append(args, collection)
	}
	query += " ORDER BY collection, rel_path"

	rows, err := s.db.ReadDB().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []indexedEmbedDoc
	for rows.Next() {
		var doc indexedEmbedDoc
		if err := rows.Scan(&doc.id, &doc.collection, &doc.relPath, &doc.title); err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
	return docs, rows.Err()
}

func (s *Store) indexedChunks(docID string) ([]indexedEmbedChunk, error) {
	rows, err := s.db.ReadDB().Query(`
SELECT chunk_id, chunk_index, content
FROM chunks_meta
WHERE doc_id=?
ORDER BY chunk_index
`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chunks []indexedEmbedChunk
	for rows.Next() {
		var chunk indexedEmbedChunk
		if err := rows.Scan(&chunk.id, &chunk.index, &chunk.content); err != nil {
			return nil, err
		}
		chunks = append(chunks, chunk)
	}
	return chunks, rows.Err()
}

func (s *Store) docHasFreshVectors(docID string, chunkCount int) (bool, error) {
	if chunkCount == 0 {
		return true, nil
	}
	var count int
	err := s.db.ReadDB().QueryRow(
		"SELECT COUNT(*) FROM vec_chunks WHERE doc_id=? AND embed_fingerprint=?",
		docID, s.embeddingFingerprint(),
	).Scan(&count)
	return count == chunkCount, err
}

func (s *Store) embedIndexedChunks(doc indexedEmbedDoc, chunks []indexedEmbedChunk) (int, error) {
	type chunkVector struct {
		chunkID string
		vector  []float32
	}
	var vectorsToWrite []chunkVector
	for _, ch := range chunks {
		text := strings.TrimSpace(ch.content)
		if text == "" {
			continue
		}
		vec, err := s.embedder.Embed(s.formatDocumentEmbeddingInput(doc.title, text))
		if err != nil {
			return 0, err
		}
		vectorsToWrite = append(vectorsToWrite, chunkVector{chunkID: ch.id, vector: vec})
	}
	err := s.db.WriteTx(func(tx *sql.Tx) error {
		model := s.embeddingModelName()
		fingerprint := s.embeddingFingerprint()
		for _, item := range vectorsToWrite {
			if err := s.vecStore.UpsertChunk(tx, item.chunkID, doc.id, item.vector, model, fingerprint); err != nil {
				return err
			}
		}
		return nil
	})
	return len(vectorsToWrite), err
}
