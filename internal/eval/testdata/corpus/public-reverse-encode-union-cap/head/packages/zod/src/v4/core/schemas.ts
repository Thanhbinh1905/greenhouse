export const $ZodType: core.$constructor<$ZodType> = /*@__PURE__*/ core.$constructor<$ZodType>(
  "$ZodType",
  (inst, def) => {
    inst ??= {} as any;

    inst._zod.def = def; // set _def property
    inst._zod.bag = inst._zod.bag || {}; // initialize _bag object
    inst._zod.version = version;

    const defChecks = inst._zod.def.checks;
    // if inst is itself a checks.$ZodCheck, run it as a check
    const checks: checks.$ZodCheck<never>[] = inst._zod.traits.has("$ZodCheck")
      ? [inst as any, ...(defChecks ?? [])]
      : defChecks?.length
        ? [...defChecks]
        : [];

    for (const ch of checks) {
      for (const fn of ch._zod.onattach) {
        fn(inst);
      }
    }

    if (checks.length === 0) {
      // deferred initializer inst._zod.parse is not yet defined
      inst._zod.deferred ??= [];
      inst._zod.deferred?.push(() => {
        inst._zod.run = inst._zod.parse;
      });
    } else {
      const runChecks = (
        payload: ParsePayload,
        checks: checks.$ZodCheck<never>[],
        ctx?: ParseContextInternal | undefined
      ): util.MaybeAsync<ParsePayload> => {
        if (payload.memo) return payload;

        let isAborted = util.aborted(payload);

        let asyncResult!: Promise<unknown> | undefined;
        for (const ch of checks) {
          if (ch._zod.def.when) {
            if (util.explicitlyAborted(payload)) continue;
            const shouldRun = ch._zod.def.when(payload);
            if (!shouldRun) continue;
          } else if (isAborted) {
            continue;
          }
          const currLen = payload.issues.length;
          const _ = ch._zod.check(payload as any) as any as ParsePayload;

          if (_ instanceof Promise && ctx?.async === false) {
            throw new core.$ZodAsyncError();
          }
          if (asyncResult || _ instanceof Promise) {
            asyncResult = (asyncResult ?? Promise.resolve()).then(async () => {
              await _;
              const nextLen = payload.issues.length;
              if (nextLen === currLen) return;
              util.attachSchema(payload.issues, currLen, inst);
              if (!isAborted) isAborted = util.aborted(payload, currLen);
            });
          } else {
            const nextLen = payload.issues.length;
            if (nextLen === currLen) continue;
            util.attachSchema(payload.issues, currLen, inst);
            if (!isAborted) isAborted = util.aborted(payload, currLen);
          }
        }

        if (asyncResult) {
          return asyncResult.then(() => {
            return payload;
          });
        }
        return payload;
      };

      const handleCanaryResult = (canary: ParsePayload, payload: ParsePayload, ctx: ParseContextInternal) => {
        // abort if the canary is aborted
        if (util.aborted(canary)) {
          canary.aborted = true;
          return canary;
        }

        // run checks first, then
        const checkResult = runChecks(payload, checks, ctx);
        if (checkResult instanceof Promise) {
          if (ctx.async === false) throw new core.$ZodAsyncError();
          return checkResult.then((checkResult) => inst._zod.parse(checkResult, ctx));
        }
        return inst._zod.parse(checkResult, ctx);
      };

      inst._zod.run = (payload, ctx) => {
        if (ctx.skipChecks) {
          return inst._zod.parse(payload, ctx);
        }
        if (ctx.direction === "backward") {
          // run canary initial pass (no checks)
          const canary = inst._zod.parse({ value: payload.value, issues: [] }, { ...ctx, skipChecks: true });

          if (canary instanceof Promise) {
            return canary.then((canary) => {
              return handleCanaryResult(canary, payload, ctx);
            });
          }

          return handleCanaryResult(canary, payload, ctx);
        }

        // forward
        const result = inst._zod.parse(payload, ctx);
        if (result instanceof Promise) {
          if (ctx.async === false) throw new core.$ZodAsyncError();
          return result.then((result) => runChecks(result, checks, ctx));
        }

        return runChecks(result, checks, ctx);
      };
    }
  },
  {
    // Wrappers extend this by installing a richer factory over it; reading it eagerly would defeat the laziness.
    get "~standard"(): StandardSchemaV1.Props<any, any> {
      return util.hide(this, "~standard", standardProps(this));
    },
    set "~standard"(value: StandardSchemaV1.Props<any, any>) {
      util.own(this, "~standard", value);
    },
  }
);

/** The Standard Schema surface for `inst`. Shared so wrappers can extend it without forcing it. */
// a Standard Schema result only reports issues, so a failure finalizes them straight off the raw payload: no ZodError, and no lazy result to read through
