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
        match prefix.by_ref().try_for_each(|wanted| match self.next() {
            Some(got) if eq(&got, &wanted) => Ok(()),
            got => Err((got, wanted)),
        }) {
            Ok(()) => Ok(self),
            Err(mismatch) => Err(StripPrefixError {
                iterator: self,
                prefix,
                mismatch,
            }),
        }
    }

}
