pub trait Itertools: Iterator {
    fn all_unique(&mut self) -> bool
    where
        Self: Sized,
        Self::Item: Eq + Hash,
    {
        self.all_unique_with_hasher(RandomState::new())
    }

    fn all_unique_with_hasher<S>(&mut self, hash_builder: S) -> bool
    where
        Self: Sized,
        Self::Item: Eq + Hash,
        S: BuildHasher,
    {
        let mut used = HashSet::with_hasher(hash_builder);
        self.all(move |elt| used.insert(elt))
    }

}
