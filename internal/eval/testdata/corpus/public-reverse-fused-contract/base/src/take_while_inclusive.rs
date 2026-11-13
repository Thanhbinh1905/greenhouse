impl<I, F> FusedIterator for TakeWhileInclusive<I, F>
where
    I: FusedIterator,
    F: FnMut(&I::Item) -> bool,
{
}
