package extract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractOnlySupportsMarkdown(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "guide.md")
	txt := filepath.Join(dir, "guide.txt")
	if err := os.WriteFile(md, []byte("---\ntitle: Guide\n---\n\n# Guide\n\nmarkdown body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(txt, []byte("plain text body\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	content, err := Extract(md, "auto")
	if err != nil {
		t.Fatalf("Extract markdown: %v", err)
	}
	wantText := "---\ntitle: Guide\n---\n\n# Guide\n\nmarkdown body\n"
	if content.Type != "article" || content.Text != wantText {
		t.Fatalf("markdown content = %#v, want raw markdown text", content)
	}

	if _, err := Extract(txt, "auto"); err == nil || !strings.Contains(err.Error(), "unsupported source extension") {
		t.Fatalf("Extract txt error = %v, want unsupported source extension", err)
	}
}

func TestChunkTextWithOverlapUsesSourcePositionLikeQMD(t *testing.T) {
	text := strings.Join([]string{
		"alpha alpha alpha alpha alpha alpha alpha alpha",
		"boundary context before marker",
		"beta beta beta beta beta beta beta beta",
	}, "\n\n")

	chunks := ChunkTextWithOverlap(text, 4, 0.5)
	if len(chunks) < 2 {
		t.Fatalf("len(chunks) = %d, want at least 2", len(chunks))
	}
	firstEnd := strings.Index(text, chunks[0].Text) + len(chunks[0].Text)
	secondStart := strings.Index(text, chunks[1].Text)
	if secondStart < 0 || firstEnd < 0 || secondStart >= firstEnd {
		t.Fatalf("chunks = %#v, want second chunk to start inside first chunk source span", chunks)
	}
}

func TestChunkTextUsesListAndCodeBlockBoundaries(t *testing.T) {
	text := strings.Join([]string{
		"intro intro intro intro intro intro",
		"- first operational step",
		"- second operational step",
		"```",
		"dig example.com",
		"```",
		"closing closing closing closing closing closing",
	}, "\n")

	chunks := ChunkText(text, 6)
	if len(chunks) < 3 {
		t.Fatalf("len(chunks) = %d, want chunks split on list/code boundaries", len(chunks))
	}
	for _, chunk := range chunks {
		if strings.Contains(chunk.Text, "```") && strings.Count(chunk.Text, "```") != 2 {
			t.Fatalf("chunk split inside code fence: %#v", chunks)
		}
	}
}

func TestChunkTextWithHeadingsSkipsCodeFenceInternalBreakpoints(t *testing.T) {
	text := strings.Join([]string{
		"# Runbook",
		"intro intro intro intro intro intro",
		"```",
		"dig example.com with several resolver flags and source binding options",
		"dig example.net with several resolver flags and source binding options",
		"```",
		"closing closing closing closing closing closing",
	}, "\n")

	chunks := ChunkText(text, 6)
	if len(chunks) < 3 {
		t.Fatalf("len(chunks) = %d, want heading section split into semantic chunks", len(chunks))
	}
	for i, chunk := range chunks {
		if strings.Contains(chunk.Text, "dig example.com") {
			if i == 0 || strings.Contains(chunks[i-1].Text, "```") {
				t.Fatalf("chunks = %#v, want qmd cutoff search to skip code-fence-internal breakpoints", chunks)
			}
			return
		}
	}
	t.Fatalf("chunks = %#v, want code block content present", chunks)
}
