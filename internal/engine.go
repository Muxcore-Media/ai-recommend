package internal

import (
	"sort"
	"strings"
)

// LibraryItem is a title already in the household library.
type LibraryItem struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Year      int      `json:"year,omitempty"`
	MediaType string   `json:"media_type,omitempty"`
	Genres    []string `json:"genres,omitempty"`
	Franchise string   `json:"franchise,omitempty"`
	TMDBID    int      `json:"tmdb_id,omitempty"`
	Seasons   int      `json:"seasons,omitempty"`
}

// Suggestion is a title the household does not have yet.
type Suggestion struct {
	Title     string   `json:"title"`
	Year      int      `json:"year,omitempty"`
	MediaType string   `json:"media_type"`
	Genres    []string `json:"genres,omitempty"`
	Reason    string   `json:"reason"`
	Score     float64  `json:"score"`
	Franchise string   `json:"franchise,omitempty"`
	Kind      string   `json:"kind"` // related, sequel, taste, complete_collection
}

// CatalogEntry is a known related title used for offline suggestions.
type CatalogEntry struct {
	Title     string
	Year      int
	MediaType string
	Genres    []string
	Franchise string
	After     string // title that typically precedes this one
}

var defaultCatalog = []CatalogEntry{
	{Title: "The Dark Knight", Year: 2008, MediaType: "movie", Genres: []string{"action", "crime"}, Franchise: "batman", After: "batman begins"},
	{Title: "The Dark Knight Rises", Year: 2012, MediaType: "movie", Genres: []string{"action"}, Franchise: "batman", After: "the dark knight"},
	{Title: "Batman Begins", Year: 2005, MediaType: "movie", Genres: []string{"action"}, Franchise: "batman"},
	{Title: "Mad Max: Fury Road", Year: 2015, MediaType: "movie", Genres: []string{"action"}, Franchise: "mad max"},
	{Title: "Furiosa: A Mad Max Saga", Year: 2024, MediaType: "movie", Genres: []string{"action"}, Franchise: "mad max", After: "mad max: fury road"},
	{Title: "Arrival", Year: 2016, MediaType: "movie", Genres: []string{"scifi", "drama"}},
	{Title: "Interstellar", Year: 2014, MediaType: "movie", Genres: []string{"scifi", "drama"}},
	{Title: "The Martian", Year: 2015, MediaType: "movie", Genres: []string{"scifi"}},
	{Title: "Dune", Year: 2021, MediaType: "movie", Genres: []string{"scifi"}, Franchise: "dune"},
	{Title: "Dune: Part Two", Year: 2024, MediaType: "movie", Genres: []string{"scifi"}, Franchise: "dune", After: "dune"},
	{Title: "Andor", Year: 2022, MediaType: "tv", Genres: []string{"scifi"}, Franchise: "star wars"},
	{Title: "Rogue One", Year: 2016, MediaType: "movie", Genres: []string{"scifi"}, Franchise: "star wars"},
	{Title: "The Expanse", Year: 2015, MediaType: "tv", Genres: []string{"scifi"}},
	{Title: "Better Call Saul", Year: 2015, MediaType: "tv", Genres: []string{"crime", "drama"}, Franchise: "breaking bad", After: "breaking bad"},
	{Title: "El Camino", Year: 2019, MediaType: "movie", Genres: []string{"crime"}, Franchise: "breaking bad", After: "breaking bad"},
}

func suggestAdditions(library []LibraryItem, catalog []CatalogEntry) []Suggestion {
	if catalog == nil {
		catalog = defaultCatalog
	}
	have := map[string]LibraryItem{}
	genreCount := map[string]int{}
	for _, it := range library {
		have[normTitle(it.Title)] = it
		for _, g := range it.Genres {
			genreCount[strings.ToLower(g)]++
		}
		if it.Franchise != "" {
			genreCount["franchise:"+strings.ToLower(it.Franchise)]++
		}
	}
	var out []Suggestion
	for _, c := range catalog {
		if _, ok := have[normTitle(c.Title)]; ok {
			continue
		}
		score := 0.15
		kind := "taste"
		reason := "matches household taste"
		if c.Franchise != "" {
			for _, it := range library {
				if strings.EqualFold(it.Franchise, c.Franchise) || strings.Contains(normTitle(it.Title), strings.ToLower(c.Franchise)) {
					score = 0.9
					kind = "complete_collection"
					reason = "completes the " + c.Franchise + " collection"
					break
				}
			}
		}
		if c.After != "" {
			if _, ok := have[normTitle(c.After)]; ok {
				score = 0.95
				kind = "sequel"
				reason = "sequel or follow-up to " + c.After
			}
		}
		if score < 0.5 {
			for _, g := range c.Genres {
				if genreCount[strings.ToLower(g)] > 0 {
					score = 0.55 + 0.05*float64(genreCount[strings.ToLower(g)])
					kind = "related"
					reason = "shares " + g + " with the current library"
					break
				}
			}
		}
		if score < 0.5 && len(library) == 0 {
			score = 0.2
			reason = "starter catalog pick"
		}
		if score >= 0.5 || len(library) == 0 {
			out = append(out, Suggestion{
				Title: c.Title, Year: c.Year, MediaType: c.MediaType,
				Genres: c.Genres, Reason: reason, Score: score,
				Franchise: c.Franchise, Kind: kind,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

func tasteClusters(library []LibraryItem) []string {
	counts := map[string]int{}
	for _, it := range library {
		for _, g := range it.Genres {
			counts[strings.ToLower(g)]++
		}
	}
	type pair struct {
		g string
		n int
	}
	var pairs []pair
	for g, n := range counts {
		pairs = append(pairs, pair{g, n})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].n > pairs[j].n })
	out := make([]string, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, p.g)
	}
	return out
}

func normTitle(s string) string {
	return strings.TrimSpace(strings.ToLower(s))
}
