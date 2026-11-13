pub trait Itertools: Iterator {
    fn unique(self) -> Unique<Self>
    where
        Self: Sized,
        Self::Item: Clone + Eq + Hash,
    {
        unique_impl::unique(self)
    }

    fn unique_by<V, F>(self, f: F) -> UniqueBy<Self, V, F>
    where
        Self: Sized,
        V: Eq + Hash,
        F: FnMut(&Self::Item) -> V,
    {
        unique_impl::unique_by(self, f)
    }

}
