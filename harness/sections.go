package harness

import "strings"

type Section struct {
	Name    string
	Content string
}

func ParseSections(content string) []Section {
	var sections []Section
	var current Section
	var buf strings.Builder

	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "## ") {
			if current.Name != "" {
				current.Content = strings.TrimSpace(buf.String())
				sections = append(sections, current)
				buf.Reset()
			}
			current = Section{Name: strings.TrimPrefix(line, "## ")}
			continue
		}
		if current.Name != "" {
			buf.WriteString(line)
			buf.WriteString("\n")
		}
	}
	if current.Name != "" {
		current.Content = strings.TrimSpace(buf.String())
		sections = append(sections, current)
	}

	return sections
}

func SectionNames(sections []Section) []string {
	names := make([]string, len(sections))
	for i, s := range sections {
		names[i] = s.Name
	}
	return names
}

func FindSection(sections []Section, name string) (string, bool) {
	target := strings.ToLower(name)
	for _, s := range sections {
		if strings.ToLower(s.Name) == target {
			return s.Content, true
		}
	}
	return "", false
}

func ExtractSummary(content string) string {
	sections := ParseSections(content)
	if s, ok := FindSection(sections, "Summary"); ok {
		return s
	}
	if s, ok := FindSection(sections, "Overview"); ok {
		return s
	}
	lines := strings.Split(content, "\n")
	if len(lines) > 30 {
		lines = lines[:30]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
