import random
import string
import time
from collections import defaultdict
from typing import Dict

import pytest
from hydraidepy.name import Name


class TestName:
    def test_name_creation_and_chaining(self) -> None:
        name = Name().sanctuary("users").realm("profiles").swamp("alice123")

        assert name.get() == "users/profiles/alice123"
        assert name.sanctuary_id == "users"
        assert name.realm_name == "profiles"
        assert name.swamp_name == "alice123"

    def test_partial_name_creation(self) -> None:
        name_sanctuary = Name().sanctuary("users")
        assert name_sanctuary.get() == "users"
        assert name_sanctuary.sanctuary_id == "users"
        assert name_sanctuary.realm_name == ""
        assert name_sanctuary.swamp_name == ""

        name_realm = Name().sanctuary("users").realm("profiles")
        assert name_realm.get() == "users/profiles"
        assert name_realm.sanctuary_id == "users"
        assert name_realm.realm_name == "profiles"
        assert name_realm.swamp_name == ""

    def test_wildcard_pattern_detection(self) -> None:
        name_normal = Name().sanctuary("users").realm("profiles").swamp("alice")
        assert not name_normal.is_wildcard_pattern()

        name_sanctuary_wild = Name().sanctuary("*").realm("profiles").swamp("alice")
        assert name_sanctuary_wild.is_wildcard_pattern()

        name_realm_wild = Name().sanctuary("users").realm("*").swamp("alice")
        assert name_realm_wild.is_wildcard_pattern()

        name_swamp_wild = Name().sanctuary("users").realm("profiles").swamp("*")
        assert name_swamp_wild.is_wildcard_pattern()

        name_multi_wild = Name().sanctuary("*").realm("*").swamp("*")
        assert name_multi_wild.is_wildcard_pattern()

    def test_load_from_path(self) -> None:
        name_full = Name.load("users/profiles/alice123")
        assert name_full.get() == "users/profiles/alice123"
        assert name_full.sanctuary_id == "users"
        assert name_full.realm_name == "profiles"
        assert name_full.swamp_name == "alice123"

        name_sanctuary = Name.load("users")
        assert name_sanctuary.get() == "users"
        assert name_sanctuary.sanctuary_id == "users"
        assert name_sanctuary.realm_name == ""
        assert name_sanctuary.swamp_name == ""

        name_realm = Name.load("users/profiles")
        assert name_realm.get() == "users/profiles"
        assert name_realm.sanctuary_id == "users"
        assert name_realm.realm_name == "profiles"
        assert name_realm.swamp_name == ""

    def test_get_island_id_consistency(self) -> None:
        name = Name().sanctuary("users").realm("profiles").swamp("alice123")
        all_islands = 1000

        island_id_1 = name.get_island_id(all_islands)
        island_id_2 = name.get_island_id(all_islands)
        island_id_3 = name.get_island_id(all_islands)

        assert island_id_1 == island_id_2 == island_id_3
        assert 1 <= island_id_1 <= all_islands

    def test_get_island_id_different_names_different_ids(self) -> None:
        all_islands = 1000
        name1 = Name().sanctuary("users").realm("profiles").swamp("alice")
        name2 = Name().sanctuary("users").realm("profiles").swamp("bob")
        name3 = Name().sanctuary("products").realm("catalog").swamp("item123")

        id1 = name1.get_island_id(all_islands)
        id2 = name2.get_island_id(all_islands)
        id3 = name3.get_island_id(all_islands)

        assert len({id1, id2, id3}) >= 2  # At least 2 should be different

    def test_get_island_id_caching(self) -> None:
        name = Name().sanctuary("users").realm("profiles").swamp("alice123")

        assert name.island_number == 0

        island_id = name.get_island_id(1000)
        assert name.island_number == island_id
        assert island_id > 0

        assert name.get_island_id(1000) == island_id


