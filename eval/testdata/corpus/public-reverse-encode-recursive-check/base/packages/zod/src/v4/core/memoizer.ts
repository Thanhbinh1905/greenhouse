interface State {
  /** Weak throughout because `attach` caches a bucket on the schema instance, which outlives the parse. */
  buckets: WeakMap<$ZodType, WeakMap<object, Entry>>;
  /** The same for a pass with checks skipped, whose results a checked pass must never reuse. */
  unchecked: WeakMap<$ZodType, WeakMap<object, Entry>> | undefined;
  /** Nodes a back-edge resolved to before they finished. */
  backEdges: WeakSet<object> | undefined;
}

/** Keyed off the context object every schema in one parse call already shares. */

function isRef(value: unknown): value is object {
  return value !== null && typeof value === "object";
}

// the issues from `start` on (an encode runs a node's own checks first), each by reference beside its path length since receivers only prepend, or copied while pathless

function record(issues: errors.$ZodRawIssue[], start: number): unknown[] {
  if (issues.length <= start) return NO_ISSUES;
  const out: unknown[] = [];
  for (let i = start; i < issues.length; i++) {
    const iss = issues[i]!;
    out.push(iss.path ? iss : { ...iss }, iss.path?.length);
  }
  return out;
}

// a fresh copy per hand-out, its path cut back to the recorded length

function bucketFor(state: State, inst: $ZodType, ctx: ParseContextInternal): WeakMap<object, Entry> {
  let buckets = state.buckets;
  // an encode's check-free first pass shares the state through its spread context, so it keeps buckets of its own
  if (ctx.skipChecks) {
    state.unchecked ??= new WeakMap();
    buckets = state.unchecked;
  }
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
    // a recursive schema is re-entered many times per parse and its bucket never changes
    let lastCtx: object | undefined;
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

        let bucket: WeakMap<object, Entry>;
        if (lastCtx === ctx) {
          bucket = lastBucket!;
        } else {
          bucket = bucketFor(state, inst, ctx);
          lastCtx = ctx;
          lastBucket = bucket;
        }

        const hit = bucket.get(input);
        if (hit) {
          payload.value = hit.value;
          if (hit.issues) {
            replay(hit.issues, payload.issues);
          } else {
            // Still being parsed: its own checks cover it, so skip them here.
            payload.memo = true;
            state.backEdges ??= new WeakSet();
            state.backEdges.add(hit.value as object);
          }
          return payload;
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
          return result.then((r) => {
            if (entry) entry.issues = record(r.issues, start);
            return r;
          });
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
