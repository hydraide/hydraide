package treasure

import (
	"testing"
	"time"

	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure/guard"
	"github.com/stretchr/testify/assert"
)

func MySaveMethod(_ Treasure, _ guard.ID) TreasureStatus {
	return StatusNew
}

func TestGetContentType(t *testing.T) {

	t.Run("should return ContentTypeVoid when treasure content is ContentTypeVoid", func(t *testing.T) {
		treasureInterface := New(MySaveMethod)
		guardID := treasureInterface.StartTreasureGuard(true)
		treasureInterface.SetContentVoid(guardID)
		assert.Equal(t, treasureInterface.GetContentType(), ContentTypeVoid)

		treasureInterface.ResetContentVoid(guardID)
		treasureInterface.SetContentString(guardID, "test")
		assert.Equal(t, treasureInterface.GetContentType(), ContentTypeString)

		treasureInterface.ReleaseTreasureGuard(guardID)
	})

	t.Run("should handle Uint32Slice", func(t *testing.T) {
		treasureInterface := New(MySaveMethod)

		guardID := treasureInterface.StartTreasureGuard(true)
		testUintSlice := []uint32{12, 13, 14}

		// próbálunk törölni a sliceból, ameddig az nem is létetik
		err := treasureInterface.Uint32SliceDelete(testUintSlice)
		assert.Nil(t, err, "Uint32SliceDelete should not return error")

		// próbáljuk lékérdezni a slicet, ameddig az nem is létetik
		_, err = treasureInterface.Uint32SliceGetAll()
		assert.NotNil(t, err, "Uint32SliceGetAll should return error")

		// próbáljuk a méretét lekérdezni, ameddig nem is létezik
		_, err = treasureInterface.Uint32SliceSize()
		assert.NotNil(t, err, "Uint32SliceSize should return error")

		isContentChanged := treasureInterface.IsContentChanged()
		assert.False(t, isContentChanged, "IsContentChanged should return false")

		err = treasureInterface.Uint32SlicePush(testUintSlice)
		assert.Nil(t, err, "Uint32SlicePush should not return error")

		isContentChanged = treasureInterface.IsContentChanged()
		assert.True(t, isContentChanged, "IsContentChanged should return true")

		// push content again
		err = treasureInterface.Uint32SlicePush(testUintSlice)
		assert.Nil(t, err, "Uint32SlicePush should not return error")

		assert.Equal(t, treasureInterface.GetContentType(), ContentTypeUint32Slice)

		uintSLiceFromTreasure, err := treasureInterface.Uint32SliceGetAll()
		assert.Equal(t, nil, err, "Uint32SliceGetAll should not return error")
		assert.Equal(t, testUintSlice, uintSLiceFromTreasure, "Uint32SliceGetAll should return the same slice")

		// beszúrunk 3 új számot
		testUintSlice2 := []uint32{12, 13, 17, 19, 35}
		err = treasureInterface.Uint32SlicePush(testUintSlice2)
		assert.Nil(t, err, "Uint32SlicePush should not return error")

		uintSLiceFromTreasure, err = treasureInterface.Uint32SliceGetAll()
		assert.Equal(t, nil, err, "Uint32SliceGetAll should not return error")
		// összesen 5 szám kell legyen a slice-ban
		expectedNumbers := []uint32{12, 13, 14, 17, 19, 35}
		assert.Equal(t, expectedNumbers, uintSLiceFromTreasure, "Uint32SliceGetAll should return the same slice")

		isContentChanged = treasureInterface.IsContentChanged()
		assert.True(t, isContentChanged, "IsContentChanged should return true")

		// törlünk 2 számot és egy nem létező számot is
		testUintSlice3 := []uint32{13, 19, 99}
		err = treasureInterface.Uint32SliceDelete(testUintSlice3)
		assert.Nil(t, err, "Uint32SliceDelete should not return error")

		uintSLiceFromTreasure, err = treasureInterface.Uint32SliceGetAll()
		assert.Equal(t, nil, err, "Uint32SliceGetAll should not return error")
		expectedNumbers = []uint32{12, 14, 17, 35}
		assert.Equal(t, expectedNumbers, uintSLiceFromTreasure, "Uint32SliceGetAll should return the same slice")

		// ellenőrizzük a slice méretét
		size, err := treasureInterface.Uint32SliceSize()
		assert.Nil(t, err, "Uint32SliceSize should not return error")
		assert.Equal(t, 4, size, "Uint32SliceSize should return 4")

		// töröljük az összes fennmaradó számot
		numbersForDelete := []uint32{12, 14, 17, 35}
		err = treasureInterface.Uint32SliceDelete(numbersForDelete)
		assert.Nil(t, err, "Uint32SliceDelete should not return error")

		isContentChanged = treasureInterface.IsContentChanged()
		assert.True(t, isContentChanged, "IsContentChanged should return true")

		// ellenőrizzük a slice méretét
		size, err = treasureInterface.Uint32SliceSize()
		assert.Nil(t, err, "Uint32SliceSize should not return error")
		assert.Equal(t, 0, size, "Uint32SliceSize should return 0")

		treasureInterface.ResetContentUint32Slice(guardID)
		assert.Equal(t, treasureInterface.GetContentType(), ContentTypeVoid)

		treasureInterface.ReleaseTreasureGuard(guardID)
	})

	t.Run("should return uint8 when treasure content is uint8", func(t *testing.T) {
		treasureInterface := New(MySaveMethod)
		guardID := treasureInterface.StartTreasureGuard(true)
		testInt := uint8(12)
		treasureInterface.SetContentUint8(guardID, testInt)
		assert.Equal(t, treasureInterface.GetContentType(), ContentTypeUint8)
		getInt, err := treasureInterface.GetContentUint8()
		assert.Equal(t, err, nil)
		assert.Equal(t, getInt, testInt)

		treasureInterface.ResetContentUint8(guardID)
		assert.Equal(t, treasureInterface.GetContentType(), ContentTypeVoid)

		treasureInterface.ReleaseTreasureGuard(guardID)
	})

	t.Run("should return uint16 when treasure content is uint16", func(t *testing.T) {
		treasureInterface := New(MySaveMethod)
		guardID := treasureInterface.StartTreasureGuard(true)
		testInt := uint16(12)
		treasureInterface.SetContentUint16(guardID, testInt)
		assert.Equal(t, treasureInterface.GetContentType(), ContentTypeUint16)
		getInt, err := treasureInterface.GetContentUint16()
		assert.Equal(t, err, nil)
		assert.Equal(t, getInt, testInt)

		treasureInterface.ResetContentUint16(guardID)
		assert.Equal(t, treasureInterface.GetContentType(), ContentTypeVoid)

		treasureInterface.ReleaseTreasureGuard(guardID)
	})

	t.Run("should return uint32 when treasure content is uint32", func(t *testing.T) {
		treasureInterface := New(MySaveMethod)
		guardID := treasureInterface.StartTreasureGuard(true)
		testInt := uint32(12)
		treasureInterface.SetContentUint32(guardID, testInt)
		assert.Equal(t, treasureInterface.GetContentType(), ContentTypeUint32)
		getInt, err := treasureInterface.GetContentUint32()
		assert.Equal(t, err, nil)
		assert.Equal(t, getInt, testInt)

		treasureInterface.ResetContentUint32(guardID)
		assert.Equal(t, treasureInterface.GetContentType(), ContentTypeVoid)

		treasureInterface.ReleaseTreasureGuard(guardID)
	})

	t.Run("should return uint64 when treasure content is uint64", func(t *testing.T) {
		treasureInterface := New(MySaveMethod)
		guardID := treasureInterface.StartTreasureGuard(true)
		testInt := uint64(12)
		treasureInterface.SetContentUint64(guardID, testInt)
		assert.Equal(t, treasureInterface.GetContentType(), ContentTypeUint64)
		getInt, err := treasureInterface.GetContentUint64()
		assert.Equal(t, err, nil)
		assert.Equal(t, getInt, testInt)

		treasureInterface.ResetContentUint64(guardID)
		assert.Equal(t, treasureInterface.GetContentType(), ContentTypeVoid)

		treasureInterface.ReleaseTreasureGuard(guardID)
	})

	t.Run("should return int8 when treasure content is int8", func(t *testing.T) {
		treasureInterface := New(MySaveMethod)
		guardID := treasureInterface.StartTreasureGuard(true)
		testInt := int8(12)
		treasureInterface.SetContentInt8(guardID, testInt)
		assert.Equal(t, treasureInterface.GetContentType(), ContentTypeInt8)
		getInt, err := treasureInterface.GetContentInt8()
		assert.Equal(t, err, nil)
		assert.Equal(t, getInt, testInt)

		treasureInterface.ResetContentInt8(guardID)
		assert.Equal(t, treasureInterface.GetContentType(), ContentTypeVoid)

		treasureInterface.ReleaseTreasureGuard(guardID)
	})

	t.Run("should return int32 when treasure content is int32", func(t *testing.T) {
		treasureInterface := New(MySaveMethod)
		guardID := treasureInterface.StartTreasureGuard(true)
		testInt := int32(12)
		treasureInterface.SetContentInt32(guardID, testInt)
		assert.Equal(t, treasureInterface.GetContentType(), ContentTypeInt32)
		getInt, err := treasureInterface.GetContentInt32()
		assert.Equal(t, err, nil)
		assert.Equal(t, getInt, testInt)

		treasureInterface.ResetContentInt32(guardID)
		assert.Equal(t, treasureInterface.GetContentType(), ContentTypeVoid)

		treasureInterface.ReleaseTreasureGuard(guardID)
	})

	t.Run("should return int64 when treasure content is int64", func(t *testing.T) {
		treasureInterface := New(MySaveMethod)
		guardID := treasureInterface.StartTreasureGuard(true)
		testInt := int64(12)
		treasureInterface.SetContentInt64(guardID, testInt)
		assert.Equal(t, treasureInterface.GetContentType(), ContentTypeInt64)
		getInt, err := treasureInterface.GetContentInt64()
		assert.Equal(t, err, nil)
		assert.Equal(t, getInt, testInt)

		treasureInterface.ResetContentInt64(guardID)
		assert.Equal(t, treasureInterface.GetContentType(), ContentTypeVoid)

		treasureInterface.ReleaseTreasureGuard(guardID)
	})

	t.Run("should return float32 when treasure content is float32", func(t *testing.T) {
		treasureInterface := New(MySaveMethod)
		guardID := treasureInterface.StartTreasureGuard(true)
		testFloat := float32(12.0)
		treasureInterface.SetContentFloat32(guardID, testFloat)
		assert.Equal(t, treasureInterface.GetContentType(), ContentTypeFloat32)
		getFloat, err := treasureInterface.GetContentFloat32()
		assert.Equal(t, err, nil)
		assert.Equal(t, getFloat, testFloat)

		treasureInterface.ResetContentFloat32(guardID)
		assert.Equal(t, treasureInterface.GetContentType(), ContentTypeVoid)
		treasureInterface.ReleaseTreasureGuard(guardID)

	})

	t.Run("should return float64 when treasure content is float64", func(t *testing.T) {
		treasureInterface := New(MySaveMethod)
		guardID := treasureInterface.StartTreasureGuard(true)
		testFloat := 12.0
		treasureInterface.SetContentFloat64(guardID, testFloat)
		assert.Equal(t, treasureInterface.GetContentType(), ContentTypeFloat64)
		getFloat, err := treasureInterface.GetContentFloat64()
		assert.Equal(t, err, nil)
		assert.Equal(t, getFloat, testFloat)

		treasureInterface.ResetContentFloat64(guardID)
		assert.Equal(t, treasureInterface.GetContentType(), ContentTypeVoid)

		treasureInterface.ReleaseTreasureGuard(guardID)
	})

	t.Run("should return ContentTypeString when treasure content is ContentTypeString", func(t *testing.T) {
		treasureInterface := New(MySaveMethod)
		guardID := treasureInterface.StartTreasureGuard(true)
		testString := "test"
		treasureInterface.SetContentString(guardID, testString)
		assert.Equal(t, treasureInterface.GetContentType(), ContentTypeString)
		getString, err := treasureInterface.GetContentString()
		assert.Equal(t, err, nil)
		assert.Equal(t, getString, testString)

		treasureInterface.ResetContentString(guardID)
		assert.Equal(t, treasureInterface.GetContentType(), ContentTypeVoid)

		treasureInterface.ReleaseTreasureGuard(guardID)
	})

	t.Run("should return ContentTypeBoolean when treasure content is ContentTypeBoolean", func(t *testing.T) {
		treasureInterface := New(MySaveMethod)
		guardID := treasureInterface.StartTreasureGuard(true)
		treasureInterface.SetContentBool(guardID, true)
		assert.Equal(t, treasureInterface.GetContentType(), ContentTypeBoolean)
		getBool, err := treasureInterface.GetContentBool()
		assert.Equal(t, err, nil)
		assert.Equal(t, getBool, true)

		treasureInterface.ResetContentBool(guardID)
		assert.Equal(t, treasureInterface.GetContentType(), ContentTypeVoid)

		treasureInterface.ReleaseTreasureGuard(guardID)
	})

	t.Run("should return ContentTypeByteArray when treasure content is ContentTypeByteArray", func(t *testing.T) {
		treasureInterface := New(MySaveMethod)
		guardID := treasureInterface.StartTreasureGuard(true)
		testBytes := []byte("test")
		treasureInterface.SetContentByteArray(guardID, testBytes)
		assert.Equal(t, treasureInterface.GetContentType(), ContentTypeByteArray)
		getBytes, err := treasureInterface.GetContentByteArray()
		assert.Equal(t, err, nil)
		assert.Equal(t, getBytes, testBytes)

		treasureInterface.ResetContentByteArray(guardID)
		assert.Equal(t, treasureInterface.GetContentType(), ContentTypeVoid)

		treasureInterface.ReleaseTreasureGuard(guardID)
	})

	t.Run("should return with correct creator and modifier data", func(t *testing.T) {

		treasureInterface := New(MySaveMethod)
		guardID := treasureInterface.StartTreasureGuard(true)
		treasureInterface.SetContentString(guardID, "test")

		treasureInterface.SetCreatedBy(guardID, "creator-user")
		treasureInterface.SetCreatedAt(guardID, time.Now())
		treasureInterface.SetModifiedBy(guardID, "modifier-user")
		treasureInterface.SetModifiedAt(guardID, time.Now())

		assert.Equal(t, treasureInterface.GetModifiedBy(), "modifier-user")
		assert.Equal(t, treasureInterface.GetCreatedBy(), "creator-user")
		assert.LessOrEqual(t, treasureInterface.GetCreatedAt(), time.Now().UnixNano())
		assert.LessOrEqual(t, treasureInterface.GetModifiedAt(), time.Now().UnixNano())

		treasureInterface.ReleaseTreasureGuard(guardID)
	})

	t.Run("should set and get the expiration time of the treasure", func(t *testing.T) {
		treasureInterface := New(MySaveMethod)
		guardID := treasureInterface.StartTreasureGuard(true)
		treasureInterface.SetContentString(guardID, "test")
		treasureInterface.SetExpirationTime(guardID, time.Now().Add(time.Hour))
		assert.Greater(t, treasureInterface.GetExpirationTime(), time.Now().UnixNano(), "Expiration time should be greater than current time")
		treasureInterface.ReleaseTreasureGuard(guardID)
	})

	t.Run("should clone treasure", func(t *testing.T) {

		treasureInterface := New(MySaveMethod)
		guardID := treasureInterface.StartTreasureGuard(true, guard.BodyAuthID)

		treasureInterface.BodySetKey(guardID, "test-key")
		treasureInterface.SetContentString(guardID, "test")

		cloneTreasure := treasureInterface.Clone(guardID)
		treasureInterface.ReleaseTreasureGuard(guardID)

		assert.Equal(t, "test-key", cloneTreasure.GetKey())
		contentString, err := cloneTreasure.GetContentString()
		assert.Nil(t, err)
		assert.Equal(t, contentString, "test")

		if cloneTreasure.GetKey() != "test-key" {
			t.Error("Clone should not have file name")
		}

	})

	t.Run("should clone just the ContentTypeString content of the treasure", func(t *testing.T) {

		treasureInterface := New(MySaveMethod)
		guardID := treasureInterface.StartTreasureGuard(true, guard.BodyAuthID)

		treasureInterface.BodySetKey(guardID, "test-key")
		treasureInterface.SetContentString(guardID, "test")

		clonedContent := treasureInterface.CloneContent(guardID)
		treasureInterface.ReleaseTreasureGuard(guardID)

		expectedString := "test"
		if *clonedContent.String != expectedString {
			t.Error("Cloned content should be test")
		}

	})

	t.Run("should clone just the Integer content of the treasure", func(t *testing.T) {

		treasureInterface := New(MySaveMethod)
		guardID := treasureInterface.StartTreasureGuard(true, guard.BodyAuthID)

		treasureInterface.BodySetKey(guardID, "test-key")
		treasureInterface.SetContentInt64(guardID, 12)

		clonedContent := treasureInterface.CloneContent(guardID)
		treasureInterface.ReleaseTreasureGuard(guardID)

		expectedInt := int64(12)
		if *clonedContent.Int64 != expectedInt {
			t.Error("Cloned content should be 12")
		}

	})

	t.Run("should clone just the ContentTypeFloat64 content of the treasure", func(t *testing.T) {

		treasureInterface := New(MySaveMethod)
		guardID := treasureInterface.StartTreasureGuard(true, guard.BodyAuthID)

		treasureInterface.BodySetKey(guardID, "test-key")
		treasureInterface.SetContentFloat64(guardID, 12.0)

		clonedContent := treasureInterface.CloneContent(guardID)
		treasureInterface.ReleaseTreasureGuard(guardID)

		expectedFloat := 12.0
		if *clonedContent.Float64 != expectedFloat {
			t.Error("Cloned content should be 12.0")
		}

	})

	t.Run("should clone just the ContentTypeBoolean content of the treasure", func(t *testing.T) {

		treasureInterface := New(MySaveMethod)
		guardID := treasureInterface.StartTreasureGuard(true, guard.BodyAuthID)

		treasureInterface.BodySetKey(guardID, "test-key")
		treasureInterface.SetContentBool(guardID, true)

		clonedContent := treasureInterface.CloneContent(guardID)
		treasureInterface.ReleaseTreasureGuard(guardID)

		if *clonedContent.Boolean != true {
			t.Error("Cloned content should be true")
		}

	})

	t.Run("should clone just the ContentTypeByteArray content of the treasure", func(t *testing.T) {

		treasureInterface := New(MySaveMethod)
		guardID := treasureInterface.StartTreasureGuard(true, guard.BodyAuthID)

		testBytes := []byte("test")
		treasureInterface.BodySetKey(guardID, "test-key")
		treasureInterface.SetContentByteArray(guardID, testBytes)

		clonedContent := treasureInterface.CloneContent(guardID)
		treasureInterface.ReleaseTreasureGuard(guardID)

		if string(clonedContent.ByteArray) != string(testBytes) {
			t.Error("Cloned content should be test")
		}

	})

	t.Run("should clone just the ContentTypeVoid content of the treasure", func(t *testing.T) {

		treasureInterface := New(MySaveMethod)
		guardID := treasureInterface.StartTreasureGuard(true, guard.BodyAuthID)

		treasureInterface.BodySetKey(guardID, "test-key")
		treasureInterface.SetContentVoid(guardID)

		clonedContent := treasureInterface.CloneContent(guardID)
		treasureInterface.ReleaseTreasureGuard(guardID)

		if clonedContent.Void == false {
			t.Error("Cloned content should be ContentTypeVoid")
		}

	})

	t.Run("should set content from content interface", func(t *testing.T) {

		treasureInterface := New(MySaveMethod)
		guardID := treasureInterface.StartTreasureGuard(true)

		testStringContent := "test"
		content := Content{
			String: &testStringContent,
		}

		treasureInterface.SetContent(guardID, content)

		assert.Equal(t, treasureInterface.GetContentType(), ContentTypeString)
		contentString, err := treasureInterface.GetContentString()
		assert.Nil(t, err)
		assert.Equal(t, contentString, "test")

		treasureInterface.ReleaseTreasureGuard(guardID)

	})

	t.Run("should set the file name of the treasure", func(t *testing.T) {

		treasureInterface := New(MySaveMethod)
		guardID := treasureInterface.StartTreasureGuard(true, guard.BodyAuthID)
		treasureInterface.BodySetKey(guardID, "test-key")
		treasureInterface.SetContentString(guardID, "test")

		treasureInterface.BodySetFileName(guardID, "test-file-name")

		assert.Equal(t, *treasureInterface.GetFileName(), "test-file-name")
		treasureInterface.ReleaseTreasureGuard(guardID)

	})

	t.Run("should the treasure expired", func(t *testing.T) {

		treasureInterface := New(MySaveMethod)
		guardID := treasureInterface.StartTreasureGuard(true, guard.BodyAuthID)
		treasureInterface.BodySetKey(guardID, "test-key")
		treasureInterface.SetContentString(guardID, "test")

		assert.Equal(t, treasureInterface.IsExpired(), false)

		treasureInterface.SetExpirationTime(guardID, time.Now().Add(-time.Hour))

		assert.Equal(t, treasureInterface.IsExpired(), true)

		treasureInterface.ReleaseTreasureGuard(guardID)

	})

	t.Run("should get the deleted at and deleted by of the treasure", func(t *testing.T) {

		treasureInterface := New(MySaveMethod)
		guardID := treasureInterface.StartTreasureGuard(true, guard.BodyAuthID)
		treasureInterface.BodySetKey(guardID, "test-key")
		treasureInterface.SetContentString(guardID, "test")

		treasureInterface.BodySetForDeletion(guardID, "remover-user", false)

		assert.Equal(t, "remover-user", treasureInterface.GetDeletedBy())
		assert.LessOrEqual(t, treasureInterface.GetDeletedAt(), time.Now().UnixNano())
		assert.Equal(t, ContentTypeVoid, treasureInterface.GetContentType())

		// set the content again for the treasure
		treasureInterface.BodySetKey(guardID, "test-key")
		treasureInterface.SetContentString(guardID, "test")

		assert.Equal(t, "", treasureInterface.GetDeletedBy())
		assert.Equal(t, int64(0), treasureInterface.GetDeletedAt())

		treasureInterface.ReleaseTreasureGuard(guardID)

	})

	t.Run("should test the treasure save method", func(t *testing.T) {

		executed := false
		var SaveFunc = func(t Treasure, guardID guard.ID) TreasureStatus {
			executed = true
			return StatusNew
		}

		treasureInterface := New(SaveFunc)
		guardID := treasureInterface.StartTreasureGuard(true, guard.BodyAuthID)
		treasureInterface.BodySetKey(guardID, "test-key")
		status := treasureInterface.Save(guardID)
		treasureInterface.ReleaseTreasureGuard(guardID)
		assert.Equal(t, StatusNew, status)
		time.Sleep(100 * time.Millisecond)

		assert.Equal(t, true, executed)

	})

}

