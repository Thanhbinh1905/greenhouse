impl<I> Debug for ExactlyOneError<I>
where
    I: Iterator,
    I::Item: Debug,
{
    fn fmt(&self, f: &mut Formatter) -> FmtResult {
        let mut dbg = f.debug_struct("ExactlyOneError");
        match &self.first_two {
            Some(Either::Left([first, second])) => {
                dbg.field("first", first).field("second", second);
            }
            Some(Either::Right(second)) => {
                dbg.field("second", second);
            }
            None => {}
        }
        dbg.finish_non_exhaustive()
    }
}

#[cfg(feature = "use_std")]

impl<I> Error for ExactlyOneError<I>
where
    I: Iterator,
    I::Item: Debug,
{
}
