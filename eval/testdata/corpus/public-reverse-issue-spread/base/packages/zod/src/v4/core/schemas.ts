function handleArrayResult(result: ParsePayload<any>, final: ParsePayload<any[]>, index: number) {
  if (result.issues.length) {
    util.pushPrefixed(final.issues, index, result.issues);
  }
  final.value[index] = result.value;
}


function handlePropertyResult(
  result: ParsePayload,
  final: ParsePayload,
  key: PropertyKey,
  input: any,
  optin: "optional" | "defaulted" | undefined,
  optout: "optional" | undefined
) {
  const isPresent = key in input;
  const isOptionalOut = optout === "optional";
  // The middle rung means "absence permitted, nothing supplied in its place", so an absent key contributes nothing — whatever the schema made of `undefined` is invented, not substituted. Only `optional` reaches this with a value: `defaulted` substitutes, and a schema that isn't optional-out has to keep the key.
  if (!isPresent && isOptionalOut && optin === "optional") {
    return;
  }
  if (result.issues.length) {
    // For optional-in/out schemas, ignore errors on absent keys.
    if (optin !== undefined && isOptionalOut && !isPresent) {
      return;
    }
    util.pushPrefixed(final.issues, key, result.issues);
  }

  if (!isPresent && optin === undefined) {
    if (!result.issues.length) {
      final.issues.push({
        code: "invalid_type",
        expected: "nonoptional",
        input: undefined,
        path: [key],
      });
    }
    return;
  }

  if (result.value === undefined) {
    if (isPresent || (optin === "defaulted" && !isOptionalOut)) {
      (final.value as any)[key] = undefined;
    }
  } else {
    (final.value as any)[key] = result.value;
  }
}


function handleTupleResult(result: ParsePayload, final: ParsePayload<any[]>, index: number) {
  if (result.issues.length) {
    util.pushPrefixed(final.issues, index, result.issues);
  }
  final.value[index] = result.value;
}


function handleTupleResults(
  itemResults: ParsePayload[],
  final: ParsePayload<any[]>,
  items: readonly $ZodType[],
  input: unknown[],
  optoutStart: number
) {
  // Walk results in order. Mirror $ZodObject's swallow-on-absent-optional rule, but only after `optoutStart`: the first index where the output tuple tail can be absent.
  for (let i = 0; i < items.length; i++) {
    const r = itemResults[i];
    const isPresent = i < input.length;
    // The array analog of `handlePropertyResult`'s absent-key early return: the middle rung permits absence without supplying anything in its place, so the tail truncates here instead of materializing whatever the item made of `undefined`.
    if (!isPresent && i >= optoutStart && items[i]._zod.optin === "optional") {
      final.value.length = i;
      break;
    }
    if (r.issues.length) {
      if (!isPresent && i >= optoutStart) {
        final.value.length = i;
        break;
      }
      util.pushPrefixed(final.issues, i, r.issues);
    }
    final.value[i] = r.value;
  }

  // Drop trailing slots that produced `undefined` for absent input
  // (the array analog of an absent optional key on an object). The
  // `i >= input.length` floor is critical: an explicit `undefined`
  // *inside* the input must be preserved even when the schema is
  // optional-out (e.g. `z.string().or(z.undefined())` accepting an
  // explicit undefined value).
  for (let i = final.value.length - 1; i >= input.length; i--) {
    if (items[i]._zod.optout === "optional" && final.value[i] === undefined) {
      final.value.length = i;
    } else {
      break;
    }
  }
  return final;
}

//////////////////////////////////////////
//////////////////////////////////////////
//////////                      //////////
//////////      $ZodRecord      //////////
//////////                      //////////
//////////////////////////////////////////
//////////////////////////////////////////


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
                  util.pushPrefixed(payload.issues, key, result.issues);
                }
                payload.value[outKey] = result.value;
              })
            );
          } else {
            if (result.issues.length) {
              util.pushPrefixed(payload.issues, key, result.issues);
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
                util.pushPrefixed(payload.issues, key, result.issues);
              }
              payload.value[outKey] = result.value;
            })
          );
        } else {
          if (result.issues.length) {
            util.pushPrefixed(payload.issues, key, result.issues);
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

function handleMapResult(
  keyResult: ParsePayload,
  valueResult: ParsePayload,
  final: ParsePayload<Map<unknown, unknown>>,
  key: unknown,
  input: Map<any, any>,
  inst: $ZodMap,
  ctx?: ParseContext | undefined
): void {
  if (keyResult.issues.length) {
    if (util.propertyKeyTypes.has(typeof key)) {
      util.pushPrefixed(final.issues, key as PropertyKey, keyResult.issues);
    } else {
      final.issues.push({
        code: "invalid_key",

        origin: "map",
        input,
        inst,
        issues: keyResult.issues.map((iss) => util.finalizeIssue(iss, ctx, core.config())),
      });
    }
  }
  if (valueResult.issues.length) {
    if (util.propertyKeyTypes.has(typeof key)) {
      util.pushPrefixed(final.issues, key as PropertyKey, valueResult.issues);
    } else {
      final.issues.push({
        origin: "map",
        code: "invalid_element",

        input,
        inst,
        key: key,
        issues: valueResult.issues.map((iss) => util.finalizeIssue(iss, ctx, core.config())),
      });
    }
  }
  final.value.set(keyResult.value, valueResult.value);
}

///////////////////////////////////////
///////////////////////////////////////
//////////                   //////////
//////////      $ZodSet      //////////
//////////                   //////////
///////////////////////////////////////
///////////////////////////////////////

function handleSetResult(result: ParsePayload, final: ParsePayload<Set<any>>) {
  if (result.issues.length) {
    for (const iss of result.issues) final.issues.push(iss);
  }
  final.value.add(result.value);
}

////////////////////////////////////////
////////////////////////////////////////
//////////                    //////////
//////////      $ZodEnum      //////////
//////////                    //////////
////////////////////////////////////////
////////////////////////////////////////
