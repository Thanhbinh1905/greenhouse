pub trait Itertools: Iterator {
    fn into_grouping_map<K, V>(self) -> GroupingMap<Self>
    where
        Self: Iterator<Item = (K, V)> + Sized,
        K: Hash + Eq,
    {
        grouping_map::new(self, RandomState::new())
    }

    fn into_grouping_map_with_hasher<K, V, S>(self, hash_builder: S) -> GroupingMap<Self, S>
    where
        Self: Iterator<Item = (K, V)> + Sized,
        K: Hash + Eq,
        S: BuildHasher,
    {
        grouping_map::new(self, hash_builder)
    }

    fn into_grouping_map_by<K, V, F>(self, key_mapper: F) -> GroupingMapBy<Self, F>
    where
        Self: Iterator<Item = V> + Sized,
        K: Hash + Eq,
        F: FnMut(&V) -> K,
    {
        grouping_map::new(
            grouping_map::new_map_for_grouping(self, key_mapper),
            RandomState::new(),
        )
    }

    fn into_grouping_map_by_with_hasher<K, V, F, S>(
        self,
        key_mapper: F,
        hash_builder: S,
    ) -> GroupingMapBy<Self, F, S>
    where
        Self: Iterator<Item = V> + Sized,
        K: Hash + Eq,
        F: FnMut(&V) -> K,
        S: BuildHasher,
    {
        grouping_map::new(
            grouping_map::new_map_for_grouping(self, key_mapper),
            hash_builder,
        )
    }

}
