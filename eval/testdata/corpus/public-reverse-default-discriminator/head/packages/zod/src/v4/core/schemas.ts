export interface $ZodDiscriminatedUnionInternals<
  Options extends readonly SomeType[] = readonly $ZodType[],
  Disc extends string = string,
> extends $ZodUnionInternals<Options> {
  def: $ZodDiscriminatedUnionDef<Options, Disc>;
  propValues: util.PropValues;
  bag: util.LoosePartial<{
    optionsMap: Map<util.Primitive, $ZodType>;
  }>;
}

/** The discriminator values a member of `Options` can declare. An omittable discriminator contributes `undefined`, matching what `propValues` claims for it. */

export type $DiscriminatedOption<Options extends readonly SomeType[], Disc extends string, V> = {
  [I in keyof Options]: Options[I] extends { _zod: { output: infer Out } }
    ? Disc extends keyof Out
      ? V extends Out[Disc]
        ? Options[I]
        : never
      : never
    : never;
}[number];

/** Returns the option of `union` whose discriminator claims `value`. */

export function getDiscriminatedOption<
  Options extends readonly SomeType[],
  Disc extends string,
  const V extends $DiscriminatorValue<Options, Disc>,
>(union: $ZodDiscriminatedUnion<Options, Disc>, value: V): $DiscriminatedOption<Options, Disc, V> {
  const internals = union._zod;
  let map = internals.bag.optionsMap;
  if (!map) {
    map = new Map();
    const { options, discriminator } = internals.def;
    for (const option of options as unknown as readonly $ZodType[]) {
      // First declaration wins, matching the order the parse path resolves a duplicate in.
      for (const v of option._zod.propValues?.[discriminator] ?? []) if (!map.has(v)) map.set(v, option);
    }
    internals.bag.optionsMap = map;
  }
  return map.get(value as util.Primitive) as any;
}


export const $ZodDiscriminatedUnion: core.$constructor<$ZodDiscriminatedUnion> =
  /*@__PURE__*/
  core.$constructor("$ZodDiscriminatedUnion", (inst, def) => {
    def.inclusive = false;

    $ZodUnion.init(inst, def);

    const _super = inst._zod.parse;
    util.defineLazyInternal(inst, "propValues", (zod) => {
      const propValues: util.PropValues = {};
      for (const option of zod.def.options) {
        const pv = option._zod.propValues;
        if (!pv || Object.keys(pv).length === 0)
          throw new Error(`Invalid discriminated union option at index "${zod.def.options.indexOf(option)}"`);
        for (const [k, v] of Object.entries(pv!)) {
          if (!Object.prototype.hasOwnProperty.call(propValues, k)) {
            util.assignProp(propValues, k, new Set());
          }
          for (const val of v) {
            propValues[k].add(val);
          }
        }
      }
      return propValues;
    });

    // Checked now rather than in the lookup map below, so an option that lacks the discriminator fails at the `discriminatedUnion` call instead of on the first object parsed. Options whose shape cannot be enumerated without resolving it — pipes and lazies — are left to the map.
    def.options.forEach((option, i) => {
      const propShape = util.rawShape(option._zod.def);
      if (propShape && !Object.prototype.hasOwnProperty.call(propShape, def.discriminator)) {
        throw new Error(`Invalid discriminated union option at index "${i}"`);
      }
    });

    const disc = util.cached(() => {
      const opts = def.options;
      const map: Map<util.Primitive, $ZodType> = new Map();
      for (const o of opts) {
        const values = o._zod.propValues?.[def.discriminator];
        if (!values || values.size === 0)
          throw new Error(`Invalid discriminated union option at index "${def.options.indexOf(o)}"`);
        for (const v of values) {
          if (map.has(v)) {
            throw new Error(`Duplicate discriminator value "${String(v)}"`);
          }
          map.set(v, o);
        }
      }
      return map;
    });

    inst._zod.parse = (payload, ctx) => {
      const input = payload.value;
      if (!util.isObject(input)) {
        payload.issues.push({
          code: "invalid_type",

          expected: "object",
          input,
          inst,
        });
        return payload;
      }

      const opt = disc.value.get(input?.[def.discriminator] as any);
      if (opt) {
        return opt._zod.run(payload, ctx) as any;
      }

      // Fall back to union matching when the fast discriminator path fails:
      // - explicitly enabled via unionFallback, or
      // - during backward direction (encode), since codec-based discriminators have different values in forward vs backward directions
      if (def.unionFallback || ctx.direction === "backward") {
        return _super(payload, ctx);
      }

      // no matching discriminator
      payload.issues.push({
        code: "invalid_union",
        errors: [],
        note: "No matching discriminator",
        discriminator: def.discriminator,
        options: Array.from(disc.value.keys()),
        input,
        path: [def.discriminator],
        inst,
      });

      return payload;
    };
  });

////////////////////////////////////////////////
////////////////////////////////////////////////
//////////                            //////////
//////////      $ZodIntersection      //////////
//////////                            //////////
////////////////////////////////////////////////
////////////////////////////////////////////////
