pub trait Itertools: Iterator {
    fn counts(self) -> HashMap<Self::Item, usize>
    where
        Self: Sized,
        Self::Item: Eq + Hash,
    {
        let mut counts = HashMap::new();
        self.for_each(|item| *counts.entry(item).or_default() += 1);
        counts
    }

    fn counts_by<K, F>(self, f: F) -> HashMap<K, usize>
    where
        Self: Sized,
        K: Eq + Hash,
        F: FnMut(Self::Item) -> K,
    {
        self.map(f).counts()
    }

}
