package model

type DocumentData struct {
	ID        string
	CreatedAt string
	DeletedAt string
}

type PanelData struct {
	ID               string
	DocumentID       string
	Title            string
	TemplateSlug     string
	Content          map[string]any
	OriginalContent  string
	GeneratedLines   []map[string]any
	CreatedAt        string
	ContentUpdatedAt string
	DeletedAt        string
}

type NoteCandidate struct {
	Document DocumentData
	Panel    PanelData
}

type ExtractedContent struct {
	Markdown     string
	FirstHeading string
	Source       string
}

type ExportCounts struct {
	Exported int
	Updated  int
	Skipped  int
	Errors   int
}

type ExistingRecord struct {
	Path             string
	GranolaUpdatedAt string
}
