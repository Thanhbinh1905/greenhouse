export interface $ZodDiscriminatedUnionInternals<
  Options extends readonly SomeType[] = readonly $ZodType[],
  Disc extends string = string,
> extends $ZodUnionInternals<Options> {
  def: $ZodDiscriminatedUnionDef<Options, Disc>;
  propValues: util.PropValues;
  bag: util.LoosePartial<{
    optionsMap: Map<util.Primitive, $ZodType | null>;
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

/** Returns the option whose discriminator claims `value`, or throws if ambiguous. */

export function getDiscriminatedOption<
  Options extends readonly SomeType[],
  Disc extends string,
  const V extends $DiscriminatorValue<Options, Disc>,
>(union: $ZodDiscriminatedUnion<Options, Disc>, value: V): $DiscriminatedOption<Options, Disc, V> {
  const internals = union._zod;
  let map = internals.bag.optionsMap;
  if (!map) {
    map = discriminatorMap(internals.def);
    internals.bag.optionsMap = map;
  }
  const option = map.get(value as util.Primitive);
  if (option === null) throw new Error(`Ambiguous discriminator value "${String(value)}"`);
  return option as any;
}


function discriminatorMap(def: $ZodDiscriminatedUnionDef<readonly SomeType[]>): Map<util.Primitive, $ZodType | null> {
  const map = new Map<util.Primitive, $ZodType | null>();
  for (const option of def.options as readonly $ZodType[]) {
    const values = option._zod.propValues?.[def.discriminator];
    if (!values || values.size === 0)
      throw new Error(`Invalid discriminated union option at index "${def.options.indexOf(option)}"`);
    for (const value of values) {
      if (map.has(value)) {
        if (value !== undefined) throw new Error(`Duplicate discriminator value "${String(value)}"`);
        // keep the collision marked so a later member cannot reclaim it
        map.set(value, null);
      } else {
        map.set(value, option);
      }
    }
  }
  return map;
}


export const $ZodDiscriminatedUnion: core.$constructor<$ZodDiscriminatedUnion> =
  /*@__PURE__*/
  core.$constructor("$ZodDiscriminatedUnion", (inst, def) => {
    def.inclusive = false;

    $ZodUnion.init(inst, def);

    const _super = inst._zod.parse;
    util.defineLazyInternal(inst, "propValues", (zod) => {
      const propValues: util.PropValues = {};
      let undefinedCount = 0;
      for (const option of zod.def.options) {
        const pv = option._zod.propValues;
        if (!pv || Object.keys(pv).length === 0)
          throw new Error(`Invalid discriminated union option at index "${zod.def.options.indexOf(option)}"`);
        if (pv[zod.def.discriminator]?.has(undefined)) undefinedCount++;
        for (const [k, v] of Object.entries(pv!)) {
          if (!Object.prototype.hasOwnProperty.call(propValues, k)) {
            util.assignProp(propValues, k, new Set());
          }
          for (const val of v) {
            propValues[k].add(val);
          }
        }
      }
      if (!zod.def.unionFallback && undefinedCount > 1) propValues[zod.def.discriminator]?.delete(undefined);
      return propValues;
    });

    // Checked now rather than in the lookup map below, so an option that lacks the discriminator fails at the `discriminatedUnion` call instead of on the first object parsed. Options whose shape cannot be enumerated without resolving it — pipes and lazies — are left to the map.
    def.options.forEach((option, i) => {
      const propShape = util.rawShape(option._zod.def);
      if (propShape && !Object.prototype.hasOwnProperty.call(propShape, def.discriminator)) {
        throw new Error(`Invalid discriminated union option at index "${i}"`);
      }
    });

    const disc = util.cached(() => discriminatorMap(def));

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

      const value = input?.[def.discriminator];
      const opt = disc.value.get(value as util.Primitive);
      // forward metadata cannot choose an encoder for an absent tag
      if (opt && (value !== undefined || ctx.direction !== "backward")) {
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
        options: Array.from(disc.value.keys()).filter((value) => disc.value.get(value) !== null),
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
