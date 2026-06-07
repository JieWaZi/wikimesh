package index

import "testing"

func TestConstructorsAreAvailable(t *testing.T) {
	db, err := Open(t.TempDir() + "/index.db")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	if NewDocumentStore(db) == nil {
		t.Fatal("NewDocumentStore returned nil")
	}
	if NewChunkStore(db) == nil {
		t.Fatal("NewChunkStore returned nil")
	}
	if NewVectorStore(db) == nil {
		t.Fatal("NewVectorStore returned nil")
	}
}
