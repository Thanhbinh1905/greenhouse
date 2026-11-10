interface Entry {
  value: unknown;
  /** `null` during the visit's synchronous part, so a hit then is an ancestor; a `Link` while async work is left; then what `record` returns. */
  issues: unknown[] | Link | null;
}

/** A visit in an async parse, carried on the context because a continuation keeps the one it was given. */

interface Link {
  up: Link | undefined;
  /** Settles once the entry has its issues. */
  done: Promise<unknown> | undefined;
  /** The parents of the visits waiting on this one. */
  waiters: Set<Link> | undefined;
}


type WithState = { [STATE]?: State; "~link"?: Link };


function replay(rec: unknown[], into: errors.$ZodRawIssue[]): void {
  for (let i = 0; i < rec.length; i += 2) {
    const iss = rec[i] as errors.$ZodRawIssue;
    into.push(iss.path ? { ...iss, path: iss.path.slice(iss.path.length - (rec[i + 1] as number)) } : { ...iss });
  }
}

// whether `target` can't finish until `from` does: it is above `from`, or above a visit waiting on something that is

function blockedOn(from: Link | undefined, target: Link): boolean {
  // a wait this visit already has adds no edge, so it can't close a loop
  if (from && target.waiters?.has(from)) return false;
  const seen = new Set<Link>();
  const pending = [from];
  while (pending.length) {
    for (let link = pending.pop(); link && !seen.has(link); link = link.up) {
      if (link === target) return true;
      seen.add(link);
      if (link.waiters) for (const waiter of link.waiters) pending.push(waiter);
    }
  }
  return false;
}

// out of line so the sync path captures nothing more

function waitFor(hit: Entry, link: Link, up: Link | undefined, payload: ParsePayload): Promise<ParsePayload> {
  if (up) {
    link.waiters ??= new Set();
    link.waiters.add(up);
  }
  return link.done!.then(() => {
    replay(hit.issues as unknown[], payload.issues);
    return payload;
  });
}


function bucketFor(buckets: State["buckets"], inst: $ZodType): WeakMap<object, Entry> {
  let bucket = buckets.get(inst);
  if (!bucket) {
    bucket = new WeakMap();
    buckets.set(inst, bucket);
  }
  return bucket;
}

// Set immediately before delegating to core and cleared immediately after, so `alloc` registers only for a visit this module is driving.

const memo: $ZodMemoizer = {
  alloc(_inst, payload, empty) {
    const bucket = handoff;
    if (!bucket) return empty;
    handoff = undefined;
    const entry: Entry = { value: empty, issues: null };
    bucket.set(payload.value as object, entry);
    open.push(entry);
    return empty;
  },

  guard(inst) {
    inst._zod.deferred ??= [];
    inst._zod.deferred.push(() => {
      const base = inst._zod.parse;
      const wrapped = (payload: ParsePayload, ctx: ParseContextInternal): util.MaybeAsync<ParsePayload> => {
        // The value is a placeholder a back-edge is still waiting on, so the cycle closes through this transform. Its output can't exist in time to bind.
        if (ctx.direction !== "backward" && isBackEdge(ctx, payload.value)) throw new $ZodCyclicError();
        return base(payload, ctx);
      };
      inst._zod.parse = wrapped;
      if (inst._zod.run === base) inst._zod.run = wrapped;
    });
  },

  attach(inst) {
    let isRecursiveInst: boolean | undefined;
    let rechecked = false;
    // a recursive schema is re-entered many times per pass and its bucket never changes
    let lastBuckets: State["buckets"] | undefined;
    let lastBucket: WeakMap<object, Entry> | undefined;

    // Wraps `parse` in a deferred so it sees the container's final parse. Core's own deferred copies `parse` into `run` when there are no checks, and it ran first, so `run` is patched to match; with checks, `run` reads `parse` dynamically.
    inst._zod.deferred ??= [];
    inst._zod.deferred.push(() => {
      const base = inst._zod.parse;

      const wrapped = (payload: ParsePayload, ctx: ParseContextInternal): util.MaybeAsync<ParsePayload> => {
        if (isRecursiveInst === undefined) {
          const walked = isRecursive(inst, new Set(), false);
          if (walked === NONE) {
            // Nothing here can ever fire, so take it back out.
            inst._zod.parse = base;
            if (inst._zod.run === wrapped) inst._zod.run = base;
            return base(payload, ctx);
          }
          // this parse resolves the deferred edges on its own path, so ask once more before latching
          if (walked === PROVEN || rechecked) isRecursiveInst = true;
          else rechecked = true;
        }

        const input = payload.value;
        if (!isRef(input)) return base(payload, ctx);

        let state = (ctx as WithState)[STATE];
        if (!state) {
          state = { buckets: new WeakMap(), unchecked: undefined, backEdges: undefined };
          (ctx as WithState)[STATE] = state;
        }

        // an encode's check-free first pass shares the state through its spread context, so it keeps buckets of its own
        if (ctx.skipChecks) state.unchecked ??= new WeakMap();
        const buckets = ctx.skipChecks ? state.unchecked! : state.buckets;
        // keyed on the pass, since an async visit hands its children a context of their own that must not outlive the parse
        if (lastBuckets !== buckets) {
          lastBuckets = buckets;
          lastBucket = bucketFor(buckets, inst);
        }
        const bucket = lastBucket!;

        const hit = bucket.get(input);
        if (hit) {
          payload.value = hit.value;
          const issues = hit.issues;
          if (Array.isArray(issues)) {
            replay(issues, payload.issues);
          } else if (issues && !blockedOn((ctx as WithState)["~link"], issues)) {
            // a concurrent visit holds the entry and may discard its issues, so reuse them only once it finishes
            return waitFor(hit, issues, (ctx as WithState)["~link"], payload);
          } else {
            // still being parsed by a visit that needs this one: its own checks cover it, so skip them here
            payload.memo = true;
            state.backEdges ??= new WeakSet();
            state.backEdges.add(hit.value as object);
          }
          return payload;
        }

        let link: Link | undefined;
        if (ctx.async) {
          link = { up: (ctx as WithState)["~link"], done: undefined, waiters: undefined };
          // a literal key, since a computed one here cost 3x
          ctx = { ...ctx, "~link": link } as ParseContextInternal;
        }

        handoff = bucket;
        const depth = open.length;
        const start = payload.issues.length;
        const result = base(payload, ctx);
        handoff = undefined;
        // A container that rejected its input outright allocated nothing.
        const entry = open.length > depth ? open.pop()! : undefined;

        // Both paths written out so the sync one allocates no closure. It runs once per node, and capturing here cost more than everything else combined.
        if (result instanceof Promise) {
          const done = result.then((r) => {
            if (entry) entry.issues = record(r.issues, start);
            return r;
          });
          if (entry && link) {
            link.done = done;
            entry.issues = link;
          }
          return done;
        }
        if (entry) entry.issues = record(result.issues, start);
        return result;
      };

      inst._zod.parse = wrapped;
      if (inst._zod.run === base) inst._zod.run = wrapped;
    });
  },
};

/** The memoizer that gives containers cycle support. `zod` installs it by default; `zod/mini` opts in with `config({ memoizer: memoizer() })`. */
