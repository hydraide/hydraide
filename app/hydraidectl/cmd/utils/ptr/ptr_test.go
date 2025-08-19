package ptr

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTo(t *testing.T) {
	t.Run("string pointer", func(t *testing.T) {
		str := "hello"
		ptr := To(str)
		assert.NotNil(t, ptr)
		assert.Equal(t, str, *ptr)
	})

	t.Run("int pointer", func(t *testing.T) {
		num := 42
		ptr := To(num)
		assert.NotNil(t, ptr)
		assert.Equal(t, num, *ptr)
	})

	t.Run("bool pointer", func(t *testing.T) {
		val := true
		ptr := To(val)
		assert.NotNil(t, ptr)
		assert.Equal(t, val, *ptr)
	})

	t.Run("struct pointer", func(t *testing.T) {
		type TestStruct struct {
			Name string
			Age  int
		}
		s := TestStruct{Name: "test", Age: 30}
		ptr := To(s)
		assert.NotNil(t, ptr)
		assert.Equal(t, s, *ptr)
	})
}

func TestFromPtr(t *testing.T) {
	t.Run("nil string pointer", func(t *testing.T) {
		var ptr *string
		val := FromPtr(ptr)
		assert.Equal(t, "", val)
	})

	t.Run("valid string pointer", func(t *testing.T) {
		str := "hello"
		ptr := &str
		val := FromPtr(ptr)
		assert.Equal(t, str, val)
	})

	t.Run("nil int pointer", func(t *testing.T) {
		var ptr *int
		val := FromPtr(ptr)
		assert.Equal(t, 0, val)
	})

	t.Run("valid int pointer", func(t *testing.T) {
		num := 42
		ptr := &num
		val := FromPtr(ptr)
		assert.Equal(t, num, val)
	})
}

func TestEqual(t *testing.T) {
	t.Run("both nil", func(t *testing.T) {
		var p1, p2 *string
		assert.True(t, Equal(p1, p2))
	})

	t.Run("one nil", func(t *testing.T) {
		p1 := To("hello")
		var p2 *string
		assert.False(t, Equal(p1, p2))
		assert.False(t, Equal(p2, p1))
	})

	t.Run("equal values", func(t *testing.T) {
		p1 := To("hello")
		p2 := To("hello")
		assert.True(t, Equal(p1, p2))
	})

	t.Run("different values", func(t *testing.T) {
		p1 := To("hello")
		p2 := To("world")
		assert.False(t, Equal(p1, p2))
	})

	t.Run("same pointer", func(t *testing.T) {
		str := "hello"
		p1 := &str
		p2 := p1
		assert.True(t, Equal(p1, p2))
	})
}
