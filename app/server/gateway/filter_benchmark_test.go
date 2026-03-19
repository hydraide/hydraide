package gateway

import (
	"math"
	"math/rand"
	"testing"

	hydrapb "github.com/hydraide/hydraide/generated/hydraidepbgo"
	"github.com/vmihailenco/msgpack/v5"
)

// makeRandomNormalizedFloat32 creates a random unit-length float32 vector.
func makeRandomNormalizedFloat32(rng *rand.Rand, dim int) []float32 {
	v := make([]float32, dim)
	var norm float32
	for i := range v {
		v[i] = rng.Float32()*2 - 1
		norm += v[i] * v[i]
	}
	invNorm := float32(1.0 / math.Sqrt(float64(norm)))
	for i := range v {
		v[i] *= invNorm
	}
	return v
}

// makeRandomMsgpackBytesVal creates a BytesVal with a random normalized vector field.
func makeRandomMsgpackBytesVal(rng *rand.Rand, dim int) []byte {
	vec := makeRandomNormalizedFloat32(rng, dim)
	floats := make([]interface{}, len(vec))
	for i, v := range vec {
		floats[i] = float64(v)
	}
	data := map[string]interface{}{
		"Category":  "business",
		"Language":  "hu",
		"Embedding": floats,
	}
	encoded, _ := msgpack.Marshal(data)
	return append([]byte{msgpackMagic0, msgpackMagic1}, encoded...)
}

func BenchmarkDotProduct384(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	a := makeRandomNormalizedFloat32(rng, 384)
	v := makeRandomNormalizedFloat32(rng, 384)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dotProduct(a, v)
	}
}

func BenchmarkDotProduct768(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	a := makeRandomNormalizedFloat32(rng, 768)
	v := makeRandomNormalizedFloat32(rng, 768)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dotProduct(a, v)
	}
}

func BenchmarkEvaluateVectorFilter_384(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	bytesVal := makeRandomMsgpackBytesVal(rng, 384)
	queryVec := makeRandomNormalizedFloat32(rng, 384)

	treasure := &hydrapb.Treasure{
		Key:      "bench-domain",
		BytesVal: bytesVal,
	}

	vf := &hydrapb.VectorFilter{
		BytesFieldPath: "Embedding",
		QueryVector:    queryVec,
		MinSimilarity:  0.5,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		evaluateVectorFilter(treasure, vf)
	}
}

func BenchmarkEvaluateVectorFilter_WithPreFilter(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	bytesVal := makeRandomMsgpackBytesVal(rng, 384)
	queryVec := makeRandomNormalizedFloat32(rng, 384)

	treasure := &hydrapb.Treasure{
		Key:      "bench-domain-combined",
		Int32Val: int32Ptr(1), // category match
		BytesVal: bytesVal,
	}

	categoryPath := "Category"
	group := &hydrapb.FilterGroup{
		Logic: hydrapb.FilterLogic_AND,
		Filters: []*hydrapb.TreasureFilter{
			{
				Operator:       hydrapb.Relational_EQUAL,
				CompareValue:   &hydrapb.TreasureFilter_StringVal{StringVal: "business"},
				BytesFieldPath: &categoryPath,
			},
		},
		VectorFilters: []*hydrapb.VectorFilter{
			{
				BytesFieldPath: "Embedding",
				QueryVector:    queryVec,
				MinSimilarity:  0.5,
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		evaluateFilterGroup(treasure, group)
	}
}

// BenchmarkDotProduct384_Batch simulates scanning 10K vectors.
func BenchmarkDotProduct384_Batch10K(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	query := makeRandomNormalizedFloat32(rng, 384)
	vectors := make([][]float32, 10000)
	for i := range vectors {
		vectors[i] = makeRandomNormalizedFloat32(rng, 384)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, v := range vectors {
			dotProduct(query, v)
		}
	}
}
