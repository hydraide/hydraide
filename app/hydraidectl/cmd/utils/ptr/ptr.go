// Package ptr provides utility functions for creating pointers to values.
// This is particularly useful in tests and when working with APIs that require pointer arguments.
package ptr

// To returns a pointer to the given value.
// This generic function works with any type and is useful when you need to pass
// a pointer to a literal value or convert a value to a pointer.
//
// Example:
//
//	strPtr := ptr.To("hello")
//	intPtr := ptr.To(42)
//	boolPtr := ptr.To(true)
func To[T any](v T) *T {
	return &v
}

// FromPtr safely dereferences a pointer, returning the zero value if the pointer is nil.
// This is useful when you want to safely access the value of a pointer that might be nil.
//
// Example:
//
//	var strPtr *string
//	str := ptr.FromPtr(strPtr) // Returns "" (zero value for string)
//
//	validPtr := ptr.To("hello")
//	str2 := ptr.FromPtr(validPtr) // Returns "hello"
func FromPtr[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

// Equal checks if two pointers point to equal values.
// Returns true if both are nil, or both are non-nil and point to equal values.
//
// Example:
//
//	p1 := ptr.To("hello")
//	p2 := ptr.To("hello")
//	p3 := ptr.To("world")
//	var p4 *string
//
//	ptr.Equal(p1, p2) // true
//	ptr.Equal(p1, p3) // false
//	ptr.Equal(p4, p4) // true (both nil)
//	ptr.Equal(p1, p4) // false
func Equal[T comparable](a, b *T) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