class TestNameDistribution:
    @staticmethod
    def _generate_random_string(length: int = 8) -> str:
        return "".join(random.choices(string.ascii_letters + string.digits, k=length))

    def test_get_island_id_distribution(self) -> None:
        test_data_count = 100_000
        all_islands = 100

        island_counts: Dict[int, int] = defaultdict(int)

        for _ in range(test_data_count):
            sanctuary = self._generate_random_string()
            realm = self._generate_random_string()
            swamp = self._generate_random_string()

            name = Name().sanctuary(sanctuary).realm(realm).swamp(swamp)
            island_id = name.get_island_id(all_islands)
            island_counts[island_id] += 1

        assert len(island_counts) == all_islands

        expected_per_island = test_data_count // all_islands
        threshold = expected_per_island // 8  # allow 12.5% variance

        for island_id, count in island_counts.items():
            assert 1 <= island_id <= all_islands, f"Island ID {island_id} out of range"
            assert (
                expected_per_island - threshold
                <= count
                <= expected_per_island + threshold
            ), (
                f"Island {island_id} has {count} entries, expected ~{expected_per_island} ± {threshold}"
            )

        mean_count = sum(island_counts.values()) / len(island_counts)
        variance = sum(
            (count - mean_count) ** 2 for count in island_counts.values()
        ) / len(island_counts)
        std_dev = variance**0.5
        coefficient_of_variation = std_dev / mean_count

        assert coefficient_of_variation < 0.1, (
            f"Distribution too uneven: CV = {coefficient_of_variation:.4f}"
        )


class TestNameEdgeCases:
    def test_special_characters_in_names(self) -> None:
        special_chars = "test-name_123.value@domain"
        name = Name().sanctuary(special_chars).realm(special_chars).swamp(special_chars)
        expected = f"{special_chars}/{special_chars}/{special_chars}"
        assert name.get() == expected

    def test_unicode_characters(self) -> None:
        unicode_name = "测试_🔥_αβγ"
        name = Name().sanctuary(unicode_name).realm(unicode_name).swamp(unicode_name)
        expected = f"{unicode_name}/{unicode_name}/{unicode_name}"
        assert name.get() == expected

    def test_very_long_names(self) -> None:
        long_name = "x" * 1000
        name = Name().sanctuary(long_name).realm(long_name).swamp(long_name)
        expected = f"{long_name}/{long_name}/{long_name}"
        assert name.get() == expected

        island_id = name.get_island_id(100)
        assert 1 <= island_id <= 100

    def test_island_id_with_different_ranges(self) -> None:
        name = Name().sanctuary("test").realm("test").swamp("test")

        for all_islands in [1, 2, 10, 100, 1000, 10000]:
            island_id = name.get_island_id(all_islands)
            assert 1 <= island_id <= all_islands

            name.island_number = 0


