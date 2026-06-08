package qmd

import "context"

// Status 返回索引总览、集合统计和待向量化文档数量。
func (s *Store) Status(ctx context.Context) (StatusResult, error) {
	if err := ctx.Err(); err != nil {
		return StatusResult{}, err
	}
	collections, err := s.ListCollections(ctx)
	if err != nil {
		return StatusResult{}, err
	}
	var totalDocuments int
	if err := s.db.ReadDB().QueryRow(`SELECT COUNT(*) FROM collection_documents WHERE active != 0`).Scan(&totalDocuments); err != nil {
		return StatusResult{}, err
	}
	var vectorCount int
	if err := s.db.ReadDB().QueryRow(`SELECT COUNT(*) FROM vec_chunks`).Scan(&vectorCount); err != nil {
		return StatusResult{}, err
	}
	fingerprint := s.embeddingFingerprint()
	needsEmbedding := 0
	rows, err := s.db.ReadDB().Query(`SELECT id FROM collection_documents WHERE active != 0`)
	if err != nil {
		return StatusResult{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var docID string
		if err := rows.Scan(&docID); err != nil {
			return StatusResult{}, err
		}
		var chunks int
		if err := s.db.ReadDB().QueryRow(`SELECT COUNT(*) FROM chunks_meta WHERE doc_id=?`, docID).Scan(&chunks); err != nil {
			return StatusResult{}, err
		}
		var fresh int
		if fingerprint == "" {
			if err := s.db.ReadDB().QueryRow(`SELECT COUNT(*) FROM vec_chunks WHERE doc_id=?`, docID).Scan(&fresh); err != nil {
				return StatusResult{}, err
			}
		} else {
			if err := s.db.ReadDB().QueryRow(`SELECT COUNT(*) FROM vec_chunks WHERE doc_id=? AND embed_fingerprint=?`, docID, fingerprint).Scan(&fresh); err != nil {
				return StatusResult{}, err
			}
		}
		if chunks > 0 && fresh < chunks {
			needsEmbedding++
		}
	}
	if err := rows.Err(); err != nil {
		return StatusResult{}, err
	}
	return StatusResult{
		DBPath:         s.db.Path(),
		TotalDocuments: totalDocuments,
		VectorCount:    vectorCount,
		NeedsEmbedding: needsEmbedding,
		Collections:    collections,
	}, nil
}
