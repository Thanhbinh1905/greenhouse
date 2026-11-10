pub trait Itertools: Iterator {
    fn into_grouping_map<K, V>(self) -> GroupingMap<Self>
    where
        Self: Iterator<Item = (K, V)> + Sized,
        K: Hash + Eq,
    {
        grouping_map::new(self)
    }

    fn into_grouping_map_by<K, V, F>(self, key_mapper: F) -> GroupingMapBy<Self, F>
    where
        Self: Iterator<Item = V> + Sized,
        K: Hash + Eq,
        F: FnMut(&V) -> K,
    {
        grouping_map::new(grouping_map::new_map_for_grouping(self, key_mapper))
    }

}