class TestNamePerformance:
    """
    Performance benchmarks for Name operations.

    These tests mirror the Go SDK benchmarks to enable direct performance
    comparison between Python and Go implementations.
    """

    def test_benchmark_name_add(self) -> None:
        iterations = 25_000

        start_time = time.perf_counter()
        for _ in range(iterations):
            Name().sanctuary("users").realm("johndoe").swamp("info")
        end_time = time.perf_counter()

        total_time = end_time - start_time
        avg_time_ns = (total_time / iterations) * 1_000_000_000

        print(f"\nBenchmarkName_Add: {avg_time_ns:.2f} ns/op ({iterations} iterations)")
        print(f"Total time: {total_time:.4f}s")

        assert avg_time_ns < 200

    def test_benchmark_name_get(self) -> None:
        name_obj = Name().sanctuary("users").realm("johndoe").swamp("info")
        iterations = 1_000_000

        start_time = time.perf_counter()
        for _ in range(iterations):
            name_obj.get()
        end_time = time.perf_counter()

        total_time = end_time - start_time
        avg_time_ns = (total_time / iterations) * 1_000_000_000

        print(f"BenchmarkName_Get: {avg_time_ns:.2f} ns/op ({iterations} iterations)")
        print(f"Total time: {total_time:.4f}s")

        assert avg_time_ns < 40

    def test_benchmark_name_load(self) -> None:
        canonical_form = "users/johndoe/info"
        iterations = 100_000

        start_time = time.perf_counter()
        for _ in range(iterations):
            Name.load(canonical_form)
        end_time = time.perf_counter()

        total_time = end_time - start_time
        avg_time_ns = (total_time / iterations) * 1_000_000_000

        print(f"BenchmarkName_Load: {avg_time_ns:.2f} ns/op ({iterations} iterations)")
        print(f"Total time: {total_time:.4f}s")

        assert avg_time_ns < 400

    def test_benchmark_get_island_id(self) -> None:
        sanctuary = "BenchmarkSanctuary"
        realm = "BenchmarkRealm"
        swamp = "BenchmarkSwamp"
        all_folders = 100
        iterations = 76_000

        start_time = time.perf_counter()
        for _ in range(iterations):
            name = Name().sanctuary(sanctuary).realm(realm).swamp(swamp)
            name.get_island_id(all_folders)
        end_time = time.perf_counter()

        total_time = end_time - start_time
        avg_time_ns = (total_time / iterations) * 1_000_000_000

        print(
            f"BenchmarkGetIslandID: {avg_time_ns:.2f} ns/op ({iterations} iterations)"
        )
        print(f"Total time: {total_time:.4f}s")

        assert avg_time_ns < 400

    def test_benchmark_get_island_id_cached(self) -> None:
        name = Name().sanctuary("users").realm("profiles").swamp("alice123")
        all_islands = 1000
        iterations = 1_000_000

        name.get_island_id(all_islands)

        start_time = time.perf_counter()
        for _ in range(iterations):
            name.get_island_id(all_islands)
        end_time = time.perf_counter()

        total_time = end_time - start_time
        avg_time_ns = (total_time / iterations) * 1_000_000_000

        print(
            f"BenchmarkGetIslandID_Cached: {avg_time_ns:.2f} ns/op ({iterations} iterations)"
        )
        print(f"Total time: {total_time:.4f}s")

        assert avg_time_ns < 40

    @pytest.mark.skip(reason="Meant for manual performance comparison")
    def test_performance_comparison_summary(self) -> None:
        print("\n" + "=" * 60)
        print("PERFORMANCE COMPARISON: Python vs Go SDK")
        print("=" * 60)

        benchmarks = []

        iterations_add = 25_000
        start = time.perf_counter()
        for _ in range(iterations_add):
            Name().sanctuary("users").realm("johndoe").swamp("info")
        add_time_ns = ((time.perf_counter() - start) / iterations_add) * 1_000_000_000
        benchmarks.append(("Name Creation", add_time_ns, "41.09", iterations_add))

        name_obj = Name().sanctuary("users").realm("johndoe").swamp("info")
        iterations_get = 1_000_000
        start = time.perf_counter()
        for _ in range(iterations_get):
            name_obj.get()
        get_time_ns = ((time.perf_counter() - start) / iterations_get) * 1_000_000_000
        benchmarks.append(("Name.get()", get_time_ns, "0.52", iterations_get))

        iterations_load = 100_000
        start = time.perf_counter()
        for _ in range(iterations_load):
            Name.load("users/johndoe/info")
        load_time_ns = ((time.perf_counter() - start) / iterations_load) * 1_000_000_000
        benchmarks.append(("Name.load()", load_time_ns, "320.5", iterations_load))

        iterations_island = 76_000
        start = time.perf_counter()
        for _ in range(iterations_island):
            name = (
                Name()
                .sanctuary("BenchmarkSanctuary")
                .realm("BenchmarkRealm")
                .swamp("BenchmarkSwamp")
            )
            name.get_island_id(100)
        island_time_ns = (
            (time.perf_counter() - start) / iterations_island
        ) * 1_000_000_000
        benchmarks.append(
            ("get_island_id()", island_time_ns, "15.19", iterations_island)
        )

        print(
            f"{'Operation':<20} {'Python (ns/op)':<15} {'Go (ns/op)':<12} {'Ratio':<10} {'Iterations':<12}"
        )
        print("-" * 70)

        for op_name, py_time, go_time, iterations in benchmarks:
            ratio = py_time / float(go_time)
            print(
                f"{op_name:<20} {py_time:<15.2f} {go_time:<12} {ratio:<10.1f}x {iterations:<12,}"
            )

        print("\nNote: Ratio shows how many times slower Python is compared to Go")
        print("Lower ratios indicate better relative performance")

        assert all(result[1] > 0 for result in benchmarks), (
            "All benchmarks should produce positive timing results"
        )
