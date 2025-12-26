package domain

type SearchResult struct {
	ID       int64
	Title    string
	URL      string
	Content  string
	Distance float64
}
