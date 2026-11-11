pub trait Itertools: Iterator {
    fn unique(self) -> Unique<Self>
    where
        Self: Sized,
        Self::Item: Clone + Eq + Hash,
    {
        unique_impl::unique_with_hasher(self, RandomState::new())
    }

    fn unique_with_hasher<S>(self, hash_builder: S) -> Unique<Self, S>
    where
        Self: Sized,
        Self::Item: Clone + Eq + Hash,
        S: BuildHasher,
    {
        unique_impl::unique_with_hasher(self, hash_builder)
    }

    fn unique_by<V, F>(self, f: F) -> UniqueBy<Self, V, F>
    where
        Self: Sized,
        V: Eq + Hash,
        F: FnMut(&Self::Item) -> V,
    {
        unique_impl::unique_by_with_hasher(self, f, RandomState::new())
    }

    fn unique_by_with_hasher<V, F, S>(self, f: F, hash_builder: S) -> UniqueBy<Self, V, F, S>
    where
        Self: Sized,
        V: Eq + Hash,
        F: FnMut(&Self::Item) -> V,
        S: BuildHasher,
    {
        unique_impl::unique_by_with_hasher(self, f, hash_builder)
    }

}
