export function defineLazy<T, K extends keyof T>(object: T, key: K, getter: () => T[K]): void {
  let value: T[K] | typeof EVALUATING | undefined = undefined;
  Object.defineProperty(object, key, {
    get() {
      if (value === EVALUATING) {
        // Circular reference detected, return undefined to break the cycle
        return undefined as T[K];
      }
      if (value === undefined) {
        value = EVALUATING;
        try {
          value = getter();
        } catch (err) {
          // memoize nothing, so a later read runs the getter again
          value = undefined;
          throw err;
        }
      }
      return value;
    },
    set(v) {
      Object.defineProperty(object, key, {
        value: v,
        // configurable: true,
      });
      // object[key] = v;
    },
    configurable: true,
  });
}


export function installLazyProp(inst: object, key: string, make: (self: any) => unknown, enumerable: boolean): void {
  const proto = claim(inst, key);
  if (!proto) return;
  Object.defineProperty(proto, key, {
    configurable: true,
    get(this: any) {
      // Shadowed before computing, so a re-entrant read from a self-referential shape resolves to undefined instead of running the getter again. A data property rather than an accessor: an own accessor is the dictionary-mode transition this exists to avoid.
      const desc = { configurable: true, writable: true, enumerable, value: undefined as unknown };
      Object.defineProperty(this, key, desc);
      // a compute that throws leaves the shadow behind, so later reads answer undefined instead of re-throwing; `defineLazyInternal`'s delete-on-catch would cost bytes in every bundle for a case only a throwing user getter reaches
      desc.value = make(this);
      Object.defineProperty(this, key, desc);
      return desc.value;
    },
    set(this: any, value: unknown) {
      Object.defineProperty(this, key, { configurable: true, writable: true, enumerable, value });
    },
  });
}

/** Marks the thunk `_catch` synthesises for a constant catch value. `Function.length` cannot tell that thunk from a user callback — rest and defaulted parameters both report arity 0 — and a user callback reads `ctx.error`, whose issues only finalize correctly against the caller's per-parse error map. Provenance can say what arity cannot. A plain string key rather than `Symbol.for`, whose call at module scope no bundler can prove pure — the same shape that anchored `urlCanParse` into every build. */

