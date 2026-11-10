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
    final.issues.push(...util.prefixIssues(key, result.issues));
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


export const $ZodObjectJIT: core.$constructor<$ZodObject> = /*@__PURE__*/ core.$constructor(
  "$ZodObjectJIT",
  (inst, def) => {
    // requires cast because technically $ZodObject doesn't extend
    $ZodObject.init(inst, def);

    const superParse = inst._zod.parse;
    const _normalized = util.cached(() => normalizeDef(def));

    const memo = core.globalConfig.memoizer;

    const generateFastpass = (shape: any) => {
      const normalized = _normalized.value;
      const syms = normalized.symbolKeys;
      // a symbol has no source literal, so it is read as `syms[i]` off the closed-over scope
      const doc = new Doc(["payload", "ctx"], { shape, inst, memo, syms });

      const parseStr = (k: string) => `shape[${k}]._zod.run({ value: input[${k}], issues: [] }, ctx)`;

      // prefixes in place, like util.prefixIssues. newResult must land before the early return: a catchall runs after this and would otherwise write onto the caller's input
      const prefixStr = (id: string, k: string) => `
          let ${id}_ab = false;
          for (let i = 0; i < ${id}.issues.length; i++) {
            const iss = ${id}.issues[i];
            iss.path = iss.path ? [${k}, ...iss.path] : [${k}];
            payload.issues.push(iss);
            if (iss.continue !== true) ${id}_ab = true;
          }
          if (${id}_ab && ctx && ctx.abortEarly) {
            payload.value = newResult;
            return payload;
          }`;

      doc.write(`const input = payload.value;`);

      const ids: any = Object.create(null);
      let counter = 0;
      for (const key of normalized.allKeys) {
        ids[key] = `key_${counter++}`;
      }

      // A: preserve key order {
      doc.write(memo ? `const newResult = memo.alloc(inst, payload, {}, ctx);` : `const newResult = {};`);
      for (const key of normalized.allKeys) {
        if (key === "__proto__") continue;
        const id = ids[key];
        const k = typeof key === "symbol" ? `syms[${syms.indexOf(key)}]` : util.esc(key);
        const isPresent = `${k} in input`;
        const schema = shape[key];
        const optin = schema?._zod?.optin;
        const isOptionalIn = optin !== undefined;
        const isOptionalOut = schema?._zod?.optout === "optional";

        doc.write(`const ${id} = ${parseStr(k)};`);

        if (isOptionalIn && isOptionalOut) {
          // For optional-in/out schemas, ignore errors on absent keys — and, like the interpreted path, drop the value produced alongside them. The middle rung goes further: it permits absence without supplying anything in its place, so an absent key contributes nothing at all.
          const assign = optin === "optional" ? `${id}_present` : `${id}.value !== undefined || ${id}_present`;
          doc.write(`
        const ${id}_present = ${isPresent};
        if (!${id}.issues.length || ${id}_present) {
          if (${id}.issues.length) {${prefixStr(id, k)}
          }

          if (${assign}) {
            newResult[${k}] = ${id}.value;
          }
        }

      `);
        } else if (!isOptionalIn) {
          doc.write(`
        const ${id}_present = ${isPresent};
        if (${id}.issues.length) {${prefixStr(id, k)}
        }
        if (!${id}_present && !${id}.issues.length) {
          payload.issues.push({
            code: "invalid_type",
            expected: "nonoptional",
            input: undefined,
            path: [${k}]
          });
          if (ctx && ctx.abortEarly) {
            payload.value = newResult;
            return payload;
          }
        }

        if (${id}_present) {
          newResult[${k}] = ${id}.value;
        }

      `);
        } else {
          doc.write(`
        if (${id}.issues.length) {${prefixStr(id, k)}
        }
      `);
          if (optin === "defaulted") {
            doc.write(`newResult[${k}] = ${id}.value;`);
          } else {
            doc.write(`
        if (${id}.value !== undefined || ${isPresent}) {
          newResult[${k}] = ${id}.value;
        }
      `);
          }
        }
      }

      doc.write(`payload.value = newResult;`);
      doc.write(`return payload;`);
      // closing `shape` in is what pays: turbofan specializes the parser against that one shape object, so every `shape[k]._zod.run` folds to a known callee. as a parameter it stays a generic load and measures 13% slower even with the forwarding frame gone
      return doc.compile() as (payload: any, ctx: any) => any;
    };

    let fastpass!: ReturnType<typeof generateFastpass>;

    const isObject = util.isObject;
    const jit = !core.globalConfig.jitless;
    const allowsEval = util.allowsEval;

    const fastEnabled = jit && allowsEval.value; // && !def.catchall;
    const catchall = def.catchall;

    let value!: typeof _normalized.value;

    inst._zod.parse = (payload, ctx) => {
      value ??= _normalized.value;
      const input = payload.value;
      if (!isObject(input)) {
        payload.issues.push({
          expected: "object",
          code: "invalid_type",
          input,
          inst,
        });
        return payload;
      }

      if (jit && fastEnabled && ctx?.async === false && ctx.jitless !== true) {
        // always synchronous
        if (!fastpass) fastpass = generateFastpass(def.shape);
        payload = fastpass(payload, ctx);

        if (!catchall) return payload;
        return handleCatchall([], input, payload, ctx, value, inst, ctx?.abortEarly === true);
      }

      return superParse(payload, ctx);
    };
  }
);

/////////////////////////////////////////
/////////////////////////////////////////
//////////                    ///////////
//////////      $ZodUnion      //////////
//////////                    ///////////
/////////////////////////////////////////
/////////////////////////////////////////
// use generic to distribute union types

export interface $ZodPrefaultInternals<T extends SomeType = $ZodType>
  extends $ZodTypeInternals<core.output<T>, core.input<T> | undefined> {
  def: $ZodPrefaultDef<T>;
  optin: "defaulted";
  optout?: "optional" | undefined;
  isst: never;
  values: T["_zod"]["values"];
}
