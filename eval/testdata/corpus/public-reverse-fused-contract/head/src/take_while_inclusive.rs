impl<I, F> FusedIterator for TakeWhileInclusive<I, F>
where
    I: Iterator,
    F: FnMut(&I::Item) -> bool,
{
}

