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
