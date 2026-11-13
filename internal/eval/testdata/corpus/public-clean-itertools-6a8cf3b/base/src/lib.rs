pub trait Itertools: Iterator {
    fn duplicates(self) -> Duplicates<Self>
    where
        Self: Sized,
        Self::Item: Eq + Hash,
    {
        duplicates_impl::duplicates(self)
    }

    fn duplicates_by<V, F>(self, f: F) -> DuplicatesBy<Self, V, F>
    where
        Self: Sized,
        V: Eq + Hash,
        F: FnMut(&Self::Item) -> V,
    {
        duplicates_impl::duplicates_by(self, f)
    }

}
