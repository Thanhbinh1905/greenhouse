impl<T> PeekingNext for ::alloc::vec::IntoIter<T> {
    fn peeking_next<F>(&mut self, accept: F) -> Option<Self::Item>
    where
        F: FnOnce(&Self::Item) -> bool,
    {
        match accept(self.as_slice().first()?) {
            true => self.next(),
            false => None,
        }
    }
}

#[cfg(feature = "use_alloc")]

impl<'a, T> PeekingNext for ::alloc::vec::Drain<'a, T> {
    fn peeking_next<F>(&mut self, accept: F) -> Option<Self::Item>
    where
        F: FnOnce(&Self::Item) -> bool,
    {
        match accept(self.as_slice().first()?) {
            true => self.next(),
            false => None,
        }
    }
}

#[cfg(feature = "use_alloc")]

impl<'a> PeekingNext for ::alloc::string::Drain<'a> {
    fn peeking_next<F>(&mut self, accept: F) -> Option<Self::Item>
    where
        F: FnOnce(&Self::Item) -> bool,
    {
        match accept(&self.as_str().chars().next()?) {
            true => self.next(),
            false => None,
        }
    }
}


impl<T, const N: usize> PeekingNext for ::core::array::IntoIter<T, N> {
    fn peeking_next<F>(&mut self, accept: F) -> Option<Self::Item>
    where
        F: FnOnce(&Self::Item) -> bool,
    {
        match accept(self.as_slice().first()?) {
            true => self.next(),
            false => None,
        }
    }
}
