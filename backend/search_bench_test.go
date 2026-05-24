package backend

import "testing"

func BenchmarkParseSearchQuery_Simple(b *testing.B) {
	const q = "invoice"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ParseSearchQuery(q)
	}
}

func BenchmarkParseSearchQuery_Complex(b *testing.B) {
	const q = `from:alice@example.com to:bob@example.com subject:"Q4 invoice" body:overdue since:2026-01-01 before:2026-05-01 larger:1024 misc terms`
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ParseSearchQuery(q)
	}
}

func BenchmarkTokenizeSearchQuery(b *testing.B) {
	const q = `from:alice "long quoted phrase here" subject:foo body:bar`
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = tokenizeSearchQuery(q)
	}
}
