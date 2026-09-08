package internal

import "testing"

func TestSuggestsSequelWhenPrequelPresent(t *testing.T) {
	got := suggestAdditions([]LibraryItem{
		{ID: "1", Title: "Dune", Year: 2021, Genres: []string{"scifi"}, Franchise: "dune"},
	}, nil)
	if len(got) == 0 || got[0].Title != "Dune: Part Two" || got[0].Kind != "sequel" {
		t.Fatalf("got %#v", got)
	}
}

func TestSuggestsRelatedGenre(t *testing.T) {
	got := suggestAdditions([]LibraryItem{
		{ID: "1", Title: "Arrival", Year: 2016, Genres: []string{"scifi", "drama"}},
	}, nil)
	found := false
	for _, s := range got {
		if s.Title == "Interstellar" && s.Kind == "related" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected Interstellar related, got %#v", got)
	}
}

func TestTasteClustersOrderByFrequency(t *testing.T) {
	got := tasteClusters([]LibraryItem{
		{Genres: []string{"scifi"}},
		{Genres: []string{"scifi", "drama"}},
		{Genres: []string{"drama"}},
		{Genres: []string{"scifi"}},
	})
	if len(got) < 2 || got[0] != "scifi" {
		t.Fatalf("clusters = %#v", got)
	}
}