func TestCreatedAndModifiedFields(t *testing.T) {

	treasureInterface := New(MySaveMethod)
	guardID := treasureInterface.StartTreasureGuard(true)

	createdBy := "creatorUser"
	createdAt := time.Now().UTC()
	treasureInterface.SetCreatedBy(guardID, createdBy)
	treasureInterface.SetCreatedAt(guardID, createdAt)

	if treasureInterface.GetCreatedBy() != createdBy {
		t.Errorf("expected CreatedBy to be %s, got %s", createdBy, treasureInterface.GetCreatedBy())
	}
	if treasureInterface.GetCreatedAt() != createdAt.UnixNano() {
		t.Errorf("expected CreatedAt to be %d, got %d", createdAt.UnixNano(), treasureInterface.GetCreatedAt())
	}

	modifiedBy := "modifierUser"
	modifiedAt := time.Now().UTC().Add(time.Minute)
	treasureInterface.SetModifiedBy(guardID, modifiedBy)
	treasureInterface.SetModifiedAt(guardID, modifiedAt)

	if treasureInterface.GetModifiedBy() != modifiedBy {
		t.Errorf("expected ModifiedBy to be %s, got %s", modifiedBy, treasureInterface.GetModifiedBy())
	}
	if treasureInterface.GetModifiedAt() != modifiedAt.UnixNano() {
		t.Errorf("expected ModifiedAt to be %d, got %d", modifiedAt.UnixNano(), treasureInterface.GetModifiedAt())
	}

	treasureInterface.ReleaseTreasureGuard(guardID)

}
