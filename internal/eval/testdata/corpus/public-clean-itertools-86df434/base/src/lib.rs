pub trait Itertools: Iterator {
    fn all_unique(&mut self) -> bool
    where
        Self: Sized,
        Self::Item: Eq + Hash,
    {
        let mut used = HashSet::new();
        self.all(move |elt| used.insert(elt))
    }

}
