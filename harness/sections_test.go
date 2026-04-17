package harness

import "testing"

func TestParseSections(t *testing.T) {
	doc := `# Title

Intro text.

## Overview

This is the overview.

## Backend structure

Backend details here.
More backend.

## Data model

Data model info.
`
	sections := ParseSections(doc)
	if len(sections) != 3 {
		t.Fatalf("expected 3 sections, got %d", len(sections))
	}

	tests := []struct {
		name    string
		content string
	}{
		{"Overview", "This is the overview."},
		{"Backend structure", "Backend details here.\nMore backend."},
		{"Data model", "Data model info."},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if sections[i].Name != tt.name {
				t.Errorf("section %d name = %q, want %q", i, sections[i].Name, tt.name)
			}
			if sections[i].Content != tt.content {
				t.Errorf("section %d content = %q, want %q", i, sections[i].Content, tt.content)
			}
		})
	}
}

func TestFindSection(t *testing.T) {
	sections := []Section{
		{Name: "Overview", Content: "overview text"},
		{Name: "Backend structure", Content: "backend text"},
	}

	content, ok := FindSection(sections, "overview")
	if !ok {
		t.Fatal("expected to find section 'overview'")
	}
	if content != "overview text" {
		t.Errorf("content = %q, want %q", content, "overview text")
	}

	_, ok = FindSection(sections, "nonexistent")
	if ok {
		t.Fatal("expected not to find section 'nonexistent'")
	}
}

func TestExtractSummary(t *testing.T) {
	t.Run("with summary section", func(t *testing.T) {
		doc := "# Title\n\n## Summary\n\nShort summary.\n\n## Details\n\nLong details."
		s := ExtractSummary(doc)
		if s != "Short summary." {
			t.Errorf("summary = %q, want %q", s, "Short summary.")
		}
	})

	t.Run("with overview section", func(t *testing.T) {
		doc := "# Title\n\n## Overview\n\nOverview text.\n\n## Details\n\nLong details."
		s := ExtractSummary(doc)
		if s != "Overview text." {
			t.Errorf("summary = %q, want %q", s, "Overview text.")
		}
	})

	t.Run("fallback to first 30 lines", func(t *testing.T) {
		doc := "# Title\n\nSome intro text."
		s := ExtractSummary(doc)
		if s != "# Title\n\nSome intro text." {
			t.Errorf("summary = %q, want %q", s, "# Title\n\nSome intro text.")
		}
	})
}

func TestSectionNames(t *testing.T) {
	sections := []Section{
		{Name: "A"},
		{Name: "B"},
		{Name: "C"},
	}
	names := SectionNames(sections)
	if len(names) != 3 || names[0] != "A" || names[1] != "B" || names[2] != "C" {
		t.Errorf("names = %v, want [A B C]", names)
	}
}
