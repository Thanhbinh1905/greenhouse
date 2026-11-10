pub trait Itertools: Iterator {
    fn strip_prefix<Prefix>(
        self,
        prefix: Prefix,
    ) -> Result<Self, StripPrefixError<Self, Prefix::IntoIter, Self::Item>>
    where
        Self: Sized,
        Prefix: IntoIterator,
        Self::Item: PartialEq<Prefix::Item>,
    {
        self.strip_prefix_by(prefix, |a, b| a == b)
    }

    fn strip_prefix_by<Prefix, F>(
        mut self,
        prefix: Prefix,
        mut eq: F,
    ) -> Result<Self, StripPrefixError<Self, Prefix::IntoIter, Self::Item>>
    where
        Self: Sized,
        Prefix: IntoIterator,
        F: FnMut(&Self::Item, &Prefix::Item) -> bool,
    {
        let mut prefix = prefix.into_iter();
        while let Some(wanted) = prefix.next() {
            match self.next() {
                Some(got) if eq(&got, &wanted) => continue,
                got => {
                    return Err(StripPrefixError {
                        iterator: self,
                        prefix,
                        mismatch: (got, wanted),
                    });
                }
            }
        }
        Ok(self)
    }

}
pub struct StripPrefixError<I, Prefix: Iterator, T> {
    /// The remainder of the original iterator, advanced past the matched
    /// prefix items but stopped at the position of the mismatch.
    pub iterator: I,
    /// The remainder of the prefix iterator, starting just after the prefix
    /// item that failed to match.
    pub prefix: Prefix,
    /// The pair of items that failed to compare equal. The first element is
    /// `None` if `iterator` was exhausted before the prefix was fully matched.
    pub mismatch: (Option<T>, Prefix::Item),
}
