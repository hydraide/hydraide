"""
Provides a deterministic naming convention for HydrAIDE, helping developers
structure and resolve data into a three-level hierarchy:
Sanctuary → Realm → Swamp.

This structure enables:
- O(1) access to data based on name
- Stateless client-side routing in multi-server setups
- Predictable folder and server mapping without orchestrators

Each Name is constructed step-by-step (Sanctuary → Realm → Swamp),
and the full path can be retrieved using get(). Additionally,
get_island_id(all_folders) maps the current name to a consistent
server index (1-based), using a fast and collision-resistant hash.

🛑 Constraints:

  - All three components — sanctuary, realm, and swamp — are **required**.
  - Each part must be at least **1 character long**.
  - The `/` character is **not allowed** in any part of the name.
    It is reserved as a structural delimiter and must not be used within values.
  - The use of **alphanumeric characters** (a–z, A–Z, 0–9) is strongly recommended
    to ensure compatibility and predictable routing across all environments.

⚠️ Note: This package does **not** perform runtime validation of these constraints.
To keep the `name` package as fast as possible (ns-level execution),
no checks or sanitization are performed internally.
It is the responsibility of the caller to ensure valid input.
Adding validation would significantly degrade performance (10–20x).

Example usage:
    name = Name().sanctuary("users").realm("profiles").swamp("alice123")
    print(name.get())  # "users/profiles/alice123"
    print(name.get_island_id(1000))  # e.g. 774

Use load(path) to reconstruct a Name from an existing path string.

This package is used across HydrAIDE SDKs to:
- Determine data placement
- Support distributed architectures
- Enforce clean, intention-driven naming
----------------------------------------
📘 HydrAIDE Python SDK
Full SDK documentation:
https://github.com/hydraide/hydraide/blob/main/docs/sdk/python/README.md
----------------------------------------
"""

from xxhash import xxh64_intdigest


class Name:
    """
    Defines a structured identifier used in HydrAIDE to deterministically
    map data into a distributed, folder-based architecture.

    Each Name represents a three-level hierarchy:
        Sanctuary → Realm → Swamp

    This structure is essential for:
    - Organizing Swamps into logical domains
    - Generating predictable folder paths
    - Assigning each Swamp to a specific folder without coordination

    The interface supports fluent chaining:
        name = Name().sanctuary("users").realm("profiles").swamp("alice123")
        name.get() # "users/profiles/alice123"
        name.get_island_id(100) # e.g. 42

    Usage of get_island_id ensures even distribution of data across N folders,
    enabling stateless multi-node architectures without external orchestrators.

    See also: load(path) to reconstruct a Name from a path.
    """

    __slots__ = (
        "path",
        "sanctuary_id",
        "realm_name",
        "swamp_name",
        "island_number",
    )

    def __init__(self) -> None:
        """
        Creates a new empty Name instance.
        Use this as the starting point for building hierarchical names
        by chaining sanctuary(), realm(), and swamp().
        """
        self.path = ""
        self.sanctuary_id = ""
        self.realm_name = ""
        self.swamp_name = ""
        self.island_number = 0

    def sanctuary(self, sanctuary_id: str) -> "Name":
        """
        Sets the top-level domain of the Name.
        Typically used to group major logical areas (e.g. "users", "products").

        :param sanctuary_id: The identifier for the Sanctuary.
        :return: The Name instance with the sanctuary set.
        """
        self.sanctuary_id = sanctuary_id
        self.path = sanctuary_id
        return self

    def realm(self, realm_name: str) -> "Name":
        """
        Sets the second-level scope under the Sanctuary.
        Often used to further categorize Swamps (e.g. "profiles", "settings").

        :param realm_name: The identifier for the Realm.
        :return: The Name instance with the realm set.
        """
        self.realm_name = realm_name
        self.path = f"{self.path}/{realm_name}"
        return self

    def swamp(self, swamp_name: str) -> "Name":
        """
        Sets the final segment of the Name — the Swamp itself.
        This represents the concrete storage unit where Treasures are kept.
        The full path becomes: sanctuary/realm/swamp.

        :param swamp_name: The identifier for the Swamp.
        :return: The Name instance with the swamp set.
        """
        self.swamp_name = swamp_name
        self.path = f"{self.path}/{swamp_name}"
        return self

    def get(self) -> str:
        """
        Returns the full hierarchical path of the Name in the format:
        "sanctuary/realm/swamp"

        🔒 Internal use only: This method is intended for SDK-level logic,
        such as logging, folder path generation, or internal diagnostics.
        SDK users should never need to call this directly.

        :return: The full path string.
        """
        return self.path

    def is_wildcard_pattern(self) -> bool:
        """
        Returns true if any part of the Name is set to "*".

        :return: True if the name contains a wildcard, False otherwise.
        """
        return (
            self.swamp_name == "*" or self.realm_name == "*" or self.sanctuary_id == "*"
        )

    def get_island_id(self, all_islands: int) -> int:
        """
        Returns the deterministic, 1-based ID of the Island where this Name
        physically resides.

        An Island is HydrAIDE’s smallest migratable physical unit — a
        deterministic storage zone that groups one or more Swamps under the
        same hash bucket. The result of this function is used to determine
        which HydrAIDE server should store the Swamp represented by this Name.

        The IslandID is calculated using a fast, consistent xxhash over the
        combined SanctuaryID, RealmName, and SwampName. The hash value is
        mapped into the provided `all_islands` range, which must be
        consistent across all clients and routers to ensure predictable
        behavior.

        📦 What is an Island?
        - A logical+physical storage unit that lives as a top-level folder
          (e.g. /data/234/)
        - The place where a Swamp is anchored
        - A fixed destination for a given SwampName, regardless of
          infrastructure changes

        🌐 Why does this matter?
        - Enables decentralized routing without coordination
        - Makes server assignments stateless and predictable
        - Supports seamless migration (moving Islands ≠ renaming Swamps)

        🚫 This function should not be used directly by application code.
        It is intended for SDK-internal routing logic.

        Example:
            island_id = name.get_island_id(1000)
            # client = router.route(island_id)

        💡 If you update the hash space (all_islands), all previous
        IslandID mappings change. Keep `all_islands` fixed across your
        system lifetime for stable routing.

        :param all_islands: The total number of available islands.
        :return: The calculated 1-based island ID.
        """
        if self.island_number != 0:
            return self.island_number

        hash_input = self.sanctuary_id + self.realm_name + self.swamp_name
        _hash = xxh64_intdigest(hash_input)
        self.island_number = _hash % all_islands + 1
        return self.island_number

    @staticmethod
    def load(path: str) -> "Name":
        """
        Reconstructs a Name from a given path string in the format:
        "sanctuary/realm/swamp"

        It parses the path segments and returns a Name instance with all
        fields set.

        🔒 Internal use only: This function is intended for SDK-level logic,
        such as reconstructing a Name from persisted references, file paths,
        or routing metadata. It should not be called by application
        developers directly.

        :param path: The path string to parse.
        :return: A new Name instance.
        :raises ValueError: If the path is not in the expected format.
        """
        parts = path.split("/", 2)

        name = Name().sanctuary(sanctuary_id=parts[0])

        if len(parts) > 1:
            name.realm(realm_name=parts[1])

        if len(parts) > 2:
            name.swamp(swamp_name=parts[2])

        return name
