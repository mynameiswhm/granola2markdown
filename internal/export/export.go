package export

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/mynameiswhm/granola2markdown/internal/extract"
	"github.com/mynameiswhm/granola2markdown/internal/model"
)

const (
	FieldDocumentID    = "granola_document_id"
	FieldUpdatedAt     = "granola_updated_at"
	FieldDate          = "date"
	FieldStrategy      = "granola_strategy"
	FieldContentSource = "granola_content_source"
)

type ExactKey struct {
	DocumentID string
	UpdatedAt  string
}

type ExistingIndex struct {
	ByExact    map[ExactKey]string
	ByDocument map[string]model.ExistingRecord
}

func ExportCandidates(candidates []model.NoteCandidate, outputDir string, verbose bool) (model.ExportCounts, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return model.ExportCounts{}, err
	}

	index, err := ScanExisting(outputDir)
	if err != nil {
		return model.ExportCounts{}, err
	}

	reservedNames, err := existingNames(outputDir)
	if err != nil {
		return model.ExportCounts{}, err
	}

	counts := model.ExportCounts{}
	for _, candidate := range candidates {
		extracted := extract.ExtractContent(candidate.Panel, candidate.Document.Title)
		heading := extracted.FirstHeading
		if strings.TrimSpace(heading) == "" {
			heading = "untitled"
		}

		granolaUpdatedAt := ComputeGranolaUpdatedAt(
			candidate.Document.CreatedAt,
			candidate.Panel.ContentUpdatedAt,
			candidate.Panel.CreatedAt,
		)
		metadata := map[string]string{
			FieldDocumentID: candidate.Document.ID,
			FieldUpdatedAt:  granolaUpdatedAt,
			FieldDate:       MarkdownDateProperty(candidate.Document.CreatedAt),
		}
		if strings.TrimSpace(candidate.Strategy) != "" {
			metadata[FieldStrategy] = candidate.Strategy
		}
		if strings.TrimSpace(extracted.Source) != "" {
			metadata[FieldContentSource] = extracted.Source
		}

		existingRecord, hasExistingRecord := index.ByDocument[metadata[FieldDocumentID]]
		hasExistingPath := hasExistingRecord && fileExists(existingRecord.Path)
		renameEligible := hasExistingPath && shouldRenameExistingPath(existingRecord.Path, candidate.Document.CreatedAt, heading)

		if hasExistingPath && existingRecord.ContentSource == "markdown" && !renameEligible {
			counts.Skipped++
			if verbose {
				fmt.Printf("skip: %s (existing markdown note is locked)\n", candidate.Document.ID)
			}
			continue
		}

		upgradeToMarkdown := hasExistingPath &&
			existingRecord.ContentSource != "markdown" &&
			extracted.Source == "markdown"

		exactKey := ExactKey{DocumentID: metadata[FieldDocumentID], UpdatedAt: metadata[FieldUpdatedAt]}
		if !upgradeToMarkdown && !renameEligible {
			if existingPath, ok := index.ByExact[exactKey]; ok && fileExists(existingPath) {
				counts.Skipped++
				if verbose {
					fmt.Printf("skip: %s (already up to date)\n", candidate.Document.ID)
				}
				continue
			}
		}

		targetPath := ""
		action := "exported"
		if hasExistingPath {
			targetPath = existingRecord.Path
			action = "updated"
			if renamedPath := renameTargetPath(outputDir, existingRecord.Path, candidate.Document.CreatedAt, heading, reservedNames); renamedPath != "" {
				targetPath = renamedPath
			}
		} else {
			baseName := GenerateFilename(candidate.Document.CreatedAt, heading)
			targetPath = UniquePath(outputDir, baseName, reservedNames)
		}

		if err := WriteMarkdown(targetPath, metadata, extracted.Markdown); err != nil {
			counts.Errors++
			if verbose {
				fmt.Printf("error: %s: %v\n", candidate.Document.ID, err)
			}
			continue
		}

		if action == "updated" && targetPath != existingRecord.Path {
			if err := os.Remove(existingRecord.Path); err != nil && !errorsIsNotExist(err) {
				counts.Errors++
				if verbose {
					fmt.Printf("error: %s: %v\n", candidate.Document.ID, err)
				}
				continue
			}
			delete(reservedNames, filepath.Base(existingRecord.Path))
		}

		reservedNames[filepath.Base(targetPath)] = struct{}{}
		index.ByExact[exactKey] = targetPath
		index.ByDocument[metadata[FieldDocumentID]] = model.ExistingRecord{
			Path:             targetPath,
			GranolaUpdatedAt: metadata[FieldUpdatedAt],
			ContentSource:    metadata[FieldContentSource],
		}

		if action == "updated" {
			counts.Updated++
		} else {
			counts.Exported++
		}

		if verbose {
			fmt.Printf("%s: %s (strategy=%s, source=%s, doc=%s)\n", action, filepath.Base(targetPath), candidate.Strategy, extracted.Source, candidate.Document.ID)
		}
	}

	return counts, nil
}

