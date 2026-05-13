package collections

import (
	"reflect"
	"testing"
)

func TestUniqueRemovesDuplicates(t *testing.T) {
	t.Run("With strings", func(t *testing.T) {
		got := Unique([]string{"acct-1", "acct-2", "acct-1", "", "acct-3", ""})
		want := []string{"acct-1", "acct-2", "", "acct-3"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Unique() = %#v, want %#v", got, want)
		}
	})

	t.Run("With ints", func(t *testing.T) {
		got := Unique([]int{1, 2, 1, 0, 3, 0})
		want := []int{1, 2, 0, 3}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Unique() = %#v, want %#v", got, want)
		}
	})
}

func TestUniqueNonEmptyRemovesZeroValuesAndDuplicates(t *testing.T) {
	t.Run("With strings", func(t *testing.T) {
		got := UniqueNonEmpty([]string{"", "acct-1", "acct-2", "acct-1", "", "acct-3"})
		want := []string{"acct-1", "acct-2", "acct-3"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("UniqueNonEmpty() = %#v, want %#v", got, want)
		}
	})

	t.Run("With ints", func(t *testing.T) {
		got := UniqueNonEmpty([]int{0, 1, 2, 1, 0, 3})
		want := []int{1, 2, 3}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("UniqueNonEmpty() = %#v, want %#v", got, want)
		}
	})
}
