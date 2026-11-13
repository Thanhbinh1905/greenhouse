pub trait Itertools: Iterator {
    fn fold_while<B, F>(&mut self, init: B, mut f: F) -> FoldWhile<B>
    where
        Self: Sized,
        F: FnMut(B, Self::Item) -> FoldWhile<B>,
    {
        use core::ops::ControlFlow::{Break, Continue};

        let result = self.try_fold(
            init,
            #[inline(always)]
            |acc, v| match f(acc, v) {
                FoldWhile::Continue(acc) => Continue(acc),
                FoldWhile::Done(acc) => Break(acc),
            },
        );

        match result {
            Continue(acc) => FoldWhile::Continue(acc),
            Break(acc) => FoldWhile::Done(acc),
        }
    }

}
