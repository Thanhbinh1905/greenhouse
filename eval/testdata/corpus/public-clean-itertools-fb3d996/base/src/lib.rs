pub trait Itertools: Iterator {
    fn position_max_by_key<K, F>(self, mut key: F) -> Option<usize>
    where
        Self: Sized,
        K: Ord,
        F: FnMut(&Self::Item) -> K,
    {
        self.map(|item| key(&item)).position_max()
    }

    fn position_min_by_key<K, F>(self, mut key: F) -> Option<usize>
    where
        Self: Sized,
        K: Ord,
        F: FnMut(&Self::Item) -> K,
    {
        self.map(|item| key(&item)).position_min()
    }

    fn position_minmax_by_key<K, F>(self, mut key: F) -> MinMaxResult<usize>
    where
        Self: Sized,
        K: PartialOrd,
        F: FnMut(&Self::Item) -> K,
    {
        self.map(|item| key(&item)).position_minmax()
    }

}
