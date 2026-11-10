pub fn into_group_map<I, K, V>(iter: I) -> HashMap<K, Vec<V>>
where
    I: Iterator<Item = (K, V)>,
    K: Hash + Eq,
{
    let mut lookup = HashMap::<K, Vec<V>>::new();

    iter.for_each(|(key, val)| {
        lookup.entry(key).or_default().push(val);
    });

    lookup
}


pub fn into_group_map_by<I, K, V, F>(iter: I, mut f: F) -> HashMap<K, Vec<V>>
where
    I: Iterator<Item = V>,
    K: Hash + Eq,
    F: FnMut(&V) -> K,
{
    into_group_map(iter.map(|v| (f(&v), v)))
}
