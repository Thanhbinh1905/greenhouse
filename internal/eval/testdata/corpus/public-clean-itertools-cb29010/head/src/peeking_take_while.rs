impl<I> PeekingNext for PutBackN<I>
where
    I: Iterator,
{
    fn peeking_next<F>(&mut self, accept: F) -> Option<Self::Item>
    where
        F: FnOnce(&Self::Item) -> bool,
    {
        if let Some(r) = self.next() {
            if !accept(&r) {
                self.put_back(r);
                return None;
            }
            Some(r)
        } else {
            None
        }
    }
}

#[cfg(feature = "use_alloc")]

impl<T> PeekingNext for ::alloc::vec::IntoIter<T> {
    fn peeking_next<F>(&mut self, accept: F) -> Option<Self::Item>
    where
        F: FnOnce(&Self::Item) -> bool,
    {
        self.as_slice().iter().peeking_next(|it| accept(*it))?;
        self.next()
    }
}
