pub trait Itertools: Iterator {
    fn into_group_map<K, V>(self) -> HashMap<K, Vec<V>>
    where
        Self: Iterator<Item = (K, V)> + Sized,
        K: Hash + Eq,
    {
        group_map::into_group_map(self)
    }

    fn into_group_map_by<K, V, F>(self, f: F) -> HashMap<K, Vec<V>>
    where
        Self: Iterator<Item = V> + Sized,
        K: Hash + Eq,
        F: FnMut(&V) -> K,
    {
        group_map::into_group_map_by(self, f)
    }

}
