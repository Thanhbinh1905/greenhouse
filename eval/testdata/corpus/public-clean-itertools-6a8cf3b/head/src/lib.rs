pub trait Itertools: Iterator {
    fn duplicates(self) -> Duplicates<Self>
    where
        Self: Sized,
        Self::Item: Eq + Hash,
    {
        duplicates_impl::duplicates_with_hasher(self, RandomState::new())
    }

    fn duplicates_with_hasher<S>(self, hash_builder: S) -> Duplicates<Self, S>
    where
        Self: Sized,
        Self::Item: Eq + Hash,
        S: BuildHasher,
    {
        duplicates_impl::duplicates_with_hasher(self, hash_builder)
    }

    fn duplicates_by<V, F>(self, f: F) -> DuplicatesBy<Self, V, F>
    where
        Self: Sized,
        V: Eq + Hash,
        F: FnMut(&Self::Item) -> V,
    {
        duplicates_impl::duplicates_by_with_hasher(self, f, RandomState::new())
    }

    fn duplicates_by_with_hasher<V, F, S>(
        self,
        f: F,
        hash_builder: S,
    ) -> DuplicatesBy<Self, V, F, S>
    where
        Self: Sized,
        V: Eq + Hash,
        F: FnMut(&Self::Item) -> V,
        S: BuildHasher,
    {
        duplicates_impl::duplicates_by_with_hasher(self, f, hash_builder)
    }

}
