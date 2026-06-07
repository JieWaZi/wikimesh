package index

import (
	"database/sql"
	"testing"
)

func TestChunkVectorsUseSQLiteVecTable(t *testing.T) {
	db, err := Open(t.TempDir() + "/vec.db")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	store := NewVectorStore(db)
	if err := db.WriteTx(func(tx *sql.Tx) error {
		if err := store.UpsertChunk(tx, "doc:c0", "doc", []float32{1, 0}, "fake", "fp-a"); err != nil {
			return err
		}
		return store.UpsertChunk(tx, "doc:c1", "doc", []float32{0, 1}, "fake", "fp-a")
	}); err != nil {
		t.Fatalf("UpsertChunk: %v", err)
	}

	var vecRows int
	if err := db.ReadDB().QueryRow("SELECT COUNT(*) FROM vec_chunks_vec_2").Scan(&vecRows); err != nil {
		t.Fatalf("vec0 count query: %v", err)
	}
	if vecRows != 2 {
		t.Fatalf("vec0 rows = %d, want 2", vecRows)
	}

	hits, err := store.SearchChunksByFingerprint([]float32{1, 0}, "fp-a", 2)
	if err != nil {
		t.Fatalf("SearchChunksByFingerprint: %v", err)
	}
	if len(hits) == 0 || hits[0].ChunkID != "doc:c0" {
		t.Fatalf("hits = %#v, want doc:c0 first", hits)
	}
}

func TestDeleteDocChunkVectorsRemovesSQLiteVecRows(t *testing.T) {
	db, err := Open(t.TempDir() + "/vec.db")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	store := NewVectorStore(db)
	if err := db.WriteTx(func(tx *sql.Tx) error {
		if err := store.UpsertChunk(tx, "doc:c0", "doc", []float32{1, 0}, "fake", "fp-a"); err != nil {
			return err
		}
		if err := store.UpsertChunk(tx, "doc:c1", "doc", []float32{0, 1}, "fake", "fp-a"); err != nil {
			return err
		}
		return store.UpsertChunk(tx, "other:c0", "other", []float32{1, 1}, "fake", "fp-a")
	}); err != nil {
		t.Fatalf("UpsertChunk: %v", err)
	}

	if err := store.DeleteDocChunkVectors("doc"); err != nil {
		t.Fatalf("DeleteDocChunkVectors: %v", err)
	}

	var metaRows int
	if err := db.ReadDB().QueryRow("SELECT COUNT(*) FROM vec_chunks WHERE doc_id = 'doc'").Scan(&metaRows); err != nil {
		t.Fatalf("vec_chunks count query: %v", err)
	}
	if metaRows != 0 {
		t.Fatalf("vec_chunks rows for doc = %d, want 0", metaRows)
	}

	var vecRows int
	if err := db.ReadDB().QueryRow("SELECT COUNT(*) FROM vec_chunks_vec_2").Scan(&vecRows); err != nil {
		t.Fatalf("vec0 count query: %v", err)
	}
	if vecRows != 1 {
		t.Fatalf("vec0 rows = %d, want only other doc row", vecRows)
	}

	hits, err := store.SearchChunksByFingerprint([]float32{1, 0}, "fp-a", 10)
	if err != nil {
		t.Fatalf("SearchChunksByFingerprint: %v", err)
	}
	for _, hit := range hits {
		if hit.DocID == "doc" {
			t.Fatalf("deleted doc still returned in hits: %#v", hits)
		}
	}
}

func TestUpsertChunkRemovesPreviousSQLiteVecDimensionRows(t *testing.T) {
	db, err := Open(t.TempDir() + "/vec.db")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	store := NewVectorStore(db)
	if err := db.WriteTx(func(tx *sql.Tx) error {
		return store.UpsertChunk(tx, "doc:c0", "doc", []float32{1, 0}, "fake-2d", "fp-2d")
	}); err != nil {
		t.Fatalf("first UpsertChunk: %v", err)
	}
	if err := db.WriteTx(func(tx *sql.Tx) error {
		return store.UpsertChunk(tx, "doc:c0", "doc", []float32{1, 0, 0}, "fake-3d", "fp-3d")
	}); err != nil {
		t.Fatalf("second UpsertChunk: %v", err)
	}

	var oldRows int
	if err := db.ReadDB().QueryRow("SELECT COUNT(*) FROM vec_chunks_vec_2").Scan(&oldRows); err != nil {
		t.Fatalf("old vec0 count query: %v", err)
	}
	if oldRows != 0 {
		t.Fatalf("old vec0 rows = %d, want 0 after dimension change", oldRows)
	}

	var newRows int
	if err := db.ReadDB().QueryRow("SELECT COUNT(*) FROM vec_chunks_vec_3").Scan(&newRows); err != nil {
		t.Fatalf("new vec0 count query: %v", err)
	}
	if newRows != 1 {
		t.Fatalf("new vec0 rows = %d, want 1", newRows)
	}
}