func GenerateFilename(createdAt string, heading string) string {
	datePart := "unknown-date"
	if len(createdAt) >= 10 {
		datePart = createdAt[:10]
	}
	slug := Slugify(heading)
	if slug == "" {
		slug = "untitled"
	}
	return fmt.Sprintf("%s-%s.md", datePart, slug)
}

func renameTargetPath(outputDir string, currentPath string, createdAt string, heading string, reservedNames map[string]struct{}) string {
	if !shouldRenameExistingPath(currentPath, createdAt, heading) {
		return ""
	}

	baseName := GenerateFilename(createdAt, heading)
	currentBase := filepath.Base(currentPath)
	adjustedReserved := make(map[string]struct{}, len(reservedNames))
	for name := range reservedNames {
		if name == currentBase {
			continue
		}
		adjustedReserved[name] = struct{}{}
	}
	targetPath := UniquePath(outputDir, baseName, adjustedReserved)
	if targetPath == currentPath {
		return ""
	}
	return targetPath
}

func shouldRenameExistingPath(currentPath string, createdAt string, heading string) bool {
	return shouldRenameUnknownDatePath(currentPath, createdAt) || shouldRenamePlaceholderTitlePath(currentPath, heading)
}

func shouldRenameUnknownDatePath(currentPath string, createdAt string) bool {
	if len(strings.TrimSpace(createdAt)) < 10 {
		return false
	}
	base := filepath.Base(currentPath)
	if !strings.HasPrefix(base, "unknown-date-") {
		return false
	}
	return strings.HasPrefix(GenerateFilename(createdAt, "placeholder"), createdAt[:10]+"-")
}

func shouldRenamePlaceholderTitlePath(currentPath string, heading string) bool {
	if Slugify(heading) == "untitled" {
		return false
	}

	base := filepath.Base(currentPath)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	currentSlug, ok := generatedFilenameSlug(stem)
	if !ok {
		return false
	}

	return isPlaceholderUntitledSlug(currentSlug)
}

func generatedFilenameSlug(stem string) (string, bool) {
	switch {
	case strings.HasPrefix(stem, "unknown-date-"):
		return strings.TrimPrefix(stem, "unknown-date-"), true
	case len(stem) > 11 && stem[4] == '-' && stem[7] == '-' && stem[10] == '-':
		return stem[11:], true
	default:
		return "", false
	}
}

func isPlaceholderUntitledSlug(slug string) bool {
	if slug == "untitled" {
		return true
	}
	if !strings.HasPrefix(slug, "untitled-") {
		return false
	}

	suffix := strings.TrimPrefix(slug, "untitled-")
	if suffix == "" {
		return false
	}

	_, err := strconv.Atoi(suffix)
	return err == nil
}

func Slugify(text string) string {
	value := strings.ToLower(strings.TrimSpace(text))
	if value == "" {
		return ""
	}

	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case r == ' ' || r == '-' || r == '_':
			if !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
		default:
			if !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}

	result := strings.Trim(b.String(), "-")
	result = strings.TrimSpace(result)
	return result
}

func UniquePath(outputDir string, baseName string, reservedNames map[string]struct{}) string {
	base := filepath.Base(baseName)
	ext := filepath.Ext(base)
	if ext == "" {
		ext = ".md"
	}
	stem := strings.TrimSuffix(base, filepath.Ext(base))

	candidateName := stem + ext
	if _, exists := reservedNames[candidateName]; !exists {
		candidatePath := filepath.Join(outputDir, candidateName)
		if !fileExists(candidatePath) {
			return candidatePath
		}
	}

	index := 2
	for {
		candidateName = fmt.Sprintf("%s-%d%s", stem, index, ext)
		if _, exists := reservedNames[candidateName]; exists {
			index++
			continue
		}
		candidatePath := filepath.Join(outputDir, candidateName)
		if !fileExists(candidatePath) {
			return candidatePath
		}
		index++
	}
}

func WriteMarkdown(path string, metadata map[string]string, body string) error {
	frontMatter := SerializeFrontMatter(metadata)
	textBody := strings.TrimSpace(body)
	payload := frontMatter + "\n"
	if textBody != "" {
		payload = frontMatter + textBody + "\n"
	}
	return os.WriteFile(path, []byte(payload), 0o644)
}

func SerializeFrontMatter(metadata map[string]string) string {
	lines := []string{"---"}
	for _, key := range []string{FieldDocumentID, FieldUpdatedAt, FieldDate} {
		if value, ok := metadata[key]; ok {
			lines = append(lines, fmt.Sprintf("%s: %q", key, value))
		}
	}

	var remaining []string
	for key := range metadata {
		if key == FieldDocumentID || key == FieldUpdatedAt || key == FieldDate {
			continue
		}
		remaining = append(remaining, key)
	}
	sort.Strings(remaining)
	for _, key := range remaining {
		lines = append(lines, fmt.Sprintf("%s: %q", key, metadata[key]))
	}

	lines = append(lines, "---", "")
	return strings.Join(lines, "\n")
}

