pub trait Itertools: Iterator {
    fn into_group_map<K, V>(self) -> HashMap<K, Vec<V>>
    where
        Self: Iterator<Item = (K, V)> + Sized,
        K: Hash + Eq,
    {
        group_map::into_group_map_with_hasher(self, RandomState::new())
    }

    fn into_group_map_with_hasher<K, V, S>(self, hash_builder: S) -> HashMap<K, Vec<V>, S>
    where
        Self: Iterator<Item = (K, V)> + Sized,
        K: Hash + Eq,
        S: BuildHasher,
    {
        group_map::into_group_map_with_hasher(self, hash_builder)
    }

    fn into_group_map_by<K, V, F>(self, f: F) -> HashMap<K, Vec<V>>
    where
        Self: Iterator<Item = V> + Sized,
        K: Hash + Eq,
        F: FnMut(&V) -> K,
    {
        group_map::into_group_map_by_with_hasher(self, f, RandomState::new())
    }

    fn into_group_map_by_with_hasher<K, V, F, S>(
        self,
        f: F,
        hash_builder: S,
    ) -> HashMap<K, Vec<V>, S>
    where
        Self: Iterator<Item = V> + Sized,
        K: Hash + Eq,
        F: FnMut(&V) -> K,
        S: BuildHasher,
    {
        group_map::into_group_map_by_with_hasher(self, f, hash_builder)
    }

}
