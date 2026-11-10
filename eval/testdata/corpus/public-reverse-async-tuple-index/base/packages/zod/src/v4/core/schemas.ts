export const $ZodTuple: core.$constructor<$ZodTuple> = /*@__PURE__*/ core.$constructor("$ZodTuple", (inst, def) => {
  $ZodType.init(inst, def);
  const items = def.items;
  const memo = core.globalConfig.memoizer;
  memo?.attach(inst);

  inst._zod.parse = (payload, ctx) => {
    const input = payload.value;
    if (!Array.isArray(input)) {
      payload.issues.push({
        input,
        inst,
        expected: "tuple",
        code: "invalid_type",
      });
      return payload;
    }

    payload.value = memo ? memo.alloc(inst, payload, [] as unknown[], ctx) : [];
    const proms: Promise<any>[] = [];

    const optinStart = getTupleOptStart(items, "optin");
    const optoutStart = getTupleOptStart(items, "optout");

    if (!def.rest) {
      if (input.length < optinStart) {
        payload.issues.push({
          code: "too_small",
          minimum: optinStart,
          inclusive: true,
          input,
          inst,
          origin: "array" as const,
        });
        return payload;
      }
      if (input.length > items.length) {
        payload.issues.push({
          code: "too_big",
          maximum: items.length,
          inclusive: true,
          input,
          inst,
          origin: "array" as const,
        });
      }
    }

    // Run every item in parallel, collecting results into an indexed array. The post-processing in `handleTupleResults` walks them in order so it can decide whether an absent optional-output error can truncate the tail or must be reported to preserve required output.
    const itemResults: ParsePayload[] = new Array(items.length);
    // only tracked when there is a rest loop to skip
    const abortEarly = def.rest ? ctx?.abortEarly : undefined;
    let itemAborted = false;
    for (let i = 0; i < items.length; i++) {
      const r = items[i]._zod.run({ value: input[i], issues: [] }, ctx);
      if (r instanceof Promise) {
        proms.push(
          r.then((rr) => {
            itemResults[i] = rr;
          })
        );
      } else {
        itemResults[i] = r;
        if (abortEarly && !itemAborted && r.issues.length) itemAborted = util.aborted(r);
      }
    }

    // sound because rest is non-empty exactly when every fixed index is present, the one case handleTupleResults cannot discard an item's issues
    if (def.rest && !itemAborted) {
      let seen = payload.issues.length;
      // the loop's own `let i` gives each async element its own index
      for (let i = items.length; i < input.length; i++) {
        if (abortEarly && payload.issues.length !== seen) {
          if (util.aborted(payload, seen)) break;
          seen = payload.issues.length;
        }
        const result = def.rest._zod.run({ value: input[i], issues: [] }, ctx);
        if (result instanceof Promise) {
          proms.push(result.then((r) => handleTupleResult(r, payload, i)));
        } else {
          handleTupleResult(result, payload, i);
        }
      }
    }

    if (proms.length) {
      return Promise.all(proms).then(() => handleTupleResults(itemResults, payload, items, input, optoutStart));
    }
    return handleTupleResults(itemResults, payload, items, input, optoutStart);
  };
});


