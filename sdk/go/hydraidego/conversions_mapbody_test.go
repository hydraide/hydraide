package hydraidego

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mapBodyCatalog struct {
	Domain    string    `hydraide:"key"`
	ASN       string    `hydraide:"ASN"`
	TLD       string    `hydraide:"TLD"`
	Priority  int8      `hydraide:"Priority"`
	ClaimedBy string    `hydraide:"ClaimedBy,omitempty"`
	ClaimedAt time.Time `hydraide:"ClaimedAt,omitempty"`
	ExpireAt  time.Time `hydraide:"expireAt"`
}

func TestInspectCatalogModel_Shapes(t *testing.T) {
	t.Run("map-body", func(t *testing.T) {
		shape, fields, err := inspectCatalogModel(reflect.TypeOf(mapBodyCatalog{}))
		require.NoError(t, err)
		assert.Equal(t, catalogShapeMapBody, shape)
		// 5 body fields: ASN, TLD, Priority, ClaimedBy, ClaimedAt
		assert.Len(t, fields, 5)
	})

	t.Run("single-value", func(t *testing.T) {
		type sv struct {
			Key   string `hydraide:"key"`
			Value string `hydraide:"value"`
		}
		shape, fields, err := inspectCatalogModel(reflect.TypeOf(sv{}))
		require.NoError(t, err)
		assert.Equal(t, catalogShapeSingleVal, shape)
		assert.Empty(t, fields)
	})

	t.Run("key-only", func(t *testing.T) {
		type ko struct {
			Key string `hydraide:"key"`
		}
		shape, _, err := inspectCatalogModel(reflect.TypeOf(ko{}))
		require.NoError(t, err)
		assert.Equal(t, catalogShapeKeyOnly, shape)
	})

	t.Run("mixed shape rejected", func(t *testing.T) {
		type mix struct {
			Key   string `hydraide:"key"`
			Value string `hydraide:"value"`
			ASN   string `hydraide:"ASN"`
		}
		_, _, err := inspectCatalogModel(reflect.TypeOf(mix{}))
		assert.Error(t, err)
	})
}

func TestMapBodyCatalog_SaveReadRoundtrip(t *testing.T) {
	exp := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	claimed := time.Date(2026, 5, 8, 11, 30, 0, 0, time.UTC)

	in := &mapBodyCatalog{
		Domain:    "example.hu",
		ASN:       "AS12345",
		TLD:       "hu",
		Priority:  3,
		ClaimedBy: "worker-1",
		ClaimedAt: claimed,
		ExpireAt:  exp,
	}

	kv, err := convertCatalogModelToKeyValuePair(in, EncodingMsgPack)
	require.NoError(t, err)

	// BytesVal must carry the wrapped msgpack body.
	require.NotNil(t, kv.BytesVal)
	assert.True(t, isMsgpackEncoded(kv.BytesVal), "BytesVal must have the msgpack magic prefix")
	assert.Nil(t, kv.VoidVal, "map-body must not be void")
	require.NotNil(t, kv.ExpiredAt, "expireAt must round-trip via the metadata slot, not the body")

	treasure := convertKeyValuePairToTreasure(kv)

	out := &mapBodyCatalog{}
	require.NoError(t, convertProtoTreasureToCatalogModel(treasure, out))

	assert.Equal(t, "example.hu", out.Domain)
	assert.Equal(t, "AS12345", out.ASN)
	assert.Equal(t, "hu", out.TLD)
	assert.Equal(t, int8(3), out.Priority)
	assert.Equal(t, "worker-1", out.ClaimedBy)
	assert.True(t, out.ClaimedAt.Equal(claimed), "got %v want %v", out.ClaimedAt, claimed)
	assert.True(t, out.ExpireAt.Equal(exp))
}

func TestMapBodyCatalog_Omitempty(t *testing.T) {
	exp := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)

	// ClaimedBy / ClaimedAt unset → must be skipped in the body.
	in := &mapBodyCatalog{
		Domain:   "example.hu",
		ASN:      "AS1",
		TLD:      "hu",
		Priority: 1,
		ExpireAt: exp,
	}

	kv, err := convertCatalogModelToKeyValuePair(in, EncodingMsgPack)
	require.NoError(t, err)
	require.NotNil(t, kv.BytesVal)

	treasure := convertKeyValuePairToTreasure(kv)
	out := &mapBodyCatalog{}
	require.NoError(t, convertProtoTreasureToCatalogModel(treasure, out))

	assert.Equal(t, "AS1", out.ASN)
	assert.Equal(t, "", out.ClaimedBy, "omitempty body field must round-trip as zero value")
	assert.True(t, out.ClaimedAt.IsZero())
}

func TestMapBodyCatalog_RejectsValueMixing(t *testing.T) {
	mixed := &struct {
		Key   string `hydraide:"key"`
		Value string `hydraide:"value"`
		ASN   string `hydraide:"ASN"`
	}{Key: "k", Value: "v", ASN: "x"}
	_, err := convertCatalogModelToKeyValuePair(mixed, EncodingMsgPack)
	require.Error(t, err)
}

func TestMapBodyCatalog_KeyOnlyUnchanged(t *testing.T) {
	// Key-only structs (no value tag, no body fields) keep the historical
	// behaviour: BytesVal stays nil, the server treats it as void.
	in := &struct {
		Key string `hydraide:"key"`
	}{Key: "k"}
	kv, err := convertCatalogModelToKeyValuePair(in, EncodingMsgPack)
	require.NoError(t, err)
	assert.Equal(t, "k", kv.Key)
	assert.Nil(t, kv.BytesVal)
}
