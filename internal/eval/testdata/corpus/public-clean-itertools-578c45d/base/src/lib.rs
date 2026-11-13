pub mod structs {
    #[cfg(feature = "use_alloc")]
    pub use crate::adaptors::MultiProduct;
    pub use crate::adaptors::{
        Batching, Coalesce, Dedup, DedupBy, DedupByWithCount, DedupWithCount, FilterMapOk,
        FilterOk, Interleave, InterleaveShortest, MapInto, MapOk, Positions, Product, PutBack,
        TakeWhileRef, TupleCombinations, Update, WhileSome,
    };
    #[cfg(feature = "use_alloc")]
    pub use crate::combinations::{ArrayCombinations, Combinations};
    #[cfg(feature = "use_alloc")]
    pub use crate::combinations_with_replacement::CombinationsWithReplacement;
    pub use crate::cons_tuples_impl::ConsTuples;
    #[cfg(feature = "use_std")]
    pub use crate::duplicates_impl::{Duplicates, DuplicatesBy};
    pub use crate::exactly_one_err::ExactlyOneError;
    pub use crate::flatten_ok::FlattenOk;
    pub use crate::format::{Format, FormatWith};
    #[allow(deprecated)]
    #[cfg(feature = "use_alloc")]
    pub use crate::groupbylazy::GroupBy;
    #[cfg(feature = "use_alloc")]
    pub use crate::groupbylazy::{Chunk, ChunkBy, Chunks, Group, Groups, IntoChunks};
    #[cfg(feature = "use_std")]
    pub use crate::grouping_map::{GroupingMap, GroupingMapBy};
    pub use crate::intersperse::{Intersperse, IntersperseWith};
    #[cfg(feature = "use_alloc")]
    pub use crate::kmerge_impl::{KMerge, KMergeBy};
    pub use crate::merge_join::{Merge, MergeBy, MergeJoinBy};
    #[cfg(feature = "use_alloc")]
    pub use crate::multipeek_impl::MultiPeek;
    pub use crate::pad_tail::PadUsing;
    #[cfg(feature = "use_alloc")]
    pub use crate::peek_nth::PeekNth;
    pub use crate::peeking_take_while::PeekingTakeWhile;
    #[cfg(feature = "use_alloc")]
    pub use crate::permutations::Permutations;
    #[cfg(feature = "use_alloc")]
    pub use crate::powerset::Powerset;
    pub use crate::process_results_impl::ProcessResults;
    #[cfg(feature = "use_alloc")]
    pub use crate::put_back_n_impl::PutBackN;
    #[cfg(feature = "use_alloc")]
    pub use crate::rciter_impl::RcIter;
    pub use crate::repeatn::RepeatN;
    #[allow(deprecated)]
    pub use crate::sources::{Iterate, Unfold};
    pub use crate::take_while_inclusive::TakeWhileInclusive;
    #[cfg(feature = "use_alloc")]
    pub use crate::tee::Tee;
    pub use crate::tuple_impl::{CircularTupleWindows, TupleBuffer, TupleWindows, Tuples};
    #[cfg(feature = "use_std")]
    pub use crate::unique_impl::{Unique, UniqueBy};
    pub use crate::with_position::WithPosition;
    pub use crate::zip_eq_impl::ZipEq;
    pub use crate::zip_longest::ZipLongest;
    pub use crate::ziptuple::Zip;
}

/// Traits helpful for using certain `Itertools` methods in generic contexts.

mod either_or_both;
pub use crate::either_or_both::EitherOrBoth;
#[doc(hidden)]

pub trait Itertools: Iterator {
    fn tuples<T>(self) -> Tuples<Self, T>
    where
        Self: Sized + Iterator<Item = T::Item>,
        T: traits::HomogeneousTuple,
    {
        tuple_impl::tuples(self)
    }

}
