export const $ZodRecord: core.$constructor<$ZodRecord> = /*@__PURE__*/ core.$constructor("$ZodRecord", (inst, def) => {
  $ZodType.init(inst, def);
  const memo = core.globalConfig.memoizer;
  memo?.attach(inst);

  inst._zod.parse = (payload, ctx) => {
    const input = payload.value;

    if (!util.isPlainObject(input)) {
      payload.issues.push({
        expected: "record",
        code: "invalid_type",

        input,
        inst,
      });
      return payload;
    }

    // no guard in either loop below: a record's invalid_key aborts but an enclosing intersection can reconcile it, so a stopped loop hides keys the sibling does not own and the intersection then rejects nothing
    const proms: Promise<any>[] = [];

    const values = def.keyType._zod.values;
    if (values && !def.partial) {
      payload.value = memo ? memo.alloc(inst, payload, {}, ctx) : {};
      const recordKeys = new Set<string | symbol>();
      for (const key of values) {
        if (typeof key === "string" || typeof key === "number" || typeof key === "symbol") {
          recordKeys.add(typeof key === "number" ? key.toString() : key);
          // A declared __proto__ is stripped but is not an unrecognized key.
          if (key === "__proto__") continue;
          const keyResult = def.keyType._zod.run({ value: key, issues: [] }, ctx);
          if (keyResult instanceof Promise) {
            throw new Error("Async schemas not supported in object keys currently");
          }
          if (keyResult.issues.length) {
            payload.issues.push({
              code: "invalid_key",
              origin: "record",
              issues: keyResult.issues.map((iss) => util.finalizeIssue(iss, ctx, core.config())),
              input: key,
              path: [key],
              inst,
            });
            continue;
          }
          const outKey = keyResult.value as PropertyKey;
          if (outKey === "__proto__") continue;
          const result = def.valueType._zod.run({ value: input[key], issues: [] }, ctx);

          if (result instanceof Promise) {
            proms.push(
              result.then((result) => {
                if (result.issues.length) {
                  payload.issues.push(...util.prefixIssues(key, result.issues));
                }
                payload.value[outKey] = result.value;
              })
            );
          } else {
            if (result.issues.length) {
              payload.issues.push(...util.prefixIssues(key, result.issues));
            }
            payload.value[outKey] = result.value;
          }
        }
      }

      let unrecognized!: string[];
      for (const key in input) {
        if (!recordKeys.has(key)) {
          if (def.mode === "loose") {
            // skip __proto__ so it can't replace the result prototype via the assignment setter on the plain {} we build into
            if (key === "__proto__") continue;
            payload.value[key] = input[key];
          } else {
            unrecognized = unrecognized ?? [];
            unrecognized.push(key);
          }
        }
      }
      if (unrecognized && unrecognized.length > 0) {
        payload.issues.push({
          code: "unrecognized_keys",

          input,
          inst,
          keys: unrecognized,
          continue: true,
        });
      }
    } else {
      payload.value = memo ? memo.alloc(inst, payload, {}, ctx) : {};
      // An enumerable key schema declares which keys the record owns, so a key outside the set is unrecognized. A non-enumerable one (regex, refine) is a constraint every key must satisfy, so a failing key is invalid. Only the former is reconcilable against the other side of an intersection.
      let unrecognized!: string[];
      // Reflect.ownKeys for Symbol-key support; filter non-enumerable to match z.object()
      for (const key of Reflect.ownKeys(input)) {
        if (!Object.prototype.propertyIsEnumerable.call(input, key)) continue;
        let keyResult = def.keyType._zod.run({ value: key, issues: [] }, ctx);
        if (keyResult instanceof Promise) {
          throw new Error("Async schemas not supported in object keys currently");
        }

        // Numeric string fallback: if key is a numeric string and failed, retry with Number(key). This handles z.number(), z.literal([1, 2, 3]), and unions containing numeric literals
        const checkNumericKey = typeof key === "string" && regexes.number.test(key) && keyResult.issues.length;
        if (checkNumericKey) {
          const retryResult = def.keyType._zod.run({ value: Number(key), issues: [] }, ctx);
          if (retryResult instanceof Promise) {
            throw new Error("Async schemas not supported in object keys currently");
          }
          if (retryResult.issues.length === 0) {
            keyResult = retryResult;
          }
        }

        if (keyResult.issues.length) {
          if (def.mode === "loose") {
            // pass through unchanged, except __proto__, which would replace the result prototype
            if (key !== "__proto__") payload.value[key] = input[key];
          } else if (values) {
            unrecognized = unrecognized ?? [];
            unrecognized.push(key as string);
          } else {
            // Default "strict" behavior: error on invalid key
            payload.issues.push({
              code: "invalid_key",
              origin: "record",
              issues: keyResult.issues.map((iss) => util.finalizeIssue(iss, ctx, core.config())),
              input: key,
              path: [key],
              inst,
            });
          }
          continue;
        }

        // never write an input __proto__, even renamed, nor a key normalized into __proto__
        const outKey = keyResult.value as PropertyKey;
        if (key === "__proto__" || outKey === "__proto__") continue;

        const result = def.valueType._zod.run({ value: input[key], issues: [] }, ctx);

        if (result instanceof Promise) {
          proms.push(
            result.then((result) => {
              if (result.issues.length) {
                payload.issues.push(...util.prefixIssues(key, result.issues));
              }
              payload.value[outKey] = result.value;
            })
          );
        } else {
          if (result.issues.length) {
            payload.issues.push(...util.prefixIssues(key, result.issues));
          }
          payload.value[outKey] = result.value;
        }
      }

      if (unrecognized && unrecognized.length > 0) {
        payload.issues.push({
          code: "unrecognized_keys",

          input,
          inst,
          keys: unrecognized,
          continue: true,
        });
      }
    }

    if (proms.length) {
      return Promise.all(proms).then(() => payload);
    }
    return payload;
  };
});

///////////////////////////////////////
///////////////////////////////////////
//////////                   //////////
//////////      $ZodMap      //////////
//////////                   //////////
///////////////////////////////////////
///////////////////////////////////////

