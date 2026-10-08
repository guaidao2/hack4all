package core

// View is the flat, JSON-friendly shape of a technique.
//
// The non-interactive CLI (--json) and the web API both serve exactly this
// structure, so an AI agent or a script that learns one has learned the other.
// Body carries only the requested language; title and summary stay bilingual
// because they are cheap and make results self-describing across languages.
type View struct {
	ID         string   `json:"id"`
	Title      LangText `json:"title"`
	Summary    LangText `json:"summary"`
	Body       string   `json:"body"`
	BodyLang   string   `json:"body_lang"`
	Category   []string `json:"category"`
	Tags       []string `json:"tags"`
	Tools      []string `json:"tools"`
	ATTACK     []string `json:"attck"`
	Platform   []string `json:"platform"`
	Difficulty string   `json:"difficulty"`
	Updated    string   `json:"updated"`
	Path       string   `json:"path"`
	Score      int      `json:"score,omitempty"`
}

// SearchResponse is the envelope around a set of matches.
type SearchResponse struct {
	Query   string `json:"query"`
	Lang    string `json:"lang"`
	Count   int    `json:"count"`
	Matches []View `json:"matches"`
}

// View converts a technique into its transfer shape for the given language.
func (t *Technique) View(lang string, score int) View {
	body, bodyLang := t.Body.Get(lang)
	return View{
		ID:         t.ID,
		Title:      t.Title,
		Summary:    t.Summary,
		Body:       body,
		BodyLang:   bodyLang,
		Category:   t.Category,
		Tags:       t.Tags,
		Tools:      t.Tools,
		ATTACK:     t.ATTACK,
		Platform:   t.Platform,
		Difficulty: t.Difficulty,
		Updated:    t.Updated,
		Path:       t.Path,
		Score:      score,
	}
}

// SearchResponse runs a search and renders it in the shared transfer shape.
func (l *Library) SearchResponse(query, lang string, limit int) SearchResponse {
	lang = NormalizeLang(lang)
	matches := l.Search(query, limit)
	views := make([]View, 0, len(matches))
	for _, m := range matches {
		views = append(views, m.Technique.View(lang, m.Score))
	}
	return SearchResponse{
		Query:   query,
		Lang:    lang,
		Count:   len(views),
		Matches: views,
	}
}

// ViewAll renders the whole library (list mode) in the transfer shape.
func (l *Library) ViewAll(lang string) SearchResponse {
	return l.SearchResponse("", lang, 0)
}
