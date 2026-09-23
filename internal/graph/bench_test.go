package graph

import (
	"os"
	"testing"

	"github.com/tilman-schieber/tlog/internal/store"
)

// The graph is rebuilt after every block edit, so its cost is the thing that
// decides whether an index is needed at all.
func BenchmarkBuildRealCorpus(b *testing.B) {
	dir := os.Getenv("REAL_DIR")
	if dir == "" {
		b.Skip("no REAL_DIR")
	}
	s, err := store.Open(dir)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Build(s); err != nil {
			b.Fatal(err)
		}
	}
}