func ParseFrontMatter(text string) map[string]string {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return map[string]string{}
	}

	result := map[string]string{}
	foundClosing := false
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			foundClosing = true
			break
		}
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		key := strings.TrimSpace(parts[0])
		if key == "" {
			continue
		}
		value := parseScalar(strings.TrimSpace(parts[1]))
		result[key] = value
	}

	if !foundClosing {
		return map[string]string{}
	}
	return result
}

func ScanExisting(outputDir string) (ExistingIndex, error) {
	index := ExistingIndex{
		ByExact:    map[ExactKey]string{},
		ByDocument: map[string]model.ExistingRecord{},
	}

	entries, err := os.ReadDir(outputDir)
	if err != nil {
		if errorsIsNotExist(err) {
			return index, nil
		}
		return index, err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}

		path := filepath.Join(outputDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		metadata := ParseFrontMatter(string(data))
		if len(metadata) == 0 {
			continue
		}

		documentID := metadata[FieldDocumentID]
		if strings.TrimSpace(documentID) == "" {
			continue
		}

		updatedAt := metadata[FieldUpdatedAt]
		if strings.TrimSpace(updatedAt) == "" {
			updatedAt = ComputeGranolaUpdatedAt(
				metadata["granola_document_created_at"],
				metadata["granola_panel_content_updated_at"],
				metadata["granola_panel_created_at"],
			)
		}
		index.ByDocument[documentID] = model.ExistingRecord{
			Path:             path,
			GranolaUpdatedAt: updatedAt,
			ContentSource:    metadata[FieldContentSource],
		}
		if strings.TrimSpace(updatedAt) == "" {
			continue
		}

		exactKey := ExactKey{DocumentID: documentID, UpdatedAt: updatedAt}
		index.ByExact[exactKey] = path
	}

	return index, nil
}

func ComputeGranolaUpdatedAt(timestamps ...string) string {
	type parsedTimestamp struct {
		value string
		time  time.Time
	}

	parsed := make([]parsedTimestamp, 0, len(timestamps))
	fallback := make([]string, 0, len(timestamps))

	for _, raw := range timestamps {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		fallback = append(fallback, value)
		if parsedTime, ok := ParseTimestamp(value); ok {
			parsed = append(parsed, parsedTimestamp{value: value, time: parsedTime})
		}
	}

	if len(parsed) > 0 {
		max := parsed[0]
		for _, item := range parsed[1:] {
			if item.time.After(max.time) {
				max = item
			}
		}
		return max.value
	}

	if len(fallback) > 0 {
		return fallback[0]
	}

	return ""
}

func MarkdownDateProperty(rawTimestamp string) string {
	value := strings.TrimSpace(rawTimestamp)
	if value == "" {
		return "[[unknown-date]]"
	}

	if parsed, ok := ParseTimestamp(value); ok {
		return fmt.Sprintf("[[%s]]", parsed.Format("2006-01-02"))
	}

	// Keep supporting timestamps that include extra suffixes by validating the date prefix.
	if len(value) >= 10 {
		if parsed, err := time.Parse("2006-01-02", value[:10]); err == nil {
			return fmt.Sprintf("[[%s]]", parsed.Format("2006-01-02"))
		}
	}

	return "[[unknown-date]]"
}

func ParseTimestamp(value string) (time.Time, bool) {
	candidate := strings.TrimSpace(value)
	if candidate == "" {
		return time.Time{}, false
	}

	layouts := []struct {
		layout string
		withTZ bool
	}{
		{time.RFC3339Nano, true},
		{time.RFC3339, true},
		{"2006-01-02T15:04:05.999999999", false},
		{"2006-01-02T15:04:05", false},
		{"2006-01-02", false},
	}

	for _, layout := range layouts {
		parsed, err := time.Parse(layout.layout, candidate)
		if err != nil {
			continue
		}
		if layout.withTZ {
			return parsed.UTC(), true
		}
		return time.Date(parsed.Year(), parsed.Month(), parsed.Day(), parsed.Hour(), parsed.Minute(), parsed.Second(), parsed.Nanosecond(), time.UTC), true
	}

	return time.Time{}, false
}

func existingNames(outputDir string) (map[string]struct{}, error) {
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		if errorsIsNotExist(err) {
			return map[string]struct{}{}, nil
		}
		return nil, err
	}

	result := map[string]struct{}{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		result[entry.Name()] = struct{}{}
	}
	return result, nil
}

func parseScalar(value string) string {
	if value == "" {
		return ""
	}

	if strings.HasPrefix(value, "\"") || strings.HasPrefix(value, "'") {
		if strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
			return strings.Trim(value, "'")
		}
		if unquoted, err := strconv.Unquote(value); err == nil {
			return unquoted
		}
	}
	if value == "null" {
		return ""
	}
	return value
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func errorsIsNotExist(err error) bool {
	return err != nil && os.IsNotExist(err)
}
