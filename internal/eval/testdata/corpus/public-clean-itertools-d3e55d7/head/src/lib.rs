pub trait Itertools: Iterator {
    fn counts(self) -> HashMap<Self::Item, usize>
    where
        Self: Sized,
        Self::Item: Eq + Hash,
    {
        self.counts_with_hasher(RandomState::new())
    }

    fn counts_with_hasher<S>(self, hash_builder: S) -> HashMap<Self::Item, usize, S>
    where
        Self: Sized,
        Self::Item: Eq + Hash,
        S: BuildHasher,
    {
        let mut counts = HashMap::with_hasher(hash_builder);
        self.for_each(|item| *counts.entry(item).or_default() += 1);
        counts
    }

    fn counts_by<K, F>(self, f: F) -> HashMap<K, usize>
    where
        Self: Sized,
        K: Eq + Hash,
        F: FnMut(Self::Item) -> K,
    {
        self.counts_by_with_hasher(f, RandomState::new())
    }

    fn counts_by_with_hasher<K, F, S>(self, f: F, hash_builder: S) -> HashMap<K, usize, S>
    where
        Self: Sized,
        K: Eq + Hash,
        F: FnMut(Self::Item) -> K,
        S: BuildHasher,
    {
        self.map(f).counts_with_hasher(hash_builder)
    }

}
