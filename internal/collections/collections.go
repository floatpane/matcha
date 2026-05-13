package collections

// Unique returns values with duplicates removed, preserving first-seen order.
func Unique[S ~[]E, E comparable](values S) S {
	seen := make(map[E]struct{}, len(values))
	unique := make(S, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}

// UniqueNonEmpty returns values with zero values and duplicates removed.
func UniqueNonEmpty[S ~[]E, E comparable](values S) S {
	var zero E
	nonEmpty := make(S, 0, len(values))
	for _, value := range values {
		if value == zero {
			continue
		}
		nonEmpty = append(nonEmpty, value)
	}
	return Unique(nonEmpty)
}
