pub trait Itertools: Iterator {
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
