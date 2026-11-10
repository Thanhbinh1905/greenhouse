impl<K, I, F> GroupInner<K, I, F>
where
    I: Iterator,
    F: for<'a> KeyFunction<&'a I::Item, Key = K>,
    K: PartialEq,
{
    /// `client`: Index of group that requests next element
    #[inline(always)]
    fn step(&mut self, client: usize) -> Option<I::Item> {
        /*
        println!("client={}, bottom_group={}, oldest_buffered_group={}, top_group={}, buffers=[{}]",
                 client, self.bottom_group, self.oldest_buffered_group,
                 self.top_group,
                 self.buffer.iter().map(|elt| elt.len()).format(", "));
        */
        if client < self.oldest_buffered_group {
            None
        } else if client < self.top_group
            || (client == self.top_group && self.buffer.len() > self.top_group - self.bottom_group)
        {
            self.lookup_buffer(client)
        } else if self.done {
            None
        } else if self.top_group == client {
            self.step_current()
        } else {
            self.step_buffering(client)
        }
    }

    #[inline(never)]
    fn lookup_buffer(&mut self, client: usize) -> Option<I::Item> {
        // if `bufidx` doesn't exist in self.buffer, it might be empty
        let bufidx = client - self.bottom_group;
        if client < self.oldest_buffered_group {
            return None;
        }
        let elt = self.buffer.get_mut(bufidx).and_then(|queue| queue.next());
        if elt.is_none() && client == self.oldest_buffered_group {
            // FIXME: VecDeque is unfortunately not zero allocation when empty,
            // so we do this job manually.
            // `bottom_group..oldest_buffered_group` is unused, and if it's large enough, erase it.
            self.oldest_buffered_group += 1;
            // skip forward further empty queues too
            while self
                .buffer
                .get(self.oldest_buffered_group - self.bottom_group)
                .map_or(false, |buf| buf.len() == 0)
            {
                self.oldest_buffered_group += 1;
            }

            let nclear = self.oldest_buffered_group - self.bottom_group;
            if nclear > 0 && nclear >= self.buffer.len() / 2 {
                let mut i = 0;
                self.buffer.retain(|buf| {
                    i += 1;
                    debug_assert!(buf.len() == 0 || i > nclear);
                    i > nclear
                });
                self.bottom_group = self.oldest_buffered_group;
            }
        }
        elt
    }

    /// Take the next element from the iterator, and set the done
    /// flag if exhausted. Must not be called after done.
    #[inline(always)]
    fn next_element(&mut self) -> Option<I::Item> {
        debug_assert!(!self.done);
        match self.iter.next() {
            None => {
                self.done = true;
                None
            }
            otherwise => otherwise,
        }
    }

    #[inline(never)]
    fn step_buffering(&mut self, client: usize) -> Option<I::Item> {
        // requested a later group -- walk through the current group up to
        // the requested group index, and buffer the elements (unless
        // the group is marked as dropped).
        // Because the `Groups` iterator is always the first to request
        // each group index, client is the next index efter top_group.
        debug_assert!(self.top_group + 1 == client);
        let mut group = Vec::new();

        if let Some(elt) = self.current_elt.take() {
            if self.top_group != self.dropped_group {
                group.push(elt);
            }
        }
        let mut first_elt = None; // first element of the next group

        while let Some(elt) = self.next_element() {
            let key = self.key.call_mut(&elt);
            match self.current_key.take() {
                Some(old_key) if old_key != key => {
                    self.current_key = Some(key);
                    first_elt = Some(elt);
                    break;
                }
                _ => {}
            }
            self.current_key = Some(key);
            if self.top_group != self.dropped_group {
                group.push(elt);
            }
        }

        if self.top_group != self.dropped_group {
            self.push_next_group(group);
        }
        if first_elt.is_some() {
            self.top_group += 1;
            debug_assert!(self.top_group == client);
        }
        first_elt
    }

    fn push_next_group(&mut self, group: Vec<I::Item>) {
        // When we add a new buffered group, fill up slots between oldest_buffered_group and top_group
        while self.top_group - self.bottom_group > self.buffer.len() {
            if self.buffer.is_empty() {
                self.bottom_group += 1;
                self.oldest_buffered_group += 1;
            } else {
                self.buffer.push(Vec::new().into_iter());
            }
        }
        self.buffer.push(group.into_iter());
        debug_assert!(self.top_group + 1 - self.bottom_group == self.buffer.len());
    }

    /// This is the immediate case, where we use no buffering
    #[inline]
    fn step_current(&mut self) -> Option<I::Item> {
        debug_assert!(!self.done);
        if let elt @ Some(..) = self.current_elt.take() {
            return elt;
        }
        match self.next_element() {
            None => None,
            Some(elt) => {
                let key = self.key.call_mut(&elt);
                match self.current_key.take() {
                    Some(old_key) if old_key != key => {
                        self.current_key = Some(key);
                        self.current_elt = Some(elt);
                        self.top_group += 1;
                        return None;
                    }
                    _ => {}
                }
                self.current_key = Some(key);
                Some(elt)
            }
        }
    }

    /// Request the just started groups' key.
    ///
    /// `client`: Index of group
    ///
    /// **Panics** if no group key is available.
    fn group_key(&mut self, client: usize) -> K {
        // This can only be called after we have just returned the first
        // element of a group.
        // Perform this by simply buffering one more element, grabbing the
        // next key.
        debug_assert!(!self.done);
        debug_assert!(client == self.top_group);
        debug_assert!(self.current_key.is_some());
        debug_assert!(self.current_elt.is_none());
        let old_key = self.current_key.take().unwrap();
        if let Some(elt) = self.next_element() {
            let key = self.key.call_mut(&elt);
            if old_key != key {
                self.top_group += 1;
            }
            self.current_key = Some(key);
            self.current_elt = Some(elt);
        }
        old_key
    }
}


