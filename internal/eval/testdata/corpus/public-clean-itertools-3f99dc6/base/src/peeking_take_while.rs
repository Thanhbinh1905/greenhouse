impl<I, F> PeekingNext for PeekingTakeWhile<'_, I, F>
where
    I: PeekingNext,
    F: FnMut(&I::Item) -> bool,
{
    fn peeking_next<G>(&mut self, g: G) -> Option<Self::Item>
    where
        G: FnOnce(&Self::Item) -> bool,
    {
        let f = &mut self.f;
        self.iter.peeking_next(|r| f(r) && g(r))
    }
}

// Some iterators are so lightweight we can simply clone them to save their
// state and use that for peeking.
macro_rules! peeking_next_by_clone {
    ([$($typarm:tt)*] $type_:ty) => {
        impl<$($typarm)*> PeekingNext for $type_ {
            fn peeking_next<F>(&mut self, accept: F) -> Option<Self::Item>
                where F: FnOnce(&Self::Item) -> bool
            {
                let saved_state = self.clone();
                if let Some(r) = self.next() {
                    if !accept(&r) {
                        *self = saved_state;
                    } else {
                        return Some(r)
                    }
                }
                None
            }
        }
    }
}

peeking_next_by_clone! { ['a, T] ::std::slice::Iter<'a, T> }
peeking_next_by_clone! { ['a] ::std::str::Chars<'a> }
peeking_next_by_clone! { ['a] ::std::str::CharIndices<'a> }
peeking_next_by_clone! { ['a] ::std::str::Bytes<'a> }
peeking_next_by_clone! { ['a, T] ::std::option::Iter<'a, T> }
peeking_next_by_clone! { ['a, T] ::std::result::Iter<'a, T> }
peeking_next_by_clone! { [T] ::std::iter::Empty<T> }
#[cfg(feature = "use_alloc")]
peeking_next_by_clone! { ['a, T] alloc::collections::linked_list::Iter<'a, T> }
#[cfg(feature = "use_alloc")]
peeking_next_by_clone! { ['a, T] alloc::collections::vec_deque::Iter<'a, T> }

// cloning a Rev has no extra overhead; peekable and put backs are never DEI.
peeking_next_by_clone! { [I: Clone + PeekingNext + DoubleEndedIterator]
::std::iter::Rev<I> }
